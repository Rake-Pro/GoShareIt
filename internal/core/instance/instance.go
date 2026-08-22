// Package instance enforces a single running GoShareIt host per user session.
// Windows uses a named kernel mutex, everything else an flock on a file in the
// app root. Both are released by the OS when the process dies, so a crash
// never leaves a stale lock.
package instance

import (
	"errors"
	"time"
)

// ErrRunning is returned when another instance holds the lock.
var ErrRunning = errors.New("instance: another GoShareIt is already running")

// Acquire takes the single-instance lock, retrying for up to wait so a
// relaunch (new process started just before the old one exits) succeeds.
// The returned release func is safe to call more than once.
func Acquire(wait time.Duration) (release func(), err error) {
	deadline := time.Now().Add(wait)
	for {
		release, err = tryAcquire()
		if !errors.Is(err, ErrRunning) || time.Now().After(deadline) {
			return release, err
		}
		time.Sleep(250 * time.Millisecond)
	}
}
