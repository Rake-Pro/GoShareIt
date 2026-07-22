//go:build darwin

package main

import (
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/gifrec"
	"github.com/Rake-Pro/GoShareIt/platform/darwin"
)

// buildProviders on darwin returns the real macOS OS seams. The Uploader is left
// nil here; main.go injects the portable Nextcloud uploader from config.
func buildProviders(_ *config.Config) (core.Providers, error) {
	// Request the macOS permissions up front so the user gets prompts instead of
	// silent hotkey/capture failures. Best-effort; a denied state is logged.
	p := darwin.RequestPermissions()
	log.Info().
		Bool("accessibility", p.Accessibility).
		Bool("screen_recording", p.ScreenRecording).
		Bool("input_monitoring", p.InputMonitoring).
		Msg("macOS permissions")

	// One Capturer instance, shared by still capture and the frame-sampling GIF
	// recorder. The composite routes GIF -> gifrec, video -> the AVFoundation
	// recorder, so Capabilities advertises both.
	capturer := darwin.NewCapturer()
	recorder := capture.NewCompositeRecorder(darwin.NewRecorder(), gifrec.New(capturer, 0, 0))

	return core.Providers{
		Capturer:  capturer,
		Recorder:  recorder,
		Clipboard: darwin.NewClipboard(),
		Notifier:  darwin.NewNotifier(),
		Tray:      darwin.NewTray(),
		Hotkeys:   darwin.NewHotkeyManager(),
	}, nil
}
