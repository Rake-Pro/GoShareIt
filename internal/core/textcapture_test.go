package core

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/fake"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/ocrtest"
)

type textRig struct {
	app  *App
	cap  *fake.Capturer
	cb   *fake.Clipboard
	up   *fake.Uploader
	ed   *fakeEditor
	eng  *ocrtest.Fake
	hist *history.History
}

// textApp builds an app whose capturer returns a real (tiny) PNG and whose
// engine is scripted. The editor is wired so a test can prove it never ran.
func textApp(t *testing.T, eng *ocrtest.Fake) textRig {
	t.Helper()
	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 4))); err != nil {
		t.Fatal(err)
	}
	cp := fake.NewCapturer()
	cp.Result = capture.Result{Bytes: buf.Bytes(), Mime: "image/png", Kind: capture.KindImage}
	r := textRig{cap: cp, cb: &fake.Clipboard{}, up: fake.NewUploader(), ed: &fakeEditor{ok: true}, eng: eng, hist: hist}
	cfg := baseCfg()
	p := Providers{Capturer: cp, Uploader: r.up, Clipboard: r.cb, Editor: r.ed, OCR: eng, Notifier: &fake.Notifier{}}
	r.app, err = New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	r.app.reprobeOCR = false
	return r
}

func available() ocr.Status {
	return ocr.Status{Available: true, Engine: ocr.EngineTesseract}
}

func TestCaptureTextCopiesWithoutEditor(t *testing.T) {
	eng := &ocrtest.Fake{Status: available(), Result: ocrtest.Lines("Hello@0,0,20,10 world@22,0,20,10", "second@0,20,30,10")}
	r := textApp(t, eng)
	r.app.ProbeOCR(context.Background())
	out, err := r.app.CaptureText(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.cb.Text != "Hello world\nsecond" {
		t.Fatalf("clipboard text = %q", r.cb.Text)
	}
	if out.Chars != len("Hello world\nsecond") || out.Lines != 2 || out.Cancelled {
		t.Fatalf("outcome = %+v", out)
	}
	if r.ed.calls != 0 {
		t.Fatalf("editor ran %d times", r.ed.calls)
	}
	if len(r.up.Names) != 0 || len(r.cb.Image) != 0 {
		t.Fatalf("uploads %v, clipboard image %d bytes", r.up.Names, len(r.cb.Image))
	}
	if entries, _ := r.hist.List(); len(entries) != 0 {
		t.Fatalf("history got %d entries", len(entries))
	}
	if len(r.cap.Calls) != 1 || r.cap.Calls[0].Mode != capture.RegionInteractive || r.cap.Calls[0].Edit || r.cap.Calls[0].SaveLocal || !r.cap.Calls[0].KeepClipboard {
		t.Fatalf("capture requests = %+v", r.cap.Calls)
	}
	if r.app.capturing.Load() {
		t.Fatal("capture guard still held")
	}
}

func TestCaptureTextCancelIsQuiet(t *testing.T) {
	eng := &ocrtest.Fake{Status: available(), Result: ocrtest.Lines("x@0,0,5,5")}
	r := textApp(t, eng)
	r.app.ProbeOCR(context.Background())
	r.cap.Err = capture.ErrCancelled
	out, err := r.app.CaptureText(context.Background())
	if err != nil || !out.Cancelled {
		t.Fatalf("out = %+v, err = %v", out, err)
	}
	if eng.Calls.Load() != 0 || r.cb.Text != "" {
		t.Fatalf("recognize calls %d, clipboard %q", eng.Calls.Load(), r.cb.Text)
	}
}

func TestCaptureTextUnavailableCapturesNothing(t *testing.T) {
	eng := &ocrtest.Fake{Status: ocr.Status{Reason: "Tesseract is not installed.", Hint: "Install it."}}
	r := textApp(t, eng)
	r.app.ProbeOCR(context.Background())
	_, err := r.app.CaptureText(context.Background())
	var ue *TextUnavailableError
	if !errors.As(err, &ue) || ue.Status.Reason != "Tesseract is not installed." {
		t.Fatalf("err = %v", err)
	}
	if len(r.cap.Calls) != 0 {
		t.Fatal("captured although recognition is unavailable")
	}

	// Before the first probe finishes it is unavailable too ("starting").
	r2 := textApp(t, &ocrtest.Fake{Status: available()})
	if _, err := r2.app.CaptureText(context.Background()); !errors.As(err, &ue) || len(r2.cap.Calls) != 0 {
		t.Fatalf("before probe: err = %v, captures = %d", err, len(r2.cap.Calls))
	}

	// Turned off in the config: unavailable with that reason.
	r3 := textApp(t, &ocrtest.Fake{Status: available()})
	off := false
	r3.app.cfg.OCR.Enabled = &off
	r3.app.ProbeOCR(context.Background())
	st := r3.app.TextStatus()
	if st.Available || st.Reason != "Text recognition is turned off." {
		t.Fatalf("status with ocr off = %+v", st)
	}
	if _, err := r3.app.CaptureText(context.Background()); !errors.As(err, &ue) || len(r3.cap.Calls) != 0 {
		t.Fatalf("ocr off: err = %v", err)
	}
}

func TestCaptureTextReprobesOnLinux(t *testing.T) {
	eng := &ocrtest.Fake{Status: ocr.Status{Reason: "Tesseract is not installed."}, Result: ocrtest.Lines("ok@0,0,5,5")}
	r := textApp(t, eng)
	r.app.reprobeOCR = true
	r.app.ProbeOCR(context.Background())
	eng.Status = available() // installed while the host runs
	if _, err := r.app.CaptureText(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.cb.Text != "ok" {
		t.Fatalf("clipboard = %q", r.cb.Text)
	}
}

func TestCaptureTextNoTextAndFailures(t *testing.T) {
	eng := &ocrtest.Fake{Status: available()}
	r := textApp(t, eng)
	r.app.ProbeOCR(context.Background())
	r.cb.Text = "previous"
	out, err := r.app.CaptureText(context.Background())
	if err != nil || out.Chars != 0 || r.cb.Text != "previous" {
		t.Fatalf("no text: out %+v err %v clipboard %q", out, err, r.cb.Text)
	}

	eng.Err = errors.New("engine exploded")
	if _, err := r.app.CaptureText(context.Background()); err == nil || err.Error() != "text recognition: engine exploded" {
		t.Fatalf("engine error = %v", err)
	}

	eng.Err = nil
	eng.Delay = time.Second
	r.app.cfg.OCR.TimeoutSeconds = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.app.CaptureText(ctx); err == nil {
		t.Fatal("cancelled context did not fail recognition")
	}

	eng.Delay = 0
	r.cap.Result.Bytes = []byte("not a png")
	if _, err := r.app.CaptureText(context.Background()); err == nil {
		t.Fatal("undecodable capture did not fail")
	}
}

func TestCaptureTextDropsOverlappingPress(t *testing.T) {
	r := textApp(t, &ocrtest.Fake{Status: available()})
	r.app.ProbeOCR(context.Background())
	r.app.capturing.Store(true)
	out, err := r.app.CaptureText(context.Background())
	if err != nil || !out.Busy || len(r.cap.Calls) != 0 {
		t.Fatalf("out %+v err %v captures %d", out, err, len(r.cap.Calls))
	}
}
