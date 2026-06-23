package core

import (
	"context"
	"image"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/fake"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
)

// fakeEditor records calls and returns canned edited bytes on confirm.
type fakeEditor struct {
	out   capture.Result
	ok    bool
	calls int
}

func (e *fakeEditor) Edit(_ context.Context, in capture.Result) (capture.Result, bool, error) {
	e.calls++
	if !e.ok {
		return in, false, nil
	}
	return e.out, true, nil
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
