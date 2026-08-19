//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdbool.h>
#include <stdlib.h>

bool gsi_notify_available(void);
int gsi_notify(const char *title, const char *body);
*/
import "C"

import (
	"fmt"
	"os/exec"
	"strings"
	"unsafe"

	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
)

// Notifier posts native macOS notifications.
type Notifier struct{}

// NewNotifier returns a macOS notifier.
func NewNotifier() *Notifier { return &Notifier{} }

// Notify shows a non-blocking banner.
//
// Bundled (.app) builds go through UNUserNotificationCenter (notify.m), so the
// banner is attributed to GoShareIt and carries the app icon. Unbundled dev
// binaries cannot use that API (it requires a bundle identifier) and fall back
// to osascript, whose banner shows the Script Editor icon.
//
// P1 limitations:
//   - OpenURL is ignored: no click action is wired up and we deliberately avoid
//     blocking modal dialogs. The URL is already placed on the clipboard by the
//     pipeline, so this is acceptable for the MVP.
//   - ThumbnailPath is ignored: attachment images need a UNNotification content
//     extension, out of scope for P1.
func (n *Notifier) Notify(notification notify.Notification) error {
	if !bool(C.gsi_notify_available()) {
		return n.notifyOsascript(notification)
	}
	ctitle := C.CString(notification.Title)
	cbody := C.CString(notification.Body)
	defer C.free(unsafe.Pointer(ctitle))
	defer C.free(unsafe.Pointer(cbody))
	switch rc := C.gsi_notify(ctitle, cbody); rc {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("darwin notify: not authorized - allow GoShareIt in System Settings > Notifications")
	case 3:
		// Notification center threw (unregistered/odd launch context, e.g.
		// running from a DMG) - the osascript path still works there.
		return n.notifyOsascript(notification)
	case 4:
		return fmt.Errorf("darwin notify: authorization prompt unanswered - this notification was dropped")
	default:
		return fmt.Errorf("darwin notify: notification center rejected the request (code %d)", int(rc))
	}
}

// notifyOsascript is the unbundled-dev fallback via `display notification`.
func (n *Notifier) notifyOsascript(notification notify.Notification) error {
	script := fmt.Sprintf(
		"display notification %s with title %s",
		osaQuote(notification.Body),
		osaQuote(notification.Title),
	)
	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("darwin notify: osascript failed: %v: %s", err, out)
	}
	return nil
}

// osaQuote wraps s as an AppleScript string literal, escaping backslashes and
// double quotes so arbitrary titles/bodies cannot break out of the literal.
func osaQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
