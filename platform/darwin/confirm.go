//go:build darwin

package darwin

import (
	"fmt"
	"os/exec"
	"strings"
)

// Confirmer shows blocking native dialogs via osascript `display dialog`.
type Confirmer struct{}

// NewConfirmer returns a macOS confirm-dialog provider.
func NewConfirmer() *Confirmer { return &Confirmer{} }

// Confirm shows a two-button dialog and blocks until the user answers, the
// dialog times out (120s, treated as "no"), or an actual error occurs.
//
// A click on a button literally named "Cancel" (or hitting Escape) makes
// osascript exit non-zero with AppleScript error -128 ("User canceled") -
// that is a normal "no" answer, not an error worth surfacing.
func (c *Confirmer) Confirm(title, body, okLabel, cancelLabel string) (bool, error) {
	script := fmt.Sprintf(
		"display dialog %s with title %s buttons {%s, %s} default button %s giving up after 120",
		osaQuote(body), osaQuote(title), osaQuote(cancelLabel), osaQuote(okLabel), osaQuote(okLabel),
	)
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "-128") {
			return false, nil
		}
		return false, fmt.Errorf("darwin confirm: osascript failed: %v: %s", err, out)
	}
	button, gaveUp, ok := parseDialogOutput(string(out))
	if !ok {
		return false, fmt.Errorf("darwin confirm: unrecognized osascript output: %s", out)
	}
	if gaveUp {
		// An ignored dialog reports gave up:true and echoes the default button
		// as the one "returned" even though osascript exits 0 - that must not
		// be read as the user actively choosing okLabel.
		return false, nil
	}
	return button == okLabel, nil
}

// parseDialogOutput parses osascript's `display dialog` stdout, of the form
// "button returned:X, gave up:false".
func parseDialogOutput(out string) (button string, gaveUp bool, ok bool) {
	const marker = "button returned:"
	idx := strings.Index(out, marker)
	if idx == -1 {
		return "", false, false
	}
	rest := out[idx+len(marker):]
	if giveIdx := strings.Index(rest, ", gave up:"); giveIdx >= 0 {
		button = rest[:giveIdx]
		gaveUp = strings.Contains(rest[giveIdx:], "gave up:true")
	} else {
		button = strings.TrimSpace(rest)
	}
	return strings.TrimRight(button, "\n"), gaveUp, true
}
