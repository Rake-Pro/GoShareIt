//go:build windows

package main

import (
	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/gifrec"
	"github.com/Rake-Pro/GoShareIt/internal/core/region"
	"github.com/Rake-Pro/GoShareIt/platform/wailsapp"
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
	warnSmartAppControl(windows.NewChooser())

	// One Wails application owns the tray, the global shortcuts, the toast
	// notifications and the confirm dialogs; Tray.Run drives its main loop.
	ui := wailsapp.New()
	if windows.IsPackaged() {
		// MSIX declares its startup task in the package manifest and the user
		// controls it from Settings > Apps > Startup; an HKCU Run value written
		// from inside the package is virtualized and would not survive anyway.
		log.Info().Msg("start at login not reconciled: Microsoft Store build, the package manifest owns the startup task")
	} else {
		ui.ReconcileAutostart(cfg.StartAtLogin)
	}

	// One Capturer instance, shared by still capture and the frame-sampling GIF
	// recorder. The composite routes GIF -> gifrec, video -> the ffmpeg recorder.
	capturer := windows.NewCapturer()
	capturer.Region = region.Launcher{HelperPath: cfg.Editor.HelperPath}
	recorder := capture.NewCompositeRecorder(windows.NewRecorder(), gifrec.New(capturer, 0, 0))

	return core.Providers{
		Capturer:  capturer,
		Clipboard: windows.NewClipboard(),
		Notifier:  ui.Notifier(),
		Confirmer: ui.Confirmer(),
		Tray:      ui.Tray(),
		// PrintScreen chords cannot be expressed as a Wails accelerator, so
		// they keep a direct RegisterHotKey path; every other chord is a Wails
		// global shortcut.
		Hotkeys:  newPrintScreenSplit(ui.Hotkeys(), windows.NewPrintScreenHotkeys()),
		Recorder: recorder,
	}, nil
}
