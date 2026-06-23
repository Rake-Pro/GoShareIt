//go:build darwin

// Package darwin provides the macOS platform backends for the GoShareIt core
// seams. All files are guarded by the darwin build tag so the portable linux
// build excludes them.
package darwin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// ErrCaptureCancelled is returned when the user dismisses an interactive
// capture without selecting anything (screencapture exits 0 but writes no
// file). The pipeline treats this as a no-op rather than an error.
var ErrCaptureCancelled = errors.New("darwin capture: cancelled by user")

// ErrUnsupportedMode is returned for modes not implemented in Phase 1.
var ErrUnsupportedMode = errors.New("darwin capture: mode not implemented in P1")

// Capturer shells out to the macOS `screencapture` binary.
type Capturer struct {
	mu sync.Mutex
	// lastRegion holds the rect of the most recent interactive region capture
	// in "x,y,w,h" form for LastRegion replay. screencapture -i does not report
	// the chosen rect, so this stays empty until a future P2 picker can supply
	// it; LastRegion therefore currently falls back to RegionInteractive.
	// TODO(P2): capture the selected rect (e.g. via a custom overlay) so
	// LastRegion can use `screencapture -R x,y,w,h`.
	lastRegion string
}

// NewCapturer returns a macOS screencapture-backed Capturer.
func NewCapturer() *Capturer { return &Capturer{} }

// Capabilities advertises only the still-image modes supported in P1.
func (c *Capturer) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{
		capture.RegionInteractive,
		capture.FullScreen,
		capture.ActiveWindow,
		capture.WindowPick,
		capture.LastRegion,
	}}
}

// Capture runs screencapture for the requested mode and returns the PNG bytes.
func (c *Capturer) Capture(ctx context.Context, r capture.Request) (capture.Result, error) {
	switch r.Mode {
	case capture.VideoRegion, capture.VideoFull, capture.GIF:
		return capture.Result{}, fmt.Errorf("%w: %s", ErrUnsupportedMode, r.Mode)
	}

	path, cleanup, err := c.outPath(r)
	if err != nil {
		return capture.Result{}, err
	}
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()

	args, err := c.args(r.Mode, path)
	if err != nil {
		return capture.Result{}, err
	}

	cmd := exec.CommandContext(ctx, "screencapture", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return capture.Result{}, fmt.Errorf("darwin capture: screencapture failed: %v: %s", err, out)
	}

	// screencapture writes nothing (and still exits 0) when an interactive
	// selection is cancelled with Esc.
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return capture.Result{}, ErrCaptureCancelled
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return capture.Result{}, fmt.Errorf("darwin capture: read output: %w", err)
	}

	res := capture.Result{
		Bytes: b,
		Mime:  "image/png",
		Kind:  capture.KindImage,
	}
	if r.SaveLocal {
		keep = true
		res.Path = path
	}
	return res, nil
}

// args builds the screencapture argument list for the mode. `-x` suppresses the
// camera sound; the trailing path is the PNG output target.
func (c *Capturer) args(mode capture.Mode, path string) ([]string, error) {
	switch mode {
	case capture.RegionInteractive:
		return []string{"-i", "-x", path}, nil
	case capture.FullScreen:
		return []string{"-x", path}, nil
	case capture.ActiveWindow, capture.WindowPick:
		// Both map to interactive window selection in P1: macOS has no stable
		// "active window" screencapture flag, so the user clicks the target.
		return []string{"-i", "-w", "-x", path}, nil
	case capture.LastRegion:
		c.mu.Lock()
		rect := c.lastRegion
		c.mu.Unlock()
		if rect != "" {
			return []string{"-R", rect, "-x", path}, nil
		}
		// No stored rect yet: fall back to interactive region.
		return []string{"-i", "-x", path}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedMode, mode)
	}
}

// outPath returns the target file plus a cleanup func. When SaveLocal is set the
// file lives under SaveDir (or the temp dir) and is kept; otherwise it is a temp
// file removed after the bytes are read.
func (c *Capturer) outPath(r capture.Request) (string, func(), error) {
	name := fmt.Sprintf("goshareit_%d.png", time.Now().UnixNano())
	dir := os.TempDir()
	if r.SaveLocal && r.SaveDir != "" {
		if err := os.MkdirAll(r.SaveDir, 0o755); err != nil {
			return "", func() {}, fmt.Errorf("darwin capture: create save dir: %w", err)
		}
		dir = r.SaveDir
	}
	path := filepath.Join(dir, name)
	return path, func() { _ = os.Remove(path) }, nil
}
