//go:build darwin

package darwin

import (
	"context"

	"fyne.io/systray"

	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
)

// Tray implements the menu-bar seam via fyne.io/systray.
//
// MAIN-LOOP OWNERSHIP: systray.Run installs and owns the Cocoa NSApp main run
// loop (it must run on the main OS thread, which runtime.LockOSThread + the Go
// runtime keep as the process's first thread). This makes systray the single
// main-loop owner in the process. The Carbon/CGEventTap hotkey backend (see
// hotkey.go) attaches its event source to CFRunLoopGetMain(), so hotkey events
// are delivered by this very run loop; the two libraries coexist by sharing it.
// systray needs cgo and a proper .app bundle to actually appear in the menu bar.
type Tray struct{}

// NewTray returns a macOS systray-backed Tray.
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

		// Quit the systray (and thus the main run loop) when ctx is cancelled.
		go func() {
			<-ctx.Done()
			systray.Quit()
		}()
	}

	systray.Run(ready, func() {})
	return nil
}
