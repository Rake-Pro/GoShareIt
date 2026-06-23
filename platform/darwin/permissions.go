//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics -framework IOKit
#include <stdbool.h>

bool gsi_request_accessibility(void);
bool gsi_request_screen_capture(void);
bool gsi_request_input_monitoring(void);
*/
import "C"

// Permissions reports which macOS TCC permissions the app currently holds.
type Permissions struct {
	Accessibility   bool // AXIsProcessTrusted - required for the global-hotkey event tap
	ScreenRecording bool // required to capture screen content
	InputMonitoring bool // required by the CGEventTap that delivers hotkeys
}

// RequestPermissions proactively triggers the macOS permission prompts the app
// needs and reports the resulting grant state. It is best-effort: when a
// permission is undetermined the OS shows a prompt (attributed to this app, so a
// signed .app gets a stable entry); when it was previously denied macOS does not
// re-prompt and the user must clear the entry (tccutil reset ...) or toggle it in
// System Settings. Calling this early, before hotkey/capture use, means the user
// is asked up front rather than hitting a silent failure.
func RequestPermissions() Permissions {
	return Permissions{
		Accessibility:   bool(C.gsi_request_accessibility()),
		ScreenRecording: bool(C.gsi_request_screen_capture()),
		InputMonitoring: bool(C.gsi_request_input_monitoring()),
	}
}
