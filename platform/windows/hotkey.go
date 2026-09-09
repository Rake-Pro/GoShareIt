//go:build windows

package windows

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows"
)

// PrintScreenHotkeys is the legacy global-hotkey path for chords built on the
// PrintScreen key, and ONLY those. Everything else goes through the Wails
// global-shortcut manager in platform/wailsapp; Wails' accelerator grammar has
// no name for PrintScreen (its Windows key table has no VK_SNAPSHOT entry), so
// those chords are bound here with Win32 RegisterHotKey directly. Register
// rejects any other key on purpose, so this cannot quietly grow back into a
// second general hotkey backend.
//
// THREAD AFFINITY: RegisterHotKey with a null window posts WM_HOTKEY to the
// message queue of the calling thread, and UnregisterHotKey must run on that
// same thread. Run therefore pins itself to one OS thread, registers there, and
// pumps that thread's queue; Unregister and shutdown reach it by posting a
// message rather than calling across threads.
type PrintScreenHotkeys struct {
	mu       sync.Mutex
	bindings map[string]*psBinding
	nextID   int32
	threadID uint32 // non-zero once Run's message queue is up
}

type psBinding struct {
	keys       string
	mods       uint32
	fn         func()
	id         int32
	registered bool
}

// NewPrintScreenHotkeys returns an empty PrintScreen-only hotkey manager.
func NewPrintScreenHotkeys() *PrintScreenHotkeys {
	return &PrintScreenHotkeys{bindings: map[string]*psBinding{}}
}

// Win32 constants: RegisterHotKey modifiers, the messages the loop handles, and
// the virtual-key code for PrintScreen (VK_SNAPSHOT, which has no name in the
// Wails accelerator grammar).
const (
	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008

	vkSnapshot = 0x2C

	wmHotkey = 0x0312
	// WM_APP-based private messages: one asks the loop to release a hotkey it
	// owns, the other asks it to exit.
	wmUnregisterOne = 0x8000 + 1 // WM_APP+1
	wmStopLoop      = 0x8000 + 2 // WM_APP+2
)

var (
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPeekMessageW       = user32.NewProc("PeekMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
)

// win32Msg mirrors the Win32 MSG structure.
type win32Msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

// IsPrintScreenChord reports whether a chord's key is PrintScreen, i.e. whether
// this manager is the one that can bind it.
func IsPrintScreenChord(keys string) bool {
	for _, raw := range strings.Split(keys, "+") {
		if isPrintScreenKey(strings.ToLower(strings.TrimSpace(raw))) {
			return true
		}
	}
	return false
}

func isPrintScreenKey(token string) bool {
	switch token {
	case "printscreen", "prtsc", "prtscn", "snapshot":
		return true
	}
	return false
}

// Register parses keys (e.g. "Win+Ctrl+PrintScreen") and stores the binding.
// The OS-level registration happens later in Run.
func (m *PrintScreenHotkeys) Register(id, keys string, fn func()) error {
	mods, err := parsePrintScreenChord(keys)
	if err != nil {
		return fmt.Errorf("windows printscreen hotkey: %q: %w", keys, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.bindings[id]; ok {
		return fmt.Errorf("windows printscreen hotkey: %q already registered", id)
	}
	m.nextID++
	m.bindings[id] = &psBinding{keys: keys, mods: mods, fn: fn, id: m.nextID}
	return nil
}

// Unregister removes a binding and, when the loop is running, asks it to
// release the OS registration on its own thread.
func (m *PrintScreenHotkeys) Unregister(id string) {
	m.mu.Lock()
	b, ok := m.bindings[id]
	// Copy what is needed before unlocking: Run mutates b.registered under the
	// same mutex, so reading it afterwards would race.
	var hotkeyID int32
	var registered bool
	if ok {
		hotkeyID, registered = b.id, b.registered
	}
	delete(m.bindings, id)
	threadID := m.threadID
	m.mu.Unlock()
	if !ok || !registered || threadID == 0 {
		return
	}
	postThreadMessage(threadID, wmUnregisterOne, uintptr(hotkeyID))
}

// Run registers every binding with the OS and pumps the thread's message queue
// until ctx is cancelled, dispatching WM_HOTKEY to the matching callback. On
// exit it releases every registration it owns. With no bindings it just waits,
// so no OS thread is pinned for nothing.
func (m *PrintScreenHotkeys) Run(ctx context.Context) error {
	m.mu.Lock()
	empty := len(m.bindings) == 0
	m.mu.Unlock()
	if empty {
		<-ctx.Done()
		return ctx.Err()
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Force the thread message queue into existence before publishing the
	// thread id, so a stop posted immediately after cannot be dropped.
	var message win32Msg
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 0)

	m.mu.Lock()
	m.threadID = windows.GetCurrentThreadId()
	for _, b := range m.bindings {
		if err := registerHotKey(b.id, b.mods); err != nil {
			// Best-effort: one failed chord (duplicate, or PrtScn claimed by
			// Snipping Tool) must not take down every other hotkey.
			log.Warn().Err(err).Str("keys", b.keys).
				Msg("hotkey unavailable; skipping (Snipping Tool owns PrintScreen: enable hotkeys.disable_snipping_printscreen, then sign out and back in)")
			continue
		}
		b.registered = true
	}
	m.mu.Unlock()

	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-stopped:
			return
		}
		postThreadMessage(m.currentThreadID(), wmStopLoop, 0)
	}()

	var loopErr error
loop:
	for {
		r, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch {
		case int32(r) == -1:
			loopErr = fmt.Errorf("windows printscreen hotkey: GetMessage: %w", err)
			break loop
		case r == 0: // WM_QUIT
			break loop
		}
		switch message.message {
		case wmHotkey:
			m.dispatch(int32(message.wParam))
		case wmUnregisterOne:
			unregisterHotKey(int32(message.wParam))
		case wmStopLoop:
			break loop
		}
	}
	close(stopped)

	m.mu.Lock()
	for _, b := range m.bindings {
		if b.registered {
			unregisterHotKey(b.id)
			b.registered = false
		}
	}
	m.threadID = 0
	m.mu.Unlock()

	if loopErr != nil {
		return loopErr
	}
	return ctx.Err()
}

func (m *PrintScreenHotkeys) currentThreadID() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.threadID
}

// dispatch runs the callback for a fired hotkey id on its own goroutine, so a
// slow handler (a capture) never stalls the message loop.
func (m *PrintScreenHotkeys) dispatch(id int32) {
	m.mu.Lock()
	var fn func()
	for _, b := range m.bindings {
		if b.id == id {
			fn = b.fn
			break
		}
	}
	m.mu.Unlock()
	if fn != nil {
		go fn()
	}
}

func registerHotKey(id int32, mods uint32) error {
	// A null window handle posts WM_HOTKEY to this thread's queue, which Run
	// is about to pump.
	r, _, err := procRegisterHotKey.Call(0, uintptr(id), uintptr(mods), uintptr(vkSnapshot))
	if r == 0 {
		return fmt.Errorf("RegisterHotKey: %w", err)
	}
	return nil
}

func unregisterHotKey(id int32) {
	procUnregisterHotKey.Call(0, uintptr(id))
}

func postThreadMessage(threadID uint32, msg uint32, wParam uintptr) {
	if threadID == 0 {
		return
	}
	procPostThreadMessageW.Call(uintptr(threadID), uintptr(msg), wParam, 0)
}

// parsePrintScreenChord converts "Mod+Mod+PrintScreen" into Win32 RegisterHotKey
// modifier flags. Any key other than PrintScreen is rejected: this path exists
// only for the one key Wails cannot name.
//
// MODIFIER MAPPING (unchanged): macOS configs use "Cmd" as the primary
// modifier, and the Windows logo key reserves most Win+<key> combos at the
// shell level, so cmd/command/super/meta map to Ctrl. "Win" still maps to the
// logo key for users who explicitly want it.
func parsePrintScreenChord(s string) (uint32, error) {
	var mods uint32
	haveKey := false
	for _, raw := range strings.Split(s, "+") {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			continue
		}
		if mod, ok := modifierFor(token); ok {
			mods |= mod
			continue
		}
		if !isPrintScreenKey(token) {
			return 0, fmt.Errorf("key %q is not PrintScreen; this backend binds PrintScreen chords only", token)
		}
		if haveKey {
			return 0, fmt.Errorf("multiple non-modifier keys")
		}
		haveKey = true
	}
	if !haveKey {
		return 0, fmt.Errorf("no PrintScreen key in chord")
	}
	return mods, nil
}

func modifierFor(s string) (uint32, bool) {
	switch s {
	case "cmd", "command", "super", "meta":
		return modControl, true
	case "win":
		return modWin, true
	case "shift":
		return modShift, true
	case "ctrl", "control":
		return modControl, true
	case "option", "opt", "alt":
		return modAlt, true
	}
	return 0, false
}
