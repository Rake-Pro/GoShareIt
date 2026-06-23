// Package tray defines the system-tray / menu-bar seam.
package tray

import "context"

// MenuItem is one entry in the tray menu. A Separator item is a divider.
type MenuItem struct {
	ID        string
	Title     string
	OnClick   func()
	Separator bool
}

// MenuSpec describes the tray menu to display.
type MenuSpec struct {
	Tooltip string
	Items   []MenuItem
}

// Tray runs the system tray until ctx is cancelled.
type Tray interface {
	Run(ctx context.Context, spec MenuSpec) error
}
