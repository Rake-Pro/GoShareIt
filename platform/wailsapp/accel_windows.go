//go:build windows

package wailsapp

import "fmt"

// modifierAccel maps a chord modifier token to its Wails spelling.
//
// MODIFIER MAPPING: macOS configs use "Cmd" as the primary modifier. On
// Windows the equivalent is Ctrl, and the Windows logo key reserves most
// Win+<key> combos at the shell level, so cmd/command/super/meta map to Wails'
// "cmd" (CmdOrCtrl, which resolves to Control here) rather than the logo key.
// "Win" still maps to the logo key ("super" in Wails' spelling) for users who
// explicitly want it.
func modifierAccel(token string) (string, bool) {
	switch token {
	case "cmd", "command", "super", "meta":
		return "cmd", true
	case "win":
		return "super", true
	case "ctrl", "control":
		return "ctrl", true
	case "option", "opt", "alt":
		return "alt", true
	case "shift":
		return "shift", true
	}
	return "", false
}

// keyAccel maps a chord key token to a Wails accelerator key.
func keyAccel(token string) (string, error) {
	switch token {
	case "printscreen", "prtsc", "prtscn", "snapshot":
		return "", fmt.Errorf("PrintScreen cannot be bound as a global shortcut: the Wails accelerator grammar has no name for it, so pick another chord")
	}
	return commonKeyAccel(token)
}
