//go:build windows

package main

import (
	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/gifrec"
	"github.com/Rake-Pro/GoShareIt/platform/windows"
)

// buildProviders on windows returns the real Windows OS seams. The Uploader is
// left nil here; main.go injects the portable Nextcloud uploader from config.
func buildProviders(_ *config.Config) (core.Providers, error) {
	// One Capturer instance, shared by still capture and the frame-sampling GIF
	// recorder. The composite routes GIF -> gifrec, video -> the ffmpeg recorder.
	capturer := windows.NewCapturer()
	recorder := capture.NewCompositeRecorder(windows.NewRecorder(), gifrec.New(capturer, 0, 0))

	return core.Providers{
		Capturer:  capturer,
		Clipboard: windows.NewClipboard(),
		Notifier:  windows.NewNotifier(),
		Tray:      windows.NewTray(),
		Hotkeys:   windows.NewHotkeyManager(),
		Recorder:  recorder,
	}, nil
}
