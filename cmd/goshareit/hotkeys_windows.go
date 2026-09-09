//go:build windows

package main

import (
	"context"
	"sync"

	"github.com/Rake-Pro/GoShareIt/internal/core/hotkey"
	"github.com/Rake-Pro/GoShareIt/platform/windows"
)

// printScreenSplit routes each chord to the backend that can bind it: chords on
// the PrintScreen key go to the direct RegisterHotKey path in platform/windows,
// because the Wails accelerator grammar has no name for that key; every other
// chord goes to the Wails global-shortcut manager.
//
// main.go already splits comma-separated alternatives into individual chords
// before calling Register, so routing is per chord and stays best-effort: a
// chord one backend refuses is reported to the caller, which logs it and moves
// on to the next.
type printScreenSplit struct {
	wails  hotkey.Manager
	legacy hotkey.Manager

	mu   sync.Mutex
	byID map[string]hotkey.Manager
}

func newPrintScreenSplit(wails hotkey.Manager, legacy hotkey.Manager) *printScreenSplit {
	return &printScreenSplit{wails: wails, legacy: legacy, byID: map[string]hotkey.Manager{}}
}

func (s *printScreenSplit) Register(id, keys string, fn func()) error {
	target := s.wails
	if windows.IsPrintScreenChord(keys) {
		target = s.legacy
	}
	if err := target.Register(id, keys, fn); err != nil {
		return err
	}
	s.mu.Lock()
	s.byID[id] = target
	s.mu.Unlock()
	return nil
}

func (s *printScreenSplit) Unregister(id string) {
	s.mu.Lock()
	target := s.byID[id]
	delete(s.byID, id)
	s.mu.Unlock()
	if target != nil {
		target.Unregister(id)
	}
}

// Run drives both backends until ctx is cancelled and returns the first
// non-nil error either of them reported (both return ctx.Err() on a normal
// shutdown, which main.go ignores while ctx is done).
func (s *printScreenSplit) Run(ctx context.Context) error {
	errs := make(chan error, 2)
	go func() { errs <- s.wails.Run(ctx) }()
	go func() { errs <- s.legacy.Run(ctx) }()

	var first error
	for i := 0; i < cap(errs); i++ {
		if err := <-errs; err != nil && first == nil {
			first = err
		}
	}
	return first
}
