//go:build !windows

package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
)

// lockPath is overridable for tests.
var lockPath = func() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "goshareit.lock"), nil
}

func tryAcquire() (func(), error) {
	path, err := lockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("instance: lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("instance: open lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, ErrRunning
		}
		return nil, fmt.Errorf("instance: flock: %w", err)
	}
	var once sync.Once
	return func() { once.Do(func() { _ = f.Close() }) }, nil
}
