//go:build darwin

package main

import (
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/gifrec"
	"github.com/Rake-Pro/GoShareIt/platform/darwin"
	"github.com/Rake-Pro/GoShareIt/platform/wailsapp"
)

// buildProviders on darwin returns the real macOS OS seams. The Uploader is left
// nil here; main.go injects the portable Nextcloud uploader from config.
func buildProviders(cfg *config.Config) (core.Providers, error) {
	// Request Screen Recording up front so the user gets a prompt instead of a
	// silent capture failure. Best-effort; a denied state is logged. Hotkeys no
	// longer appear here: the Wails global-shortcut backend uses Carbon's
	// RegisterEventHotKey, which needs neither Accessibility nor Input
	// Monitoring.
	p := darwin.RequestPermissions()
	log.Info().Bool("screen_recording", p.ScreenRecording).Msg("macOS permissions")

	// One Wails application owns the menu bar, the global shortcuts, the
	// notifications and the confirm dialogs; Tray.Run drives its main loop.
	ui := wailsapp.New()
	ui.ReconcileAutostart(cfg.StartAtLogin)

	// One Capturer instance, shared by still capture and the frame-sampling GIF
	// recorder. The composite routes GIF -> gifrec, video -> the AVFoundation
	// recorder, so Capabilities advertises both.
	capturer := darwin.NewCapturer()
	recorder := capture.NewCompositeRecorder(darwin.NewRecorder(), gifrec.New(capturer, 0, 0))

	return core.Providers{
		Capturer:  capturer,
		Recorder:  recorder,
		Clipboard: darwin.NewClipboard(),
		Notifier:  ui.Notifier(),
		Confirmer: ui.Confirmer(),
		Tray:      ui.Tray(),
		Hotkeys:   ui.Hotkeys(),
	}, nil
}
