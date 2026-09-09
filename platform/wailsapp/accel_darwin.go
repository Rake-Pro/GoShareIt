//go:build darwin

package wailsapp

// modifierAccel maps a chord modifier token to its Wails spelling. On macOS
// Cmd is the primary modifier, so cmd/command/super/meta/win all resolve to it
// - a config shared with a Windows machine then binds the same chord on both.
func modifierAccel(token string) (string, bool) {
	switch token {
	case "cmd", "command", "super", "meta", "win":
		return "cmd", true
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
		// PC keyboards deliver PrintScreen as F13 on macOS, so the same chord
		// string works across a shared config.
		return "f13", nil
	}
	return commonKeyAccel(token)
}
