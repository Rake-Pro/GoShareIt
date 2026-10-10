//go:build !linux

package main

import (
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core/clipboard"
)

// textHandoff is a log line here: the macOS and Windows clipboards keep the
// text the editor copied after it exits.
func textHandoff(clipboard.Clipboard) func(string) {
	return func(text string) {
		log.Debug().Int("chars", len([]rune(text))).Msg("ocr text copied in the editor")
	}
}
