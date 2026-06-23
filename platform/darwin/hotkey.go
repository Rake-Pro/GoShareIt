//go:build darwin

package darwin

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.design/x/hotkey"
)

// HotkeyManager implements the global-hotkey seam via golang.design/x/hotkey.
//
// MAIN-LOOP CONTRACT (verified against the v0.6.1 darwin source): on macOS this
// library installs a CGEventTap and adds its source to CFRunLoopGetMain(), so
// hotkey events are delivered by the process's MAIN run loop. fyne.io/systray
// (see tray.go) owns and runs that main loop via systray.Run. They coexist as:
//
//   - systray.Run is the single main-loop owner.
//   - Registration (hk.Register) is dispatched onto the main queue internally
//     (dispatch_sync to dispatch_get_main_queue), so it is safe to call from any
//     goroutine; it self-synchronizes against the systray main loop. The frozen
//     cmd/goshareit/main.go runs HotkeyManager.Run in its own goroutine before
//     systray starts; that goroutine simply blocks inside dispatch_sync until
//     systray's main loop begins draining the main queue, then registration
//     completes. No extra wiring is required for the two to coexist.
//   - Each hotkey's Keydown channel is serviced on its own background goroutine,
//     so callbacks never block the main loop.
//
// This package needs cgo, Accessibility (Input Monitoring) permission, and in
// practice an .app bundle for the OS to deliver global hotkey events reliably.
type HotkeyManager struct {
	mu       sync.Mutex
	bindings map[string]*binding
}

type binding struct {
	keys string
	fn   func()
	hk   *hotkey.Hotkey
}

// NewHotkeyManager returns an empty macOS hotkey manager.
func NewHotkeyManager() *HotkeyManager {
	return &HotkeyManager{bindings: map[string]*binding{}}
}

// Register parses keys (e.g. "Cmd+Shift+1") and stores the binding. The OS-level
// registration happens later in Run.
func (m *HotkeyManager) Register(id, keys string, fn func()) error {
	mods, key, err := parseHotkey(keys)
	if err != nil {
		return fmt.Errorf("darwin hotkey: %q: %w", keys, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.bindings[id]; ok {
		return fmt.Errorf("darwin hotkey: %q already registered", id)
	}
	m.bindings[id] = &binding{keys: keys, fn: fn, hk: hotkey.New(mods, key)}
	return nil
}

// Unregister removes a binding and releases it from the OS if Run is active.
func (m *HotkeyManager) Unregister(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.bindings[id]; ok {
		if b.hk != nil {
			_ = b.hk.Unregister()
		}
		delete(m.bindings, id)
	}
}

// Run registers every binding with the OS, spawns a goroutine per binding that
// ranges over its Keydown channel, and blocks until ctx is cancelled. On exit it
// unregisters all hotkeys.
//
// Because hotkey events flow through the main run loop owned by systray, Run is
// expected to be invoked from the systray onReady callback (see wire_darwin.go).
func (m *HotkeyManager) Run(ctx context.Context) error {
	m.mu.Lock()
	active := make([]*binding, 0, len(m.bindings))
	for _, b := range m.bindings {
		if err := b.hk.Register(); err != nil {
			m.mu.Unlock()
			for _, done := range active {
				_ = done.hk.Unregister()
			}
			return fmt.Errorf("darwin hotkey: register %q: %w", b.keys, err)
		}
		active = append(active, b)
	}
	m.mu.Unlock()

	for _, b := range active {
		go func(b *binding) {
			for {
				select {
				case <-ctx.Done():
					return
				case <-b.hk.Keydown():
					if b.fn != nil {
						b.fn()
					}
				}
			}
		}(b)
	}

	<-ctx.Done()

	for _, b := range active {
		_ = b.hk.Unregister()
	}
	return ctx.Err()
}

// parseHotkey converts a "Mod+Mod+Key" string into hotkey modifiers and a key.
func parseHotkey(s string) ([]hotkey.Modifier, hotkey.Key, error) {
	parts := strings.Split(s, "+")
	if len(parts) == 0 {
		return nil, 0, fmt.Errorf("empty hotkey")
	}
	var mods []hotkey.Modifier
	var key hotkey.Key
	haveKey := false
	for _, raw := range parts {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if mod, ok := modifierFor(p); ok {
			mods = append(mods, mod)
			continue
		}
		k, ok := keyFor(p)
		if !ok {
			return nil, 0, fmt.Errorf("unknown token %q", p)
		}
		if haveKey {
			return nil, 0, fmt.Errorf("multiple non-modifier keys")
		}
		key = k
		haveKey = true
	}
	if !haveKey {
		return nil, 0, fmt.Errorf("no key specified")
	}
	return mods, key, nil
}

func modifierFor(s string) (hotkey.Modifier, bool) {
	switch strings.ToLower(s) {
	case "cmd", "command", "super", "meta", "win":
		return hotkey.ModCmd, true
	case "shift":
		return hotkey.ModShift, true
	case "ctrl", "control":
		return hotkey.ModCtrl, true
	case "option", "opt", "alt":
		return hotkey.ModOption, true
	default:
		return 0, false
	}
}

// keyDigits and keyLetters map names to the cross-platform hotkey.Key constants.
var (
	keyDigits = map[string]hotkey.Key{
		"0": hotkey.Key0, "1": hotkey.Key1, "2": hotkey.Key2, "3": hotkey.Key3,
		"4": hotkey.Key4, "5": hotkey.Key5, "6": hotkey.Key6, "7": hotkey.Key7,
		"8": hotkey.Key8, "9": hotkey.Key9,
	}
	keyLetters = map[string]hotkey.Key{
		"a": hotkey.KeyA, "b": hotkey.KeyB, "c": hotkey.KeyC, "d": hotkey.KeyD,
		"e": hotkey.KeyE, "f": hotkey.KeyF, "g": hotkey.KeyG, "h": hotkey.KeyH,
		"i": hotkey.KeyI, "j": hotkey.KeyJ, "k": hotkey.KeyK, "l": hotkey.KeyL,
		"m": hotkey.KeyM, "n": hotkey.KeyN, "o": hotkey.KeyO, "p": hotkey.KeyP,
		"q": hotkey.KeyQ, "r": hotkey.KeyR, "s": hotkey.KeyS, "t": hotkey.KeyT,
		"u": hotkey.KeyU, "v": hotkey.KeyV, "w": hotkey.KeyW, "x": hotkey.KeyX,
		"y": hotkey.KeyY, "z": hotkey.KeyZ,
	}
	keyFns = map[string]hotkey.Key{
		"f1": hotkey.KeyF1, "f2": hotkey.KeyF2, "f3": hotkey.KeyF3, "f4": hotkey.KeyF4,
		"f5": hotkey.KeyF5, "f6": hotkey.KeyF6, "f7": hotkey.KeyF7, "f8": hotkey.KeyF8,
		"f9": hotkey.KeyF9, "f10": hotkey.KeyF10, "f11": hotkey.KeyF11, "f12": hotkey.KeyF12,
		"f13": hotkey.KeyF13, "f14": hotkey.KeyF14, "f15": hotkey.KeyF15, "f16": hotkey.KeyF16,
		"f17": hotkey.KeyF17, "f18": hotkey.KeyF18, "f19": hotkey.KeyF19, "f20": hotkey.KeyF20,
	}
)

func keyFor(s string) (hotkey.Key, bool) {
	l := strings.ToLower(s)
	if k, ok := keyDigits[l]; ok {
		return k, true
	}
	if k, ok := keyLetters[l]; ok {
		return k, true
	}
	if k, ok := keyFns[l]; ok {
		return k, true
	}
	return 0, false
}
