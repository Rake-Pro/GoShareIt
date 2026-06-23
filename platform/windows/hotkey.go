//go:build windows

package windows

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.design/x/hotkey"
)

// HotkeyManager implements the global-hotkey seam via golang.design/x/hotkey.
//
// On Windows the library is pure syscall and registers hotkeys with the Win32
// RegisterHotKey API. Unlike macOS this needs no special permission and no app
// bundle, and it does not require ownership of the process main run loop: the
// library spins its own message-only window/thread per hotkey internally, so it
// coexists with fyne.io/systray (see tray.go) without sharing a run loop.
//
// MODIFIER MAPPING: macOS configs use "Cmd" (e.g. "Cmd+Shift+4"). Windows has no
// Command key, so Cmd/Command/Super/Meta/Win all map to the Windows logo key
// (hotkey.ModWin). NOTE: many Win+<key> combinations are reserved by the OS/shell
// and RegisterHotKey will fail for those; users porting a macOS config may need
// to rebind to a Ctrl/Alt/Shift combination. Ctrl/Shift/Alt map to their Win32
// equivalents directly.
type HotkeyManager struct {
	mu       sync.Mutex
	bindings map[string]*binding
}

type binding struct {
	keys string
	fn   func()
	hk   *hotkey.Hotkey
}

// NewHotkeyManager returns an empty Windows hotkey manager.
func NewHotkeyManager() *HotkeyManager {
	return &HotkeyManager{bindings: map[string]*binding{}}
}

// Register parses keys (e.g. "Ctrl+Shift+1") and stores the binding. The OS-level
// registration happens later in Run.
func (m *HotkeyManager) Register(id, keys string, fn func()) error {
	mods, key, err := parseHotkey(keys)
	if err != nil {
		return fmt.Errorf("windows hotkey: %q: %w", keys, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.bindings[id]; ok {
		return fmt.Errorf("windows hotkey: %q already registered", id)
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
func (m *HotkeyManager) Run(ctx context.Context) error {
	m.mu.Lock()
	active := make([]*binding, 0, len(m.bindings))
	for _, b := range m.bindings {
		if err := b.hk.Register(); err != nil {
			m.mu.Unlock()
			for _, done := range active {
				_ = done.hk.Unregister()
			}
			return fmt.Errorf("windows hotkey: register %q: %w", b.keys, err)
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
		// macOS Cmd has no Windows equivalent; map to the Windows logo key.
		return hotkey.ModWin, true
	case "shift":
		return hotkey.ModShift, true
	case "ctrl", "control":
		return hotkey.ModCtrl, true
	case "option", "opt", "alt":
		return hotkey.ModAlt, true
	default:
		return 0, false
	}
}

// keyDigits, keyLetters and keyFns map names to the cross-platform hotkey.Key
// constants (identical set to the darwin backend).
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
