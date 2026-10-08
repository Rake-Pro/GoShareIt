//go:build linux && cgo

package wailsapp

import "fmt"

// modifierAccel maps a chord modifier token to its Wails spelling.
//
// MODIFIER MAPPING: as on Windows, macOS-style "Cmd" chords map to Wails'
// "cmd" (CmdOrCtrl, which resolves to Control on Linux), so a config written
// on a Mac keeps working. "Super"/"Win"/"Meta" map to the Super (logo) key,
// which most Linux desktops leave free for user chords.
func modifierAccel(token string) (string, bool) {
	switch token {
	case "cmd", "command":
		return "cmd", true
	case "super", "win", "meta":
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

// keyAccel maps a chord key token to a Wails accelerator key. The Wails Linux
// key table has no entry for the Print key, so PrintScreen chords are refused
// here with a clear message rather than failing inside the X11/portal backend.
func keyAccel(token string) (string, error) {
	switch token {
	case "printscreen", "prtsc", "prtscn", "snapshot", "print":
		return "", fmt.Errorf("PrintScreen cannot be bound as a global shortcut on Linux: the Wails key table has no name for it, so pick another chord")
	}
	return commonKeyAccel(token)
}
