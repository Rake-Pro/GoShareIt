//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics
#include <stdbool.h>

bool gsi_request_screen_capture(void);
*/
import "C"

// Permissions reports which macOS TCC permissions the app currently holds.
//
// Only Screen Recording is left: global hotkeys used to run on a CGEventTap,
// which needed Accessibility and Input Monitoring, and now go through the
// Wails backend's Carbon RegisterEventHotKey, which needs neither.
type Permissions struct {
	ScreenRecording bool // required to capture screen content
}

// RequestPermissions proactively triggers the macOS permission prompt the app
// needs and reports the resulting grant state. It is best-effort: when the
// permission is undetermined the OS shows a prompt (attributed to this app, so a
// signed .app gets a stable entry); when it was previously denied macOS does not
// re-prompt and the user must clear the entry (tccutil reset ...) or toggle it in
// System Settings. Calling this early, before capture, means the user is asked up
// front rather than hitting a silent failure.
func RequestPermissions() Permissions {
	return Permissions{
		ScreenRecording: bool(C.gsi_request_screen_capture()),
	}
}
