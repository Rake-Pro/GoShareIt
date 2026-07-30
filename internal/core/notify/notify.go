// Package notify defines the desktop-notification seam.
package notify

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
