package edit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// cancelledExitCode is the sentinel the editor helper returns when the user
// skips or cancels (no --out is written).
const cancelledExitCode = 64

// Launcher is the host-side Editor implementation. It does not draw anything:
// it spawns a separate editor helper process, hands it the captured PNG via a
// temp file, blocks on it (honoring ctx and Timeout), and reads the edited PNG
// back. The helper owns its own GUI main loop in its own process, so it never
// contends with the menu-bar host's main run loop.
type Launcher struct {
	HelperPath  string        // path to the editor helper binary; "" -> sibling of os.Executable()
	Timeout     time.Duration // safety cap; 0 = no cap
	Tool        string        // default tool, passed as --tool
	Color       string        // default color, passed as --color
	StrokeWidth int           // default stroke width, passed as --stroke
	Tools       []string      // enabled tools, passed as --tools csv
}

// Edit implements Editor by invoking the out-of-process editor helper.
func (l Launcher) Edit(ctx context.Context, in capture.Result) (capture.Result, bool, error) {
	if in.Kind != capture.KindImage {
		return in, false, nil
	}

	helper, err := l.resolveHelper()
	if err != nil {
		return in, false, err
	}

	dir, err := os.MkdirTemp("", "goshareit-edit-")
	if err != nil {
		return in, false, fmt.Errorf("editor: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	inPath := filepath.Join(dir, "in.png")
	outPath := filepath.Join(dir, "out.png")
	if err := os.WriteFile(inPath, in.Bytes, 0o600); err != nil {
		return in, false, fmt.Errorf("editor: write input: %w", err)
	}

	if l.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.Timeout)
		defer cancel()
	}

	args := []string{"--in", inPath, "--out", outPath}
	if l.Tool != "" {
		args = append(args, "--tool", l.Tool)
	}
	if l.Color != "" {
		args = append(args, "--color", l.Color)
	}
	if l.StrokeWidth > 0 {
		args = append(args, "--stroke", fmt.Sprintf("%d", l.StrokeWidth))
	}
	if len(l.Tools) > 0 {
		args = append(args, "--tools", strings.Join(l.Tools, ","))
	}

	cmd := exec.CommandContext(ctx, helper, args...)
	runErr := cmd.Run()
	if runErr == nil {
		edited, err := os.ReadFile(outPath)
		if err != nil {
			return in, false, fmt.Errorf("editor: read output: %w", err)
		}
		return capture.Result{
			Path:  "",
			Bytes: edited,
			Mime:  "image/png",
			Kind:  capture.KindImage,
		}, true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		switch code := exitErr.ExitCode(); code {
		case cancelledExitCode:
			return in, false, nil
		default:
			return in, false, fmt.Errorf("editor: helper exit %d", code)
		}
	}

	// Non-ExitError: helper missing, not executable, killed by ctx, etc. Fail-open.
	return in, false, fmt.Errorf("editor: run helper: %w", runErr)
}

func (l Launcher) resolveHelper() (string, error) {
	if l.HelperPath != "" {
		return l.HelperPath, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("editor: locate executable: %w", err)
	}
	name := "goshareit-editor"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}
