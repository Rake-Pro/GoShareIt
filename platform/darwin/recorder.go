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
	"image"
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

// Capabilities advertises VideoFull and VideoRegion. Region cropping is applied
// via AVCaptureScreenInput.cropRect when StartRegion is given a non-empty rect
// (see StartRegion for the coordinate conversion); an empty rect records the
// full display.
func (r *Recorder) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{capture.VideoFull, capture.VideoRegion}}
}

// Recording reports whether a recording is currently in progress.
func (r *Recorder) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

// Start begins recording the full main display to a temp .mp4 and returns once
// the OS recorder is running. It is equivalent to StartRegion with an empty
// rect. A second Start while recording returns capture.ErrAlreadyRecording.
func (r *Recorder) Start(ctx context.Context, mode capture.Mode) error {
	return r.StartRegion(ctx, mode, image.Rectangle{})
}

// StartRegion begins recording either the full main display (empty rect) or a
// sub-rectangle of it to a temp .mp4, returning once the OS recorder is running.
// A second StartRegion while recording returns capture.ErrAlreadyRecording.
//
// rect is in screen PIXEL coordinates with a TOP-LEFT origin (the convention of
// the region overlay). AVCaptureScreenInput.cropRect, however, is a CGRect in
// display POINTS with a BOTTOM-LEFT origin. The C shim
// (gsi_recorder_start_region) performs that conversion: it divides the pixel
// rect by the main display's backing scale factor to get points, and flips Y
// against the display's point height (cropY = displayHeightPoints - rectMaxY/
// scale). A zero/empty rect (w<=0 || h<=0) is passed through as "no crop" and
// records the full display, matching Start.
//
// #1 ON-DEVICE RISK: this coordinate/scale conversion is unverified on real
// hardware - on Retina displays (scale 2) and multi-display setups the cropRect
// math (scale factor source, Y-flip reference height) is the most likely thing
// to be subtly wrong. Verify the recorded area matches the selected rect on a
// real Mac before trusting it.
func (r *Recorder) StartRegion(_ context.Context, mode capture.Mode, rect image.Rectangle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recording {
		return capture.ErrAlreadyRecording
	}
	switch mode {
	case capture.VideoFull, capture.VideoRegion:
		// supported; empty rect => full display.
	default:
		return fmt.Errorf("darwin recorder: unsupported mode %s", mode)
	}

	path := filepath.Join(os.TempDir(), fmt.Sprintf("goshareit_rec_%d.mp4", time.Now().UnixNano()))
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	// An empty rect records the full display (w/h <= 0 => no crop in the shim).
	rc := C.gsi_recorder_start_region(cpath,
		C.int(rect.Min.X), C.int(rect.Min.Y), C.int(rect.Dx()), C.int(rect.Dy()))
	if rc != 0 {
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
	_ = os.Remove(path) // bytes are in memory; don't leave the temp mp4 behind

	return capture.Result{
		Bytes: b,
		Mime:  "video/mp4",
		Kind:  capture.KindVideo,
	}, nil
}
