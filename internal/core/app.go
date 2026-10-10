// Package core wires the portable orchestration of GoShareIt. It depends only
// on interface seams; concrete OS providers are injected by the cmd layer.
package core

import (
	"context"
	"fmt"
	"image"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/clipboard"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/edit"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/hotkey"
	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
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
	Confirmer notify.Confirmer // optional; nil = no blocking confirm dialogs (e.g. linux/dev)
	Tray      tray.Tray
	Hotkeys   hotkey.Manager
	Editor    edit.Editor // optional; nil -> no edit step
	OCR       ocr.Engine  // optional; nil = ocr.Unavailable
}

// App is the portable orchestrator.
type App struct {
	cfg     *config.Config
	log     zerolog.Logger
	history *history.History

	// uploadEnabled is the live upload switch: seeded from config, flippable
	// at runtime (hotkey/tray). Atomic because captures read it from hotkey
	// goroutines while the toggle writes it.
	uploadEnabled atomic.Bool

	// capturing is set while a one-shot capture is on screen (overlay, picker
	// or editor); presses that arrive meanwhile are dropped. It is released
	// before the upload, so a slow upload never swallows the next press.
	capturing atomic.Bool

	capturer  capture.Capturer
	recorder  capture.Recorder // may be nil
	uploader  upload.Uploader
	clipboard clipboard.Clipboard
	notifier  notify.Notifier
	confirmer notify.Confirmer
	tray      tray.Tray
	hotkeys   hotkey.Manager
	editor    edit.Editor // may be nil: no edit step

	// ocr is the text-recognition engine; ocrStatus caches its last probe
	// (nil until ProbeOCR stores one). reprobeOCR re-probes before an editor
	// launch while the cached status is unavailable (Linux: tesseract can be
	// installed while the host runs).
	ocr        ocr.Engine
	ocrStatus  atomic.Pointer[ocr.Status]
	reprobeOCR bool
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
	a := &App{
		cfg:       cfg,
		log:       log,
		history:   hist,
		capturer:  p.Capturer,
		recorder:  p.Recorder,
		uploader:  p.Uploader,
		clipboard: p.Clipboard,
		notifier:  p.Notifier,
		confirmer: p.Confirmer,
		tray:      p.Tray,
		hotkeys:   p.Hotkeys,
		editor:    p.Editor,
		ocr:       p.OCR,

		reprobeOCR: runtime.GOOS == "linux",
	}
	if a.ocr == nil {
		a.ocr = ocr.Unavailable{Why: "Text recognition is not available in this build."}
	}
	a.uploadEnabled.Store(cfg.UploadEnabled())
	return a, nil
}

// ocrProbeTimeout bounds one availability probe.
const ocrProbeTimeout = 3 * time.Second

// ProbeOCR asks the engine whether text recognition is available and caches
// the answer for OCRStatus and the editor. The host runs it once at start in
// a goroutine; a probe that does not finish in 3 s is stored as unavailable.
func (a *App) ProbeOCR(ctx context.Context) ocr.Status {
	ctx, cancel := context.WithTimeout(ctx, ocrProbeTimeout)
	defer cancel()
	done := make(chan ocr.Status, 1)
	go func() { done <- a.ocr.Probe(ctx) }()
	var st ocr.Status
	select {
	case st = <-done:
	case <-ctx.Done():
		st = ocr.Status{Reason: "Text recognition did not answer in time."}
	}
	prev := a.ocrStatus.Swap(&st)
	// Info for the first result and for changes; the Linux re-probe before
	// each edit capture would otherwise log the same line every time.
	ev := a.log.Info()
	if prev != nil && sameStatus(*prev, st) {
		ev = a.log.Debug()
	}
	ev.Bool("available", st.Available).Str("engine", st.Engine).Str("version", st.Version).
		Strs("langs", st.Langs).Str("reason", st.Reason).Msg("ocr probe")
	return st
}

func sameStatus(a, b ocr.Status) bool {
	return a.Available == b.Available && a.Engine == b.Engine && a.Version == b.Version &&
		a.Reason == b.Reason && a.Hint == b.Hint && slices.Equal(a.Langs, b.Langs)
}

// OCRStatus returns the cached probe result; before the first probe has
// finished it reports text recognition as still starting.
func (a *App) OCRStatus() ocr.Status {
	if st := a.ocrStatus.Load(); st != nil {
		return *st
	}
	return ocr.Status{Reason: "Text recognition is still starting."}
}

// OCREngine exposes the text-recognition engine (never nil).
func (a *App) OCREngine() ocr.Engine { return a.ocr }

// ocrStatusForEditor is the status handed to the editor. Where reprobeOCR
// is set (Linux) and the cached status is unavailable, it probes again
// first, so a user who just installed tesseract gets the tool on the next
// capture without a restart; the tesseract engine makes that a LookPath
// unless the command appeared.
func (a *App) ocrStatusForEditor(ctx context.Context) ocr.Status {
	st := a.OCRStatus()
	if st.Available || !a.reprobeOCR || a.ocrStatus.Load() == nil {
		return st
	}
	return a.ProbeOCR(ctx)
}

// UploadEnabled reports the live upload switch.
func (a *App) UploadEnabled() bool { return a.uploadEnabled.Load() }

// SetUploadEnabled flips the live upload switch (persistence is the caller's
// concern - see the cmd layer's toggle handler).
func (a *App) SetUploadEnabled(v bool) { a.uploadEnabled.Store(v) }

// UploadConfigured reports whether the active upload destination (Nextcloud,
// S3, SFTP, WebDAV, Custom or a public host) carries everything an upload
// needs; the toggle refuses to enable uploads without it.
func (a *App) UploadConfigured() bool {
	return a.cfg.UploadReady() == nil
}

// Config exposes the loaded config (read-only use).
func (a *App) Config() *config.Config { return a.cfg }

// History exposes the history store.
func (a *App) History() *history.History { return a.history }

// Hotkeys exposes the hotkey manager (may be nil).
func (a *App) Hotkeys() hotkey.Manager { return a.hotkeys }

// Tray exposes the tray provider (may be nil).
func (a *App) Tray() tray.Tray { return a.tray }

// Notifier exposes the notifier (may be nil).
func (a *App) Notifier() notify.Notifier { return a.notifier }

// Confirmer exposes the confirm-dialog provider (may be nil).
func (a *App) Confirmer() notify.Confirmer { return a.confirmer }

// RunCapture drives the full pipeline for the given mode.
func (a *App) RunCapture(ctx context.Context, mode capture.Mode) (upload.UploadResult, error) {
	return a.runCapture(ctx, mode, a.cfg.Editor.Enabled && modeInOnModes(mode, a.cfg.Editor.OnModes))
}

// RunCaptureEdit is RunCapture with the annotation editor forced on,
// independent of editor.enabled/on_modes (the *_edit hotkey variants).
func (a *App) RunCaptureEdit(ctx context.Context, mode capture.Mode) (upload.UploadResult, error) {
	return a.runCapture(ctx, mode, true)
}

func (a *App) runCapture(ctx context.Context, mode capture.Mode, edit bool) (upload.UploadResult, error) {
	req := capture.Request{
		Mode:            mode,
		CopyToClipboard: a.cfg.AfterCapture.CopyImageToClipboard,
		// When the editor will run, the capturer must not write the
		// unedited original to disk: the pipeline saves the confirmed
		// (edited) image itself, and a cancel must leave nothing behind.
		SaveLocal: a.cfg.AfterCapture.SaveLocal && !edit,
		SaveDir:   a.cfg.AfterCapture.SaveDir,
		Edit:      edit,
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

// StartRecording begins a recording for the given mode. When region is non-empty
// and the wired recorder implements capture.RegionRecorder, the recording is
// cropped to that screen-pixel rectangle; otherwise it records full screen.
func (a *App) StartRecording(ctx context.Context, mode capture.Mode, region image.Rectangle) error {
	if a.recorder == nil {
		return fmt.Errorf("core: recording not supported on this build")
	}
	if !region.Empty() {
		if rr, ok := a.recorder.(capture.RegionRecorder); ok {
			return rr.StartRegion(ctx, mode, region)
		}
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
	return a.processResult(ctx, res, edit.ActionDefault)
}
