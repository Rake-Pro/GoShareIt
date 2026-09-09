//go:build darwin || windows

package wailsapp

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/rs/zerolog/log"
)

// Manager implements the global-hotkey seam on the Wails global-shortcut
// manager (Carbon RegisterEventHotKey on macOS - which needs no Accessibility
// or Input Monitoring grant - and Win32 RegisterHotKey on Windows).
//
// Registration is accepted before the application starts: Wails queues those
// bindings and hands them to the OS when the main loop comes up, so a chord the
// OS refuses is reported through the application error handler rather than
// returned from Register. Run therefore has nothing to do but wait for
// shutdown; Wails releases every binding as the app tears down.
type Manager struct {
	p *Provider

	mu     sync.Mutex
	accels map[string]string // binding id -> Wails accelerator
}

// Register translates a GoShareIt chord ("Cmd+Shift+1") into a Wails
// accelerator and binds it. Registration is best-effort per chord: the caller
// logs the error and carries on with the remaining bindings.
func (m *Manager) Register(id, keys string, fn func()) error {
	accel, err := toAccelerator(keys)
	if err != nil {
		return fmt.Errorf("%s hotkey: %q: %w", runtime.GOOS, keys, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.accels[id]; ok {
		return fmt.Errorf("%s hotkey: %q already registered", runtime.GOOS, id)
	}
	if err := m.p.app.GlobalShortcut.Register(accel, fn); err != nil {
		return fmt.Errorf("%s hotkey: %q: %w", runtime.GOOS, keys, err)
	}
	m.accels[id] = accel
	return nil
}

// Unregister releases a binding and forgets it. No-op for an unknown id.
func (m *Manager) Unregister(id string) {
	m.mu.Lock()
	accel, ok := m.accels[id]
	delete(m.accels, id)
	m.mu.Unlock()
	if !ok {
		return
	}
	if err := m.p.app.GlobalShortcut.Unregister(accel); err != nil {
		log.Debug().Err(err).Str("id", id).Msg("unregister hotkey")
	}
}

// Run blocks until ctx is cancelled. The bindings live in the application's
// main loop, which Tray.Run owns, and Wails unregisters them during shutdown.
func (m *Manager) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
