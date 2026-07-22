//go:build darwin

package darwin

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
)

// Notifier posts native macOS notifications via osascript.
type Notifier struct{}

// NewNotifier returns a macOS notifier.
func NewNotifier() *Notifier { return &Notifier{} }

// Notify shows a non-blocking banner via `display notification`.
//
// P1 limitations:
//   - OpenURL is ignored: `display notification` has no click action and we
//     deliberately avoid blocking modal dialogs. The URL is already placed on
//     the clipboard by the pipeline, so this is acceptable for the MVP.
//   - ThumbnailPath is ignored: notification images require a bundled .app with
//     a UNNotification content extension, out of scope for P1.
func (n *Notifier) Notify(notification notify.Notification) error {
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
