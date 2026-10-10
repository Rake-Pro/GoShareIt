//go:build linux && cgo

package main

import (
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/gifrec"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/engines"
	"github.com/Rake-Pro/GoShareIt/internal/core/region"
	"github.com/Rake-Pro/GoShareIt/platform/linux"
	"github.com/Rake-Pro/GoShareIt/platform/wailsapp"
)

// buildProviders on linux returns the real desktop seams. The Uploader is
// left nil here; main.go injects the portable uploader from config.
func buildProviders(cfg *config.Config) (core.Providers, error) {
	wayland := linux.IsWayland()
	log.Info().Bool("wayland", wayland).Msg("linux session")

	// One Wails application owns the tray (StatusNotifierItem), the global
	// shortcuts (XGrabKey on X11, the GlobalShortcuts portal on Wayland), the
	// D-Bus notifications and the GTK confirm dialogs; Tray.Run drives its
	// main loop.
	ui := wailsapp.New()
	ui.ReconcileAutostart(cfg.StartAtLogin)

	capturer := linux.NewCapturer()
	capturer.Region = region.Launcher{HelperPath: cfg.Editor.HelperPath}

	// Recording is X11-only: ffmpeg x11grab for video, frame sampling of the
	// capturer for GIF. On Wayland every sampled frame would be a portal
	// round-trip, so no recorder is wired and the host hides the items.
	var recorder capture.Recorder
	if wayland {
		log.Info().Msg("screen recording is not available on Wayland sessions")
	} else {
		recorder = capture.NewCompositeRecorder(linux.NewRecorder(), gifrec.New(capturer, 0, 0))
	}

	return core.Providers{
		Capturer:  capturer,
		Recorder:  recorder,
		Clipboard: linux.NewClipboard(),
		Notifier:  ui.Notifier(),
		Confirmer: ui.Confirmer(),
		Tray:      ui.Tray(),
		Hotkeys:   ui.Hotkeys(),
		OCR:       engines.New(engines.Config{Langs: cfg.OCR.Languages, TesseractPath: config.ExpandHome(cfg.OCR.TesseractPath)}),
	}, nil
}
