//go:build windows

package windows

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
)

// Notifier posts native Windows toast notifications via PowerShell, the Windows
// analog of darwin's osascript. It shells out to a short script that builds a
// ToastNotification from the WinRT API and shows it for the running app.
//
// P2 limitations (mirroring darwin's Notify):
//   - OpenURL is ignored: wiring a click-activation handler requires a
//     registered AppUserModelID / COM activator, out of scope for P2. The URL is
//     already placed on the clipboard by the pipeline.
//   - ThumbnailPath is ignored: inline toast images need a packaged app identity
//     to reference local files reliably, out of scope for P2.
//
// The call is non-blocking (no modal dialog); PowerShell returns immediately
// after queuing the toast.
type Notifier struct{}

// NewNotifier returns a Windows notifier.
func NewNotifier() *Notifier { return &Notifier{} }

// Notify shows a non-blocking toast banner.
func (n *Notifier) Notify(notification notify.Notification) error {
	script := buildToastScript(notification.Title, notification.Body)
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", script,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("windows notify: powershell failed: %v: %s", err, out)
	}
	return nil
}

// buildToastScript returns a PowerShell snippet that displays a toast with the
// given title and body. Title/body are escaped for the XML text nodes and for
// the PowerShell single-quoted string literal.
func buildToastScript(title, body string) string {
	appID := "GoShareIt"
	xml := fmt.Sprintf(
		`<toast><visual><binding template="ToastGeneric"><text>%s</text><text>%s</text></binding></visual></toast>`,
		xmlEscape(title), xmlEscape(body),
	)
	return strings.Join([]string{
		`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null`,
		`[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null`,
		`$xml = New-Object Windows.Data.Xml.Dom.XmlDocument`,
		`$xml.LoadXml('` + psEscape(xml) + `')`,
		`$toast = New-Object Windows.UI.Notifications.ToastNotification $xml`,
		`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('` + psEscape(appID) + `').Show($toast)`,
	}, "; ")
}

// xmlEscape escapes the five XML predefined entities so titles/bodies cannot
// break out of the toast XML text nodes.
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// psEscape escapes a string for embedding inside a PowerShell single-quoted
// literal (only the single quote needs doubling).
func psEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
