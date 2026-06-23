//go:build windows

package main

import (
	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/platform/windows"
)

// buildProviders on windows returns the real Windows OS seams. The Uploader is
// left nil here; main.go injects the portable Nextcloud uploader from config.
// The struct is built with keyed fields so any provider added by a parallel
// task (e.g. a future Recorder) stays unset and the literal remains valid.
func buildProviders(_ *config.Config) (core.Providers, error) {
	return core.Providers{
		Capturer:  windows.NewCapturer(),
		Clipboard: windows.NewClipboard(),
		Notifier:  windows.NewNotifier(),
		Tray:      windows.NewTray(),
		Hotkeys:   windows.NewHotkeyManager(),
	}, nil
}
