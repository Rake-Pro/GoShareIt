// Package notify defines the desktop-notification seam.
package notify

import "net/url"

// Notification is a single desktop notification.
type Notification struct {
	Title         string
	Body          string
	ThumbnailPath string
	OpenURL       string
}

// Notifier shows desktop notifications.
type Notifier interface {
	Notify(n Notification) error
}

// Confirmer shows a native blocking yes/no dialog and reports the user's
// choice. true means the user chose okLabel.
type Confirmer interface {
	Confirm(title, body, okLabel, cancelLabel string) (bool, error)
}

// IsWebURL reports whether s is an absolute http(s) URL with a host. Links
// that come from a server (an upload response, the Nextcloud sign-in flow) are
// opened only when this holds: the OS opener behind them also launches local
// programs, file and UNC paths and arbitrary protocol handlers.
func IsWebURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}
