//go:build windows

package instance

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

// Local\ scopes the mutex to the current logon session, so two users on the
// same machine can each run their own instance.
const mutexName = `Local\GoShareIt.single-instance`

func tryAcquire() (func(), error) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, fmt.Errorf("instance: mutex name: %w", err)
	}
	h, err := windows.CreateMutex(nil, false, name)
	if h == 0 {
		return nil, fmt.Errorf("instance: create mutex: %w", err)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(h)
		return nil, ErrRunning
	}
	var once sync.Once
	return func() { once.Do(func() { _ = windows.CloseHandle(h) }) }, nil
}
