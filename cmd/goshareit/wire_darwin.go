//go:build darwin

package main

import (
	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/platform/darwin"
)

// buildProviders on darwin returns the real macOS OS seams. The Uploader is left
// nil here; main.go injects the portable Nextcloud uploader from config.
func buildProviders(_ *config.Config) (core.Providers, error) {
	return core.Providers{
		Capturer:  darwin.NewCapturer(),
		Recorder:  darwin.NewRecorder(),
		Clipboard: darwin.NewClipboard(),
		Notifier:  darwin.NewNotifier(),
		Tray:      darwin.NewTray(),
		Hotkeys:   darwin.NewHotkeyManager(),
	}, nil
}
