//go:build !windows

package instance

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireExclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "goshareit.lock")
	lockPath = func() (string, error) { return p, nil }

	release, err := Acquire(0)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := Acquire(300 * time.Millisecond); !errors.Is(err, ErrRunning) {
		t.Fatalf("second acquire: want ErrRunning, got %v", err)
	}
	release()
	release() // idempotent
	release2, err := Acquire(0)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	release2()
}
