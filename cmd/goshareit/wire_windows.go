//go:build windows

package main

import (
	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/gifrec"
	"github.com/Rake-Pro/GoShareIt/internal/core/region"
	"github.com/Rake-Pro/GoShareIt/platform/windows"
	"github.com/rs/zerolog/log"
)

// buildProviders on windows returns the real Windows OS seams. The Uploader is
// left nil here; main.go injects the portable Nextcloud uploader from config.
func buildProviders(cfg *config.Config) (core.Providers, error) {
	if cfg.Hotkeys.DisableSnippingPrintScreen {
		changed, err := windows.FreePrintScreen()
		switch {
		case err != nil:
			log.Warn().Err(err).Msg("could not disable Snipping Tool's PrintScreen claim")
		case changed:
			log.Info().Msg("disabled Snipping Tool's PrintScreen claim; sign out and back in if PrintScreen chords still fail to register")
		}
	}
	// One Capturer instance, shared by still capture and the frame-sampling GIF
	// recorder. The composite routes GIF -> gifrec, video -> the ffmpeg recorder.
	capturer := windows.NewCapturer()
	capturer.Region = region.Launcher{HelperPath: cfg.Editor.HelperPath}
	recorder := capture.NewCompositeRecorder(windows.NewRecorder(), gifrec.New(capturer, 0, 0))

	return core.Providers{
		Capturer:  capturer,
		Clipboard: windows.NewClipboard(),
		Notifier:  windows.NewNotifier(),
		Confirmer: windows.NewConfirmer(),
		Tray:      windows.NewTray(),
		Hotkeys:   windows.NewHotkeyManager(),
		Recorder:  recorder,
	}, nil
}
