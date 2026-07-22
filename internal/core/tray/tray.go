// Package tray defines the system-tray / menu-bar seam.
package tray

import "context"

// MenuItem is one entry in the tray menu. A Separator item is a divider.
type MenuItem struct {
	ID        string
	Title     string
	OnClick   func()
	Separator bool
	Disabled  bool // initial state: true => greyed out (still present) at startup
}

// MenuSpec describes the tray menu to display.
type MenuSpec struct {
	Tooltip string
	// Icon holds platform-appropriate icon bytes: a black+alpha template PNG on
	// darwin (adaptive menu bar rendering), an ICO on windows. Empty -> the
	// tray falls back to a text title.
	Icon  []byte
	Items []MenuItem
}

// Tray runs the system tray until ctx is cancelled.
type Tray interface {
	Run(ctx context.Context, spec MenuSpec) error
	// SetItemEnabled enables or greys out the item with the given ID at runtime.
	// No-op if the item does not exist (e.g. before the tray is built).
	SetItemEnabled(id string, enabled bool)
	// SetItemTitle updates an item's label at runtime. No-op if the item is absent.
	SetItemTitle(id, title string)
}
