//go:build linux

package linux

import (
	"os"
	"strings"
)

// IsWayland reports whether the session is Wayland. XDG_SESSION_TYPE is
// authoritative when set; otherwise WAYLAND_DISPLAY decides (the same rule the
// Wails global-shortcut backend uses, so both halves of the app agree).
func IsWayland() bool {
	switch strings.ToLower(os.Getenv("XDG_SESSION_TYPE")) {
	case "wayland":
		return true
	case "x11":
		return false
	}
	return os.Getenv("WAYLAND_DISPLAY") != ""
}
