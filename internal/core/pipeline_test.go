package core

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/fake"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
)

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
