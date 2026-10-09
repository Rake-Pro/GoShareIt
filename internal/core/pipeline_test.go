package core

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/edit"
	"github.com/Rake-Pro/GoShareIt/internal/core/fake"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

// fakeEditor records calls and returns canned edited bytes on confirm.
type fakeEditor struct {
	out    capture.Result
	ok     bool
	action edit.Action
	err    error
	calls  int
}

func (e *fakeEditor) Edit(_ context.Context, in capture.Result, _ edit.Opts) (capture.Result, edit.Action, bool, error) {
	e.calls++
	if e.err != nil {
		return in, edit.ActionDefault, false, e.err
	}
	if !e.ok {
		return in, edit.ActionDefault, false, nil
	}
	return e.out, e.action, true, nil
}

func testApp(t *testing.T, cfg *config.Config) (*App, *fake.Clipboard, *fake.Uploader, *fake.Notifier) {
	t.Helper()
	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	cb := &fake.Clipboard{}
	up := fake.NewUploader()
	nt := &fake.Notifier{}
	p := Providers{
		Capturer:  fake.NewCapturer(),
		Uploader:  up,
		Clipboard: cb,
		Notifier:  nt,
		Tray:      fake.Tray{},
		Hotkeys:   fake.NewHotkeyManager(),
	}
	app, err := New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	return app, cb, up, nt
}

func baseCfg() *config.Config {
	cfg := &config.Config{}
	cfg.Upload.DirectLink = true
	cfg.Upload.FilenameTemplate = "goshareit_{datetime}_{rand}.{ext}"
	cfg.AfterUpload.CopyURLToClipboard = true
	cfg.AfterUpload.Notify = true
	return cfg
}

func TestPipelineHappyPath(t *testing.T) {
	app, cb, up, nt := testApp(t, baseCfg())

	res, err := app.RunCapture(context.Background(), capture.FullScreen)
	if err != nil {
		t.Fatalf("RunCapture: %v", err)
	}

	// DirectURL copied to clipboard (direct_link: true).
	if got, _ := cb.ReadText(); got != res.DirectURL {
		t.Errorf("clipboard text = %q, want DirectURL %q", got, res.DirectURL)
	}

	// Uploader received a named body.
	if len(up.Names) != 1 {
		t.Fatalf("uploader calls = %d, want 1", len(up.Names))
	}
	if filepath.Ext(up.Names[0]) != ".png" {
		t.Errorf("uploaded name has wrong ext: %q", up.Names[0])
	}

	// Notify called.
	if nt.Count() != 1 {
		t.Errorf("notify count = %d, want 1", nt.Count())
	}

	// History appended.
	entries, err := app.History().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
	if entries[0].DirectURL != res.DirectURL || entries[0].ShareToken != res.ShareToken {
		t.Errorf("history entry mismatch: %+v", entries[0])
	}
}

func TestPipelinePublicLinkWhenDirectDisabled(t *testing.T) {
	cfg := baseCfg()
	cfg.Upload.DirectLink = false
	app, cb, _, _ := testApp(t, cfg)

	res, err := app.RunCapture(context.Background(), capture.FullScreen)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := cb.ReadText(); got != res.PublicURL {
		t.Errorf("clipboard = %q, want PublicURL %q", got, res.PublicURL)
	}
}

func TestPipelineCopyImageToClipboard(t *testing.T) {
	cfg := baseCfg()
	cfg.AfterCapture.CopyImageToClipboard = true
	app, cb, _, _ := testApp(t, cfg)

	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatal(err)
	}
	if img, ok := cb.ReadImage(); !ok || len(img) == 0 {
		t.Error("expected image written to clipboard")
	}
}

func TestStopRecordingRunsThroughPipeline(t *testing.T) {
	cfg := baseCfg()
	cfg.AfterCapture.CopyImageToClipboard = true // must NOT copy a video to clipboard

	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	cb := &fake.Clipboard{}
	up := fake.NewUploader()
	nt := &fake.Notifier{}
	rec := fake.NewRecorder()
	p := Providers{
		Capturer:  fake.NewCapturer(),
		Recorder:  rec,
		Uploader:  up,
		Clipboard: cb,
		Notifier:  nt,
		Tray:      fake.Tray{},
		Hotkeys:   fake.NewHotkeyManager(),
	}
	app, err := New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}

	if !app.RecordingSupported() {
		t.Fatal("RecordingSupported = false, want true")
	}
	if app.Recording() {
		t.Fatal("Recording = true before start")
	}
	if err := app.StartRecording(context.Background(), capture.VideoFull, image.Rectangle{}); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if !app.Recording() {
		t.Fatal("Recording = false after start")
	}

	res, err := app.StopRecording(context.Background())
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	if app.Recording() {
		t.Fatal("Recording = true after stop")
	}

	// Video uploaded with .mp4 extension and video mime.
	if len(up.Names) != 1 {
		t.Fatalf("uploader calls = %d, want 1", len(up.Names))
	}
	if filepath.Ext(up.Names[0]) != ".mp4" {
		t.Errorf("uploaded name has wrong ext: %q", up.Names[0])
	}
	if up.Mimes[0] != "video/mp4" {
		t.Errorf("uploaded mime = %q, want video/mp4", up.Mimes[0])
	}

	// URL copied to clipboard, but the video was NOT copied as an image.
	if got, _ := cb.ReadText(); got != res.DirectURL {
		t.Errorf("clipboard text = %q, want DirectURL %q", got, res.DirectURL)
	}
	if _, ok := cb.ReadImage(); ok {
		t.Error("video must not be copied to clipboard as an image")
	}

	// History appended and notify fired.
	entries, err := app.History().List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
	if nt.Count() != 1 {
		t.Errorf("notify count = %d, want 1", nt.Count())
	}
}

func TestRecordingUnsupportedWhenNilRecorder(t *testing.T) {
	app, _, _, _ := testApp(t, baseCfg())
	if app.RecordingSupported() {
		t.Error("RecordingSupported = true with nil recorder")
	}
	if app.Recording() {
		t.Error("Recording = true with nil recorder")
	}
	if err := app.StartRecording(context.Background(), capture.VideoFull, image.Rectangle{}); err == nil {
		t.Error("StartRecording with nil recorder: want error")
	}
	if _, err := app.StopRecording(context.Background()); err == nil {
		t.Error("StopRecording with nil recorder: want error")
	}
}

func TestPipelineEditStepReplacesBytes(t *testing.T) {
	cfg := baseCfg()
	cfg.Editor.Enabled = true
	cfg.Editor.OnModes = []string{"fullscreen"}

	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ed := &fakeEditor{
		out: capture.Result{Bytes: []byte("EDITEDPNG"), Mime: "image/png", Kind: capture.KindImage},
		ok:  true,
	}
	up := fake.NewUploader()
	p := Providers{
		Capturer:  fake.NewCapturer(),
		Uploader:  up,
		Clipboard: &fake.Clipboard{},
		Editor:    ed,
	}
	app, err := New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatal(err)
	}
	if ed.calls != 1 {
		t.Fatalf("editor calls = %d, want 1", ed.calls)
	}
	if len(up.Bodies) != 1 || string(up.Bodies[0]) != "EDITEDPNG" {
		t.Errorf("uploaded body = %q, want EDITEDPNG", up.Bodies[0])
	}
}

// TestPipelineEditActionCopy: the editor's Copy button copies the image to
// clipboard and skips upload/local-save entirely, even when config says both.
func TestPipelineEditActionCopy(t *testing.T) {
	cfg := baseCfg()
	cfg.Editor.Enabled = true
	cfg.Editor.OnModes = []string{"fullscreen"}
	cfg.AfterCapture.SaveLocal = true
	cfg.AfterCapture.SaveDir = t.TempDir()

	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ed := &fakeEditor{
		out:    capture.Result{Bytes: []byte("EDITEDPNG"), Mime: "image/png", Kind: capture.KindImage},
		ok:     true,
		action: edit.ActionCopy,
	}
	up := fake.NewUploader()
	cb := &fake.Clipboard{}
	p := Providers{
		Capturer:  fake.NewCapturer(),
		Uploader:  up,
		Clipboard: cb,
		Editor:    ed,
	}
	app, err := New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatal(err)
	}
	if len(up.Names) != 0 {
		t.Errorf("uploader calls = %d, want 0", len(up.Names))
	}
	if img, ok := cb.ReadImage(); !ok || string(img) != "EDITEDPNG" {
		t.Errorf("clipboard image = %q ok=%v, want EDITEDPNG", img, ok)
	}
	files, err := os.ReadDir(cfg.AfterCapture.SaveDir)
	if err != nil || len(files) != 0 {
		t.Errorf("local save dir: files=%d err=%v, want 0", len(files), err)
	}
}

// TestPipelineEditActionSave: the editor's Save button saves locally and
// skips upload/clipboard-image entirely, even when config says both.
func TestPipelineEditActionSave(t *testing.T) {
	cfg := baseCfg()
	cfg.Editor.Enabled = true
	cfg.Editor.OnModes = []string{"fullscreen"}
	cfg.AfterCapture.CopyImageToClipboard = true
	cfg.AfterCapture.SaveDir = t.TempDir()

	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ed := &fakeEditor{
		out:    capture.Result{Bytes: []byte("EDITEDPNG"), Mime: "image/png", Kind: capture.KindImage},
		ok:     true,
		action: edit.ActionSave,
	}
	up := fake.NewUploader()
	cb := &fake.Clipboard{}
	p := Providers{
		Capturer:  fake.NewCapturer(),
		Uploader:  up,
		Clipboard: cb,
		Editor:    ed,
	}
	app, err := New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatal(err)
	}
	if len(up.Names) != 0 {
		t.Errorf("uploader calls = %d, want 0", len(up.Names))
	}
	if _, ok := cb.ReadImage(); ok {
		t.Error("clipboard image copy must not happen on ActionSave")
	}
	files, err := os.ReadDir(cfg.AfterCapture.SaveDir)
	if err != nil || len(files) != 1 {
		t.Fatalf("local save dir: files=%d err=%v, want 1", len(files), err)
	}
}

// TestPipelineEditActionUpload: the editor's Upload button uploads and skips
// local save/clipboard-image, even when config says both.
func TestPipelineEditActionUpload(t *testing.T) {
	cfg := baseCfg()
	cfg.Editor.Enabled = true
	cfg.Editor.OnModes = []string{"fullscreen"}
	cfg.AfterCapture.SaveLocal = true
	cfg.AfterCapture.SaveDir = t.TempDir()
	cfg.AfterCapture.CopyImageToClipboard = true

	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ed := &fakeEditor{
		out:    capture.Result{Bytes: []byte("EDITEDPNG"), Mime: "image/png", Kind: capture.KindImage},
		ok:     true,
		action: edit.ActionUpload,
	}
	up := fake.NewUploader()
	cb := &fake.Clipboard{}
	p := Providers{
		Capturer:  fake.NewCapturer(),
		Uploader:  up,
		Clipboard: cb,
		Editor:    ed,
	}
	app, err := New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatal(err)
	}
	if len(up.Names) != 1 || string(up.Bodies[0]) != "EDITEDPNG" {
		t.Fatalf("uploader calls = %d bodies = %v, want 1 call with EDITEDPNG", len(up.Names), up.Bodies)
	}
	if _, ok := cb.ReadImage(); ok {
		t.Error("clipboard image copy must not happen on ActionUpload")
	}
	files, err := os.ReadDir(cfg.AfterCapture.SaveDir)
	if err != nil || len(files) != 0 {
		t.Errorf("local save dir: files=%d err=%v, want 0", len(files), err)
	}
}

func TestPipelineEditSkippedWhenModeNotEnabled(t *testing.T) {
	cfg := baseCfg()
	cfg.Editor.Enabled = true
	cfg.Editor.OnModes = []string{"region"} // not fullscreen

	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ed := &fakeEditor{ok: true}
	p := Providers{
		Capturer:  fake.NewCapturer(),
		Uploader:  fake.NewUploader(),
		Clipboard: &fake.Clipboard{},
		Editor:    ed,
	}
	app, err := New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatal(err)
	}
	if ed.calls != 0 {
		t.Errorf("editor calls = %d, want 0 (mode not enabled)", ed.calls)
	}
}

func TestPipelineEditSkippedForVideo(t *testing.T) {
	cfg := baseCfg()
	cfg.Editor.Enabled = true
	cfg.Editor.OnModes = []string{"region", "fullscreen", "window"}

	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ed := &fakeEditor{
		out: capture.Result{Bytes: []byte("SHOULDNOTAPPEAR"), Mime: "image/png", Kind: capture.KindImage},
		ok:  true,
	}
	rec := fake.NewRecorder()
	up := fake.NewUploader()
	p := Providers{
		Capturer:  fake.NewCapturer(),
		Recorder:  rec,
		Uploader:  up,
		Clipboard: &fake.Clipboard{},
		Editor:    ed,
	}
	app, err := New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.StartRecording(context.Background(), capture.VideoFull, image.Rectangle{}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.StopRecording(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ed.calls != 0 {
		t.Errorf("editor calls = %d, want 0 for video", ed.calls)
	}
	if len(up.Mimes) != 1 || up.Mimes[0] != "video/mp4" {
		t.Errorf("uploaded mime = %v, want video/mp4", up.Mimes)
	}
}

func TestPipelineNotifyDisabled(t *testing.T) {
	cfg := baseCfg()
	cfg.AfterUpload.Notify = false
	app, _, _, nt := testApp(t, cfg)

	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatal(err)
	}
	if nt.Count() != 0 {
		t.Errorf("notify count = %d, want 0", nt.Count())
	}
}

// Local-only mode: uploads toggled off - the uploader is never called, the
// URL clipboard copy is skipped, but local save and notify still happen.
func TestPipelineLocalOnly(t *testing.T) {
	cfg := baseCfg()
	off := false
	cfg.Upload.Enabled = &off
	cfg.AfterCapture.SaveLocal = true
	cfg.AfterCapture.SaveDir = t.TempDir()

	app, cb, up, nt := testApp(t, cfg)
	res, err := app.RunCapture(context.Background(), capture.FullScreen)
	if err != nil {
		t.Fatalf("RunCapture: %v", err)
	}
	if res.PublicURL != "" || res.DirectURL != "" {
		t.Errorf("expected empty upload result, got %+v", res)
	}
	if len(up.Names) != 0 {
		t.Fatalf("uploader called %d times, want 0", len(up.Names))
	}
	if got, _ := cb.ReadText(); got != "" {
		t.Errorf("clipboard text = %q, want empty (no URL to copy)", got)
	}
	if len(nt.Notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(nt.Notifications))
	}
	if body := nt.Notifications[0].Body; !strings.Contains(body, "(saved locally)") {
		t.Errorf("notification body = %q, want local-save marker", body)
	}
	files, err := os.ReadDir(cfg.AfterCapture.SaveDir)
	if err != nil || len(files) != 1 {
		t.Fatalf("local save dir: files=%d err=%v, want 1 file", len(files), err)
	}
}

// Runtime toggle: SetUploadEnabled(false) takes effect immediately even when
// the config says uploads are on.
func TestPipelineRuntimeUploadToggle(t *testing.T) {
	app, _, up, _ := testApp(t, baseCfg())
	if !app.UploadEnabled() {
		t.Fatal("uploads should start enabled")
	}
	app.SetUploadEnabled(false)
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatalf("RunCapture: %v", err)
	}
	if len(up.Names) != 0 {
		t.Fatalf("uploader called %d times after disable, want 0", len(up.Names))
	}
	app.SetUploadEnabled(true)
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatalf("RunCapture after re-enable: %v", err)
	}
	if len(up.Names) != 1 {
		t.Fatalf("uploader calls after re-enable = %d, want 1", len(up.Names))
	}
}

// TestPipelineEditCancelOrFailureDiscardsCapture: a cancelled editor (Cancel,
// Esc, window close) and a failed one (crash, timeout) must both drop the
// capture. Nothing may reach the uploader, the clipboard, the local save dir
// or history, even with every after-capture option on.
func TestPipelineEditCancelOrFailureDiscardsCapture(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ed      *fakeEditor
		wantErr bool
	}{
		{"cancel", &fakeEditor{ok: false}, false},
		{"failure", &fakeEditor{err: errors.New("helper killed")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseCfg()
			cfg.Editor.Enabled = true
			cfg.Editor.OnModes = []string{"fullscreen"}
			cfg.AfterCapture.CopyImageToClipboard = true
			cfg.AfterCapture.SaveLocal = true
			cfg.AfterCapture.SaveDir = t.TempDir()

			hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			up := fake.NewUploader()
			cb := &fake.Clipboard{}
			nt := &fake.Notifier{}
			app, err := New(cfg, Providers{
				Capturer:  fake.NewCapturer(),
				Uploader:  up,
				Clipboard: cb,
				Notifier:  nt,
				Editor:    tc.ed,
			}, zerolog.Nop(), hist)
			if err != nil {
				t.Fatal(err)
			}

			res, err := app.RunCaptureEdit(context.Background(), capture.FullScreen)
			if (err != nil) != tc.wantErr {
				t.Fatalf("RunCaptureEdit err = %v, wantErr %v", err, tc.wantErr)
			}
			if res.PublicURL != "" {
				t.Errorf("result = %+v, want empty", res)
			}
			if tc.ed.calls != 1 {
				t.Fatalf("editor calls = %d, want 1", tc.ed.calls)
			}
			if len(up.Names) != 0 {
				t.Errorf("uploader calls = %d, want 0", len(up.Names))
			}
			if _, ok := cb.ReadImage(); ok {
				t.Error("clipboard image written, want none")
			}
			if txt, ok := cb.ReadText(); ok {
				t.Errorf("clipboard text = %q, want none", txt)
			}
			files, err := os.ReadDir(cfg.AfterCapture.SaveDir)
			if err != nil || len(files) != 0 {
				t.Errorf("local save dir: files=%d err=%v, want 0", len(files), err)
			}
			if entries, _ := app.History().List(); len(entries) != 0 {
				t.Errorf("history entries = %d, want 0", len(entries))
			}
			if nt.Count() != 0 {
				t.Errorf("notifications = %d, want 0", nt.Count())
			}
		})
	}
}

// TestPipelineEditWithoutEditorProceeds: with no editor wired, the edit step
// is skipped and the capture runs its normal pipeline.
func TestPipelineEditWithoutEditorProceeds(t *testing.T) {
	app, _, up, _ := testApp(t, baseCfg())
	if _, err := app.RunCaptureEdit(context.Background(), capture.FullScreen); err != nil {
		t.Fatal(err)
	}
	if len(up.Names) != 1 {
		t.Fatalf("uploader calls = %d, want 1", len(up.Names))
	}
}

// "Keep a local copy" with no folder set saves to Pictures/GoShareIt instead
// of failing the capture (and with it the clipboard copy and the upload).
func TestPipelineSaveLocalDefaultsFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := baseCfg()
	cfg.AfterCapture.SaveLocal = true
	cfg.AfterCapture.CopyImageToClipboard = true
	app, cb, up, _ := testApp(t, cfg)
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatalf("RunCapture: %v", err)
	}
	files, err := os.ReadDir(filepath.Join(home, "Pictures", "GoShareIt"))
	if err != nil || len(files) != 1 {
		t.Fatalf("default save dir: files=%d err=%v, want 1", len(files), err)
	}
	if len(up.Names) != 1 {
		t.Errorf("uploader calls = %d, want 1", len(up.Names))
	}
	if _, ok := cb.ReadImage(); !ok {
		t.Error("clipboard image copy skipped")
	}
}

// blockingCapturer holds Capture until release is closed.
type blockingCapturer struct {
	*fake.Capturer
	entered chan struct{}
	release chan struct{}
}

func (b *blockingCapturer) Capture(ctx context.Context, r capture.Request) (capture.Result, error) {
	close(b.entered)
	<-b.release
	return b.Capturer.Capture(ctx, r)
}

// A press that arrives while a capture is in flight is dropped, so two
// editors or two uploads never run at once.
func TestPipelineDropsOverlappingCapture(t *testing.T) {
	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	bc := &blockingCapturer{Capturer: fake.NewCapturer(), entered: make(chan struct{}), release: make(chan struct{})}
	up := fake.NewUploader()
	app, err := New(baseCfg(), Providers{Capturer: bc, Uploader: up, Clipboard: &fake.Clipboard{}}, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := app.RunCapture(context.Background(), capture.FullScreen)
		done <- err
	}()
	<-bc.entered
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err != nil {
		t.Fatalf("overlapping RunCapture: %v", err)
	}
	close(bc.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(up.Names) != 1 {
		t.Errorf("uploader calls = %d, want 1 (second press dropped)", len(up.Names))
	}
}

// A capture the user cancels (Esc in the overlay, a dismissed picker) is not a
// failure: RunCapture returns no error, so no "Capture failed" notice, and
// nothing is uploaded.
func TestPipelineCaptureCancelledIsQuiet(t *testing.T) {
	app, _, up, nt := testApp(t, baseCfg())
	app.capturer.(*fake.Capturer).Err = fmt.Errorf("windows capture: %w", capture.ErrCancelled)
	if _, err := app.RunCapture(context.Background(), capture.RegionInteractive); err != nil {
		t.Fatalf("RunCapture after cancel: err = %v, want nil", err)
	}
	if len(up.Names) != 0 || nt.Count() != 0 {
		t.Errorf("uploads = %d, notifications = %d, want 0 and 0", len(up.Names), nt.Count())
	}
}

// savingCapturer writes the capture into SaveDir when asked, the way the real
// platform capturers do, so tests can see what lands on disk.
type savingCapturer struct{ *fake.Capturer }

func (s savingCapturer) Capture(ctx context.Context, r capture.Request) (capture.Result, error) {
	res, err := s.Capturer.Capture(ctx, r)
	if err != nil || !r.SaveLocal {
		return res, err
	}
	res.Path = filepath.Join(r.SaveDir, "capturer_original.png")
	return res, os.WriteFile(res.Path, res.Bytes, 0o644)
}

// With "Keep a local copy" on, an edited capture must never leave the
// unedited original on disk: nothing after a cancel or failure, and only the
// edited image after a confirm.
func TestPipelineEditKeepsOriginalOffDisk(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ed        *fakeEditor
		wantFiles int
	}{
		{"cancel", &fakeEditor{ok: false}, 0},
		{"failure", &fakeEditor{err: errors.New("helper killed")}, 0},
		{"confirm", &fakeEditor{ok: true, out: capture.Result{Bytes: []byte("EDITEDPNG"), Mime: "image/png", Kind: capture.KindImage}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseCfg()
			cfg.AfterCapture.SaveLocal = true
			cfg.AfterCapture.SaveDir = t.TempDir()
			hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			app, err := New(cfg, Providers{
				Capturer:  savingCapturer{fake.NewCapturer()},
				Uploader:  fake.NewUploader(),
				Clipboard: &fake.Clipboard{},
				Editor:    tc.ed,
			}, zerolog.Nop(), hist)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = app.RunCaptureEdit(context.Background(), capture.FullScreen)
			files, err := os.ReadDir(cfg.AfterCapture.SaveDir)
			if err != nil || len(files) != tc.wantFiles {
				t.Fatalf("save dir: files=%d err=%v, want %d", len(files), err, tc.wantFiles)
			}
			for _, f := range files {
				b, _ := os.ReadFile(filepath.Join(cfg.AfterCapture.SaveDir, f.Name()))
				if string(b) != "EDITEDPNG" {
					t.Errorf("%s holds %q, want only the edited image", f.Name(), b)
				}
			}
		})
	}
}

// blockingUploader holds Upload until release is closed.
type blockingUploader struct {
	*fake.Uploader
	entered chan struct{}
	release chan struct{}
}

func (b *blockingUploader) Upload(ctx context.Context, name string, body io.Reader, size int64, mime string) (upload.UploadResult, error) {
	close(b.entered)
	<-b.release
	return b.Uploader.Upload(ctx, name, body, size, mime)
}

// The one-capture-at-a-time guard covers only the on-screen part: a press
// during a slow upload starts a new capture instead of being dropped.
func TestPipelineGuardReleasedBeforeUpload(t *testing.T) {
	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	capt := fake.NewCapturer()
	bu := &blockingUploader{Uploader: fake.NewUploader(), entered: make(chan struct{}), release: make(chan struct{})}
	app, err := New(baseCfg(), Providers{Capturer: capt, Uploader: bu, Clipboard: &fake.Clipboard{}}, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := app.RunCapture(context.Background(), capture.FullScreen)
		done <- err
	}()
	<-bu.entered
	// Second press while the first upload is still running: it must capture.
	// Its own upload would block on the same channel, so stop it at capture
	// time by making the capturer fail.
	capt.Err = errors.New("second capture reached the capturer")
	if _, err := app.RunCapture(context.Background(), capture.FullScreen); err == nil {
		t.Fatal("second press was dropped while the first capture was uploading")
	}
	close(bu.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
