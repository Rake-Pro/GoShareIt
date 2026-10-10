//go:build linux && !cgo

package main

import (
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/fake"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/engines"
)

// buildProviders in a CGO-off linux build returns in-memory fakes so the
// portable core keeps building and running (the CI core job, cross-builds).
// The real Linux shell needs cgo for its Wails and Gio halves; see
// wire_linux.go.
func buildProviders(cfg *config.Config) (core.Providers, error) {
	log.Warn().Msg("CGO-off linux build: no desktop backend, using in-memory fakes")
	return core.Providers{
		Capturer:  fake.NewCapturer(),
		Recorder:  fake.NewRecorder(),
		Uploader:  fake.NewUploader(), // replaced by the real uploader in main
		Clipboard: &fake.Clipboard{},
		Notifier:  &fake.Notifier{},
		Confirmer: &fake.Confirmer{},
		Tray:      fake.Tray{},
		Hotkeys:   fake.NewHotkeyManager(),
		// tesseract is pure Go, so this build still reports a truthful
		// text-recognition status.
		OCR: engines.New(engines.Config{Langs: cfg.OCR.Languages, TesseractPath: config.ExpandHome(cfg.OCR.TesseractPath)}),
	}, nil
}
