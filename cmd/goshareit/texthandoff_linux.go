//go:build linux

package main

import (
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core/clipboard"
)

// textHandoff re-asserts text copied in the editor on the host's long-lived
// clipboard owner: X11 and Wayland selections vanish when the short-lived
// editor process exits. The text itself is never logged.
func textHandoff(cb clipboard.Clipboard) func(string) {
	return func(text string) {
		if err := cb.WriteText(text); err != nil {
			log.Warn().Err(err).Msg("ocr text copy to clipboard failed")
			return
		}
		log.Info().Int("chars", len([]rune(text))).Msg("ocr text copied")
	}
}
