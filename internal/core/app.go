// Package core wires the portable orchestration of GoShareIt. It depends only
// on interface seams; concrete OS providers are injected by the cmd layer.
package core

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/clipboard"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/hotkey"
	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

// Providers bundles the OS-specific implementations the core depends on. The
// cmd layer constructs these per-GOOS and hands them to New.
type Providers struct {
	Capturer  capture.Capturer
	Uploader  upload.Uploader
	Clipboard clipboard.Clipboard
	Notifier  notify.Notifier
	Tray      tray.Tray
	Hotkeys   hotkey.Manager
}

// App is the portable orchestrator.
type App struct {
	cfg     *config.Config
	log     zerolog.Logger
	history *history.History

	capturer  capture.Capturer
	uploader  upload.Uploader
	clipboard clipboard.Clipboard
	notifier  notify.Notifier
	tray      tray.Tray
	hotkeys   hotkey.Manager
}

// New constructs an App from config, providers, a logger and a history store.
func New(cfg *config.Config, p Providers, log zerolog.Logger, hist *history.History) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("core: nil config")
	}
	if p.Capturer == nil || p.Uploader == nil || p.Clipboard == nil {
		return nil, fmt.Errorf("core: capturer, uploader and clipboard providers are required")
	}
	if hist == nil {
		return nil, fmt.Errorf("core: nil history")
	}
	return &App{
		cfg:       cfg,
		log:       log,
		history:   hist,
		capturer:  p.Capturer,
		uploader:  p.Uploader,
		clipboard: p.Clipboard,
		notifier:  p.Notifier,
		tray:      p.Tray,
		hotkeys:   p.Hotkeys,
	}, nil
}

// Config exposes the loaded config (read-only use).
func (a *App) Config() *config.Config { return a.cfg }

// History exposes the history store.
func (a *App) History() *history.History { return a.history }

// Hotkeys exposes the hotkey manager (may be nil).
func (a *App) Hotkeys() hotkey.Manager { return a.hotkeys }

// Tray exposes the tray provider (may be nil).
func (a *App) Tray() tray.Tray { return a.tray }

// RunCapture drives the full pipeline for the given mode.
func (a *App) RunCapture(ctx context.Context, mode capture.Mode) (upload.UploadResult, error) {
	req := capture.Request{
		Mode:            mode,
		CopyToClipboard: a.cfg.AfterCapture.CopyImageToClipboard,
		SaveLocal:       a.cfg.AfterCapture.SaveLocal,
		SaveDir:         a.cfg.AfterCapture.SaveDir,
	}
	return a.runPipeline(ctx, req)
}
