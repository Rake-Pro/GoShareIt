//go:build windows

package windows

import (
	"context"

	"fyne.io/systray"

	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
)

// Tray implements the menu-bar (notification-area) seam via fyne.io/systray.
//
// On Windows systray is pure syscall (no cgo): it creates a hidden message
// window and a Shell_NotifyIcon tray icon, and systray.Run drives that window's
// message loop. The hotkey backend (see hotkey.go) uses its own message threads
// via RegisterHotKey, so the two do not contend for a shared run loop.
type Tray struct{}

// NewTray returns a Windows systray-backed Tray.
func NewTray() *Tray { return &Tray{} }

// Run builds the menu from spec and blocks until ctx is cancelled. On ctx.Done
// it calls systray.Quit, which unwinds systray.Run and returns.
func (t *Tray) Run(ctx context.Context, spec tray.MenuSpec) error {
	ready := func() {
		if spec.Tooltip != "" {
			systray.SetTitle(spec.Tooltip)
			systray.SetTooltip(spec.Tooltip)
		}
		for _, item := range spec.Items {
			if item.Separator {
				systray.AddSeparator()
				continue
			}
			mi := systray.AddMenuItem(item.Title, item.Title)
			if item.OnClick != nil {
				go func(ch <-chan struct{}, fn func()) {
					for {
						select {
						case <-ctx.Done():
							return
						case _, ok := <-ch:
							if !ok {
								return
							}
							fn()
						}
					}
				}(mi.ClickedCh, item.OnClick)
			}
		}

		// Quit the systray (and thus its message loop) when ctx is cancelled.
		go func() {
			<-ctx.Done()
			systray.Quit()
		}()
	}

	systray.Run(ready, func() {})
	return nil
}
