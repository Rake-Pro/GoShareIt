package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/fake"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/ocrtest"
)

func TestTextItemState(t *testing.T) {
	title, enabled, why := textItemState(ocr.Status{Available: true}, "Capture Text  (Ctrl+Shift+8)")
	if title != "Capture Text  (Ctrl+Shift+8)" || !enabled || why != "" {
		t.Fatalf("available: %q %v %q", title, enabled, why)
	}
	title, enabled, why = textItemState(ocr.Status{Reason: "Tesseract is not installed.", Hint: "Install it with apt."}, "Capture Text")
	if title != "Capture Text (Tesseract is not installed)" || enabled || why != "Tesseract is not installed. Install it with apt." {
		t.Fatalf("unavailable: %q %v %q", title, enabled, why)
	}
	if title, _, _ = textItemState(ocr.Status{}, "Capture Text"); title != "Capture Text (not available)" {
		t.Fatalf("no reason: %q", title)
	}
	if title, enabled, why = textItemChecking(); enabled || !strings.Contains(title, "checking") || why == "" {
		t.Fatalf("checking: %q %v %q", title, enabled, why)
	}
}

func TestTextNotification(t *testing.T) {
	for _, tc := range []struct {
		out   core.TextOutcome
		title string
		body  string
		ok    bool
	}{
		{core.TextOutcome{Cancelled: true}, "", "", false},
		{core.TextOutcome{Busy: true}, "", "", false},
		{core.TextOutcome{}, "No text found", "clipboard is unchanged", true},
		{core.TextOutcome{Chars: 1, Lines: 1}, "Text copied", "Copied 1 character to", true},
		{core.TextOutcome{Chars: 128, Lines: 3}, "Text copied", "Copied 128 characters to", true},
	} {
		title, body, ok := textNotification(tc.out)
		if ok != tc.ok || title != tc.title || !strings.Contains(body, tc.body) {
			t.Errorf("%+v: got %q %q %v", tc.out, title, body, ok)
		}
	}
}

func watchApp(t *testing.T, eng ocr.Engine, ocrOn bool) *core.App {
	t.Helper()
	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.OCR.Enabled = &ocrOn
	p := core.Providers{Capturer: fake.NewCapturer(), Uploader: fake.NewUploader(), Clipboard: &fake.Clipboard{}, OCR: eng}
	app, err := core.New(cfg, p, zerolog.Nop(), hist)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// The tray item is greyed until the first probe says otherwise, and stays
// greyed (with the reason) when text recognition is off.
func TestWatchOCRAppliesProbe(t *testing.T) {
	eng := &ocrtest.Fake{Status: ocr.Status{Available: true, Engine: ocr.EngineTesseract}}
	app := watchApp(t, eng, true)
	var got []ocr.Status
	first := make(chan struct{})
	watchOCR(context.Background(), app, func(st ocr.Status) { got = append(got, st) }, first)
	<-first
	if len(got) != 1 || !got[0].Available || eng.ProbeCalls.Load() != 1 {
		t.Fatalf("applied %+v, probes %d", got, eng.ProbeCalls.Load())
	}

	eng2 := &ocrtest.Fake{Status: ocr.Status{Available: true}}
	app2 := watchApp(t, eng2, false)
	got = nil
	first2 := make(chan struct{})
	watchOCR(context.Background(), app2, func(st ocr.Status) { got = append(got, st) }, first2)
	<-first2
	if len(got) != 1 || got[0].Available || got[0].Reason != "Text recognition is turned off." || eng2.ProbeCalls.Load() != 0 {
		t.Fatalf("ocr off: applied %+v, probes %d", got, eng2.ProbeCalls.Load())
	}

	// Unavailable: greyed with the reason; the Linux re-check loop ends with
	// the context.
	eng3 := &ocrtest.Fake{Status: ocr.Status{Reason: "Tesseract is not installed."}}
	app3 := watchApp(t, eng3, true)
	got = nil
	ctx, cancel := context.WithCancel(context.Background())
	first3 := make(chan struct{})
	done := make(chan struct{})
	go func() {
		watchOCR(ctx, app3, func(st ocr.Status) { got = append(got, st) }, first3)
		close(done)
	}()
	<-first3
	cancel()
	<-done
	if len(got) != 1 || got[0].Available {
		t.Fatalf("unavailable: applied %+v", got)
	}
}
