// Package region is the host-side launcher for the out-of-process interactive
// screen-region selector. It mirrors internal/core/edit's Launcher: it spawns
// the goshareit-editor helper with --region, blocks on it (honoring ctx and
// Timeout), and parses the selected rectangle the helper prints. The GUI lives
// entirely in the helper process so it never contends with the menu-bar host's
// main run loop. This package is pure Go (CGO-free) and unit-testable.
package region

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// cancelledExitCode is the sentinel the helper returns when the user cancels the
// selection (Esc, empty selection, or window close); no --out is written.
const cancelledExitCode = 64

// Selector resolves an interactive screen region. ok is false when the user
// cancels; err is non-nil only on a genuine failure (helper missing, bad
// output, timeout).
type Selector interface {
	Select(ctx context.Context) (rect image.Rectangle, ok bool, err error)
}

// Launcher is the host-side Selector. It execs the region overlay helper and
// parses its "x,y,w,h" output into an image.Rectangle.
type Launcher struct {
	HelperPath string        // path to the helper binary; "" -> sibling of os.Executable()
	Timeout    time.Duration // safety cap; 0 = no cap
}

// Select implements Selector by invoking the out-of-process region overlay.
func (l Launcher) Select(ctx context.Context) (image.Rectangle, bool, error) {
	helper, err := l.resolveHelper()
	if err != nil {
		return image.Rectangle{}, false, err
	}

	dir, err := os.MkdirTemp("", "goshareit-region-")
	if err != nil {
		return image.Rectangle{}, false, fmt.Errorf("region: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	outPath := filepath.Join(dir, "rect.txt")

	if l.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, helper, "--region", "--out", outPath)
	runErr := cmd.Run()
	if runErr == nil {
		raw, err := os.ReadFile(outPath)
		if err != nil {
			return image.Rectangle{}, false, fmt.Errorf("region: read output: %w", err)
		}
		rect, err := parseRect(string(raw))
		if err != nil {
			return image.Rectangle{}, false, err
		}
		return rect, true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		switch code := exitErr.ExitCode(); code {
		case cancelledExitCode:
			return image.Rectangle{}, false, nil
		default:
			return image.Rectangle{}, false, fmt.Errorf("region: helper exit %d", code)
		}
	}

	// Non-ExitError: helper missing, not executable, killed by ctx, etc.
	return image.Rectangle{}, false, fmt.Errorf("region: run helper: %w", runErr)
}

func (l Launcher) resolveHelper() (string, error) {
	if l.HelperPath != "" {
		return l.HelperPath, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("region: locate executable: %w", err)
	}
	name := "goshareit-editor"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}

// parseRect parses a single "x,y,w,h" line of four integers into a rectangle
// with corners (x,y)-(x+w,y+h). Width and height must be positive.
func parseRect(s string) (image.Rectangle, error) {
	fields := strings.Split(strings.TrimSpace(s), ",")
	if len(fields) != 4 {
		return image.Rectangle{}, fmt.Errorf("region: bad output %q: want x,y,w,h", strings.TrimSpace(s))
	}
	v := make([]int, 4)
	for i, f := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return image.Rectangle{}, fmt.Errorf("region: bad integer %q: %w", f, err)
		}
		v[i] = n
	}
	x, y, w, h := v[0], v[1], v[2], v[3]
	if w <= 0 || h <= 0 {
		return image.Rectangle{}, fmt.Errorf("region: non-positive size %dx%d", w, h)
	}
	return image.Rect(x, y, x+w, y+h), nil
}
