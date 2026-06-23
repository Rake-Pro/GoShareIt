//go:build darwin

package main

import (
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
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

	return core.Providers{
		Capturer:  darwin.NewCapturer(),
		Recorder:  darwin.NewRecorder(),
		Clipboard: darwin.NewClipboard(),
		Notifier:  darwin.NewNotifier(),
		Tray:      darwin.NewTray(),
		Hotkeys:   darwin.NewHotkeyManager(),
	}, nil
}
