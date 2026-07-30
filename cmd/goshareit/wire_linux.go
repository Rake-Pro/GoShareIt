//go:build linux

package main

import (
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/fake"
)

// buildProviders on linux returns fake OS seams so the module builds and runs.
// Real capture/clipboard/notify/tray/hotkey backends live under platform/darwin
// and platform/windows and are wired by wire_darwin.go / wire_windows.go.
func buildProviders(_ *config.Config) (core.Providers, error) {
	log.Warn().Msg("no capture backend on linux: using in-memory fakes")
	return core.Providers{
		Capturer:  fake.NewCapturer(),
		Recorder:  fake.NewRecorder(),
		Uploader:  fake.NewUploader(), // replaced by the real Nextcloud uploader in main
		Clipboard: &fake.Clipboard{},
		Notifier:  &fake.Notifier{},
		Confirmer: &fake.Confirmer{},
		Tray:      fake.Tray{},
		Hotkeys:   fake.NewHotkeyManager(),
	}, nil
}
