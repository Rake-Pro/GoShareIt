//go:build windows

package winocr

import (
	"context"
	"runtime"
	"sync"

	"github.com/go-ole/go-ole"
)

// worker owns one OS thread with WinRT initialized (multithreaded apartment)
// and runs every WinRT call on it. OcrEngine is agile, so this is not
// strictly required, but a single initialized thread removes "RoInitialize
// per caller thread" from every call site. It starts on first use and lives
// as long as the process.
type worker struct {
	once sync.Once
	reqs chan func()
}

// roInitMultithreaded is RO_INIT_MULTITHREADED.
const roInitMultithreaded = 1

func (w *worker) start() {
	w.reqs = make(chan func())
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		// The result is ignored on purpose: S_FALSE means already
		// initialized, RPC_E_CHANGED_MODE still leaves an apartment the agile
		// OCR objects work from, and a real failure surfaces at the first
		// activation, which Probe reports as a reason.
		_ = ole.RoInitialize(roInitMultithreaded)
		close(ready)
		for fn := range w.reqs {
			fn()
		}
	}()
	<-ready
}

// do runs fn on the worker thread and waits for it, or for ctx. A WinRT
// call that hangs outside await (activation, the language list) therefore
// cannot block the caller past its deadline: do returns ctx.Err() and fn
// finishes, or stays stuck, on the worker; later calls then time out while
// queued. The callers' closures only fill captured results, which the
// callers never read once do has returned an error, so a late finish is
// harmless.
func (w *worker) do(ctx context.Context, fn func()) error {
	w.once.Do(w.start)
	done := make(chan struct{})
	select {
	case w.reqs <- func() { defer close(done); fn() }:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
