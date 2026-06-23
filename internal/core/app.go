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
	"github.com/Rake-Pro/GoShareIt/internal/core/edit"
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
	Recorder  capture.Recorder // optional; nil = recording unsupported
	Uploader  upload.Uploader
	Clipboard clipboard.Clipboard
	Notifier  notify.Notifier
	Tray      tray.Tray
	Hotkeys   hotkey.Manager
	Editor    edit.Editor // optional; nil -> NoopEditor
}

// App is the portable orchestrator.
type App struct {
	cfg     *config.Config
	log     zerolog.Logger
	history *history.History

	capturer  capture.Capturer
	recorder  capture.Recorder // may be nil
	uploader  upload.Uploader
	clipboard clipboard.Clipboard
	notifier  notify.Notifier
	tray      tray.Tray
	hotkeys   hotkey.Manager
	editor    edit.Editor
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
	editor := p.Editor
	if editor == nil {
		editor = edit.NoopEditor{}
	}
	return &App{
		cfg:       cfg,
		log:       log,
		history:   hist,
		capturer:  p.Capturer,
		recorder:  p.Recorder,
		uploader:  p.Uploader,
		clipboard: p.Clipboard,
		notifier:  p.Notifier,
		tray:      p.Tray,
		hotkeys:   p.Hotkeys,
		editor:    editor,
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
		Edit:            a.cfg.Editor.Enabled && modeInOnModes(mode, a.cfg.Editor.OnModes),
	}
	return a.runPipeline(ctx, req)
}

// modeInOnModes reports whether a capture mode matches one of the configured
// editor on_modes names ("region", "fullscreen", "window").
func modeInOnModes(mode capture.Mode, onModes []string) bool {
	var name string
	switch mode {
	case capture.RegionInteractive, capture.LastRegion:
		name = "region"
	case capture.FullScreen:
		name = "fullscreen"
	case capture.ActiveWindow, capture.WindowPick:
		name = "window"
	default:
		return false
	}
	for _, m := range onModes {
		if m == name {
			return true
		}
	}
	return false
}

// Recorder exposes the recorder seam (may be nil if unsupported).
func (a *App) Recorder() capture.Recorder { return a.recorder }

// RecordingSupported reports whether this build can record: a recorder is wired
// and it advertises at least one supported mode.
func (a *App) RecordingSupported() bool {
	return a.recorder != nil && len(a.recorder.Capabilities().Modes) > 0
}

// Recording reports whether a recording is currently active.
func (a *App) Recording() bool {
	return a.recorder != nil && a.recorder.Recording()
}

// RecordingModeSupported reports whether the wired recorder advertises the given
// mode (e.g. capture.VideoFull or capture.GIF).
func (a *App) RecordingModeSupported(mode capture.Mode) bool {
	if a.recorder == nil {
		return false
	}
	for _, m := range a.recorder.Capabilities().Modes {
		if m == mode {
			return true
		}
	}
	return false
}

// StartRecording begins a recording for the given mode.
func (a *App) StartRecording(ctx context.Context, mode capture.Mode) error {
	if a.recorder == nil {
		return fmt.Errorf("core: recording not supported on this build")
	}
	return a.recorder.Start(ctx, mode)
}

// StopRecording finalizes the active recording and routes the video Result
// through the same upload pipeline as screenshots.
func (a *App) StopRecording(ctx context.Context) (upload.UploadResult, error) {
	if a.recorder == nil {
		return upload.UploadResult{}, fmt.Errorf("core: recording not supported on this build")
	}
	res, err := a.recorder.Stop(ctx)
	if err != nil {
		return upload.UploadResult{}, fmt.Errorf("stop recording: %w", err)
	}
	return a.processResult(ctx, res)
}
