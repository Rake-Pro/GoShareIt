//go:build windows

package windows

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
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
// MODIFIER MAPPING: macOS configs use "Cmd" as the primary modifier (e.g.
// "Cmd+Shift+1"). On Windows the equivalent primary modifier is Ctrl, and the
// Windows logo key (Win) reserves most Win+<key> combos at the OS/shell level, so
// Cmd/Command/Super/Meta map to hotkey.ModCtrl - this lets a shared macOS config
// register on Windows as Ctrl+Shift+1 instead of the OS-reserved Win+Shift+1.
// "Win" still maps to the logo key for users who explicitly want it. Ctrl/Shift/
// Alt map to their Win32 equivalents directly.
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
		// Best-effort: one failed chord (duplicate, OS-claimed like PrtScn by
		// Snipping Tool) must not take down every other hotkey.
		if err := b.hk.Register(); err != nil {
			msg := "hotkey unavailable; skipping"
			if strings.Contains(strings.ToLower(b.keys), "print") || strings.Contains(strings.ToLower(b.keys), "prtsc") {
				msg += " (Snipping Tool owns PrintScreen: enable hotkeys.disable_snipping_printscreen, then sign out and back in)"
			}
			log.Warn().Err(err).Str("keys", b.keys).Msg(msg)
			continue
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
	case "cmd", "command", "super", "meta":
		// macOS primary modifier -> Ctrl on Windows (Win+<key> is OS-reserved).
		return hotkey.ModCtrl, true
	case "win":
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
	// keyPunct maps punctuation (US layout) to raw VK_OEM_* codes; x/hotkey
	// has no named constants for these. Same token set as the darwin backend.
	keyPunct = map[string]hotkey.Key{
		"`": 0xC0, "grave": 0xC0, "backtick": 0xC0,
		"-": 0xBD, "minus": 0xBD,
		"=": 0xBB, "equals": 0xBB,
		"[": 0xDB, "]": 0xDD,
		";": 0xBA, "'": 0xDE,
		",": 0xBC, ".": 0xBE, "/": 0xBF, "\\": 0xDC,
		"space": hotkey.KeySpace,
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
	if k, ok := keyPunct[l]; ok {
		return k, true
	}
	switch l {
	case "printscreen", "prtsc", "prtscn", "snapshot":
		// hotkey.Key is a raw virtual-key code on windows; VK_SNAPSHOT has no
		// named constant in x/hotkey.
		return hotkey.Key(0x2C), true
	}
	return 0, false
}
