//go:build darwin

package darwin

// Recorder is the macOS screen-recording backend for the capture.Recorder seam.
// It is implemented with native AVFoundation (AVCaptureScreenInput +
// AVCaptureMovieFileOutput) via cgo - NO ffmpeg. The Objective-C lives in
// recorder.m / recorder.h; see recorder.m's header comment for the full
// rationale on choosing AVFoundation over ScreenCaptureKit.
//
// Threading: Start and Stop are guarded by a mutex so concurrent
// Start/Stop/Recording calls are safe. Stop blocks (inside the C shim) until the
// movie file output's completion delegate fires; that callback is delivered on
// AVFoundation's own queue, so Stop must NOT be invoked on the main thread (the
// app runs systray on the main thread; the recorder is driven from the pipeline
// goroutine, not main).
//
// Permissions: screen capture requires the Screen Recording TCC grant. The
// signed .app must already hold it (Info.plist carries NSScreenCaptureUsageDescription).

/*
#cgo LDFLAGS: -framework Foundation -framework AVFoundation -framework CoreMedia -framework CoreGraphics
#include <stdlib.h>
#include "recorder.h"
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// Recorder records the main display to a temporary .mp4 and returns the encoded
// bytes on Stop. Only one recording may be active at a time.
type Recorder struct {
	mu        sync.Mutex
	recording bool
	path      string
}

// NewRecorder returns an AVFoundation-backed macOS screen Recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// Capabilities advertises VideoFull only. VideoRegion is accepted by Start but
// currently records the full display (no rect is supplied to the Recorder and
// interactive video-region selection needs an overlay we do not yet have), so
// it is not advertised as a real capability.
// TODO(P3b): implement VideoRegion crop (AVCaptureScreenInput.cropRect) once an
// interactive region overlay can feed Start a rect, then advertise it here.
func (r *Recorder) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{capture.VideoFull}}
}

// Recording reports whether a recording is currently in progress.
func (r *Recorder) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

// Start begins recording the main display to a temp .mp4 and returns once the
// OS recorder is running. A second Start while recording returns
// capture.ErrAlreadyRecording.
func (r *Recorder) Start(_ context.Context, mode capture.Mode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recording {
		return capture.ErrAlreadyRecording
	}
	switch mode {
	case capture.VideoFull, capture.VideoRegion:
		// VideoRegion falls back to full-display capture; see Capabilities.
	default:
		return fmt.Errorf("darwin recorder: unsupported mode %s", mode)
	}

	path := filepath.Join(os.TempDir(), fmt.Sprintf("goshareit_rec_%d.mp4", time.Now().UnixNano()))
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	if rc := C.gsi_recorder_start(cpath); rc != 0 {
		return fmt.Errorf("darwin recorder: start failed (%d): %s", int(rc), C.GoString(C.gsi_recorder_last_error()))
	}

	r.recording = true
	r.path = path
	return nil
}

// Stop ends the active recording, blocks until the mp4 is fully finalized on
// disk (the C shim waits on the movie file output's completion delegate), then
// reads and returns the bytes. With no active recording it returns
// capture.ErrNotRecording.
func (r *Recorder) Stop(_ context.Context) (capture.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		return capture.Result{}, capture.ErrNotRecording
	}

	// gsi_recorder_stop blocks until finalization completes; only after it
	// returns is the file guaranteed fully written.
	rc := C.gsi_recorder_stop()
	r.recording = false
	path := r.path
	r.path = ""

	if rc != 0 {
		_ = os.Remove(path)
		return capture.Result{}, fmt.Errorf("darwin recorder: stop failed (%d): %s", int(rc), C.GoString(C.gsi_recorder_last_error()))
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return capture.Result{}, fmt.Errorf("darwin recorder: read output: %w", err)
	}

	return capture.Result{
		Path:  path,
		Bytes: b,
		Mime:  "video/mp4",
		Kind:  capture.KindVideo,
	}, nil
}
