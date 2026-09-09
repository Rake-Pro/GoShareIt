//go:build darwin || windows

package wailsapp

import (
	"context"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
)

// Tray implements the menu-bar / notification-area seam on the Wails system
// tray. Run owns the process main loop (see the package doc).
type Tray struct {
	p *Provider

	mu    sync.Mutex
	items map[string]*application.MenuItem
}

// OnShutdown registers a function Wails runs while the application tears down,
// on the main thread, before the process goes away.
//
// This is the only shutdown seam that fires on macOS: Quit there is
// [NSApp terminate:], the delegate's applicationShouldTerminate: runs the
// shutdown tasks and the process then exits, so app.Run never returns and no
// code after it can run. On Windows the same tasks run and then Run does
// return, so a caller that also does work after Run must make it idempotent.
func (t *Tray) OnShutdown(fn func()) { t.p.app.OnShutdown(fn) }

// Run builds the tray from spec and blocks in app.Run() until ctx is
// cancelled, at which point app.Quit unwinds the main loop. It must be called
// from the main goroutine.
func (t *Tray) Run(ctx context.Context, spec tray.MenuSpec) error {
	systemTray := t.p.app.SystemTray.New()
	switch {
	case len(spec.Icon) > 0:
		if runtime.GOOS == "darwin" {
			// Template icon: black+alpha, macOS adapts it to the menu-bar theme.
			systemTray.SetTemplateIcon(spec.Icon)
		} else {
			systemTray.SetIcon(spec.Icon)
		}
		systemTray.SetTooltip(spec.Tooltip)
	case spec.Tooltip != "":
		systemTray.SetLabel(spec.Tooltip)
		systemTray.SetTooltip(spec.Tooltip)
	}

	menu := t.p.app.NewMenu()
	for _, item := range spec.Items {
		if item.Separator {
			menu.AddSeparator()
			continue
		}
		menuItem := menu.Add(item.Title)
		if item.Disabled {
			menuItem.SetEnabled(false)
		}
		if item.OnClick != nil {
			onClick := item.OnClick
			// Wails already runs every menu callback on its own goroutine, so
			// a slow handler (a capture, a modal dialog) cannot stall the loop.
			menuItem.OnClick(func(*application.Context) { onClick() })
		}
		if item.ID != "" {
			t.mu.Lock()
			t.items[item.ID] = menuItem
			t.mu.Unlock()
		}
	}
	systemTray.SetMenu(menu)

	go func() {
		<-ctx.Done()
		// Quit is a no-op before the platform application exists, so a
		// cancellation that lands during startup must wait for it, or the main
		// loop would come up with nothing left to stop it.
		t.p.waitStarted(startupWait)
		t.p.app.Quit()
	}()

	return t.p.app.Run()
}

// SetItemEnabled enables or greys out a menu item by ID. Safe to call from any
// goroutine; no-op if the item does not exist.
func (t *Tray) SetItemEnabled(id string, enabled bool) {
	if menuItem := t.item(id); menuItem != nil {
		t.p.onMainThread(func() { menuItem.SetEnabled(enabled) })
	}
}

// SetItemTitle updates a menu item's label by ID. No-op if absent.
func (t *Tray) SetItemTitle(id, title string) {
	if menuItem := t.item(id); menuItem != nil {
		t.p.onMainThread(func() { menuItem.SetLabel(title) })
	}
}

func (t *Tray) item(id string) *application.MenuItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.items[id]
}
