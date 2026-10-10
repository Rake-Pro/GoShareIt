package edit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// Helper exit codes: the sentinels the editor helper returns via its exit
// status. 0 (plain confirm) is handled implicitly by cmd.Run() returning nil.
const (
	cancelledExitCode = 64 // skip/cancel/Esc/window-close: --out not written
	copyExitCode      = 65 // Copy action: --out written
	saveExitCode      = 66 // Save action: --out written
	uploadExitCode    = 67 // Upload action: --out written
)

// Launcher is the host-side Editor implementation. It does not draw anything:
// it spawns a separate editor helper process, hands it the captured PNG via a
// temp file, blocks on it (honoring ctx and Timeout), and reads the edited PNG
// back. The helper owns its own GUI main loop in its own process, so it never
// contends with the menu-bar host's main run loop.
type Launcher struct {
	HelperPath   string        // path to the editor helper binary; "" -> sibling of os.Executable()
	Timeout      time.Duration // safety cap; 0 = no cap
	Tool         string        // default tool, passed as --tool
	Color        string        // default color, passed as --color
	StrokeWidth  int           // default stroke width, passed as --stroke
	Tools        []string      // enabled tools, passed as --tools csv
	Theme        string        // "light"|"dark"|"system"/"", passed as --theme; the helper resolves "system"
	ConfirmLabel string        // rendered on the confirm button, passed as --confirm-label; "" -> helper falls back to "Done"
	// ConfirmLabelFor, when set, computes the confirm label per launch from
	// the live upload state (Opts.CanUpload) and wins over ConfirmLabel, so a
	// tray/hotkey upload toggle is reflected on the button.
	ConfirmLabelFor func(canUpload bool) string

	// Text recognition (config ocr.*). OCREnabled=false hides the text tools
	// entirely (--ocr=hidden); otherwise Opts.OCR decides between on and off
	// (greyed out with the reason).
	OCREnabled  bool
	OCRAutoRun  bool          // passed as --ocr-auto
	OCRLangs    []string      // passed as --ocr-langs csv
	QuickRedact []string      // passed as --ocr-quick-redact csv; nil = editor default
	OCRTimeout  time.Duration // per recognition, passed as --ocr-timeout; 0 = editor default
	// TesseractPath is config ocr.tesseract_path (Linux), passed as
	// --ocr-tesseract so the editor runs the command the host probed.
	TesseractPath string
	// OnText is called after the helper exits when the user copied text in
	// the editor (the helper wrote --text-out). The host wires it to the
	// clipboard on Linux, where a short-lived process cannot own the
	// selection; it is a no-op elsewhere. It runs before Edit returns, so a
	// later image copy in the pipeline replaces the text, in time order.
	OnText func(text string)
}

// Edit implements Editor by invoking the out-of-process editor helper.
func (l Launcher) Edit(ctx context.Context, in capture.Result, opts Opts) (capture.Result, Action, bool, error) {
	if in.Kind != capture.KindImage {
		return in, ActionDefault, false, nil
	}

	helper, err := l.resolveHelper()
	if err != nil {
		return in, ActionDefault, false, err
	}

	dir, err := os.MkdirTemp("", "goshareit-edit-")
	if err != nil {
		return in, ActionDefault, false, fmt.Errorf("editor: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	inPath := filepath.Join(dir, "in.png")
	outPath := filepath.Join(dir, "out.png")
	textPath := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(inPath, in.Bytes, 0o600); err != nil {
		return in, ActionDefault, false, fmt.Errorf("editor: write input: %w", err)
	}

	if l.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.Timeout)
		defer cancel()
	}

	args := []string{"--in", inPath, "--out", outPath, "--text-out", textPath, "--actions", "--upload-enabled=" + strconv.FormatBool(opts.CanUpload)}
	args = append(args, l.ocrArgs(opts.OCR)...)
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
	if l.Theme != "" {
		args = append(args, "--theme", l.Theme)
	}
	label := l.ConfirmLabel
	if l.ConfirmLabelFor != nil {
		label = l.ConfirmLabelFor(opts.CanUpload)
	}
	if label != "" {
		args = append(args, "--confirm-label", label)
	}

	cmd := exec.CommandContext(ctx, helper, args...)
	runErr := cmd.Run()
	l.handOffText(textPath)
	if runErr == nil {
		return l.readEdited(in, outPath, ActionDefault, true)
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		switch code := exitErr.ExitCode(); code {
		case cancelledExitCode:
			return in, ActionDefault, false, nil
		case copyExitCode:
			return l.readEdited(in, outPath, ActionCopy, true)
		case saveExitCode:
			return l.readEdited(in, outPath, ActionSave, true)
		case uploadExitCode:
			return l.readEdited(in, outPath, ActionUpload, true)
		default:
			return in, ActionDefault, false, fmt.Errorf("editor: helper exit %d", code)
		}
	}

	// Non-ExitError: helper missing, not executable, killed by ctx (timeout),
	// etc. The caller discards the capture on this error (fail-closed).
	return in, ActionDefault, false, fmt.Errorf("editor: run helper: %w", runErr)
}

// ocrArgs renders the text-recognition flags: --ocr=hidden when the user
// turned recognition off, --ocr=off plus the reason when the host's probe
// found no engine, else --ocr=on with the tuning flags.
func (l Launcher) ocrArgs(st ocr.Status) []string {
	if !l.OCREnabled {
		return []string{"--ocr=hidden"}
	}
	if !st.Available {
		return []string{"--ocr=off", "--ocr-reason", st.Explain()}
	}
	args := []string{"--ocr=on", "--ocr-auto=" + strconv.FormatBool(l.OCRAutoRun)}
	if len(l.OCRLangs) > 0 {
		args = append(args, "--ocr-langs", strings.Join(l.OCRLangs, ","))
	}
	if l.QuickRedact != nil {
		// An explicit empty list is passed too: it means "hide nothing".
		args = append(args, "--ocr-quick-redact", strings.Join(l.QuickRedact, ","))
	}
	if l.OCRTimeout > 0 {
		args = append(args, "--ocr-timeout", strconv.Itoa(int(l.OCRTimeout/time.Second)))
	}
	if l.TesseractPath != "" {
		args = append(args, "--ocr-tesseract", l.TesseractPath)
	}
	return args
}

// handOffText passes text the user copied in the editor to OnText. The
// helper rewrites the file on every copy, so it holds the last one.
func (l Launcher) handOffText(path string) {
	if l.OnText == nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return
	}
	l.OnText(string(b))
}

// readEdited reads the edited PNG the helper wrote to outPath. On a read
// failure it returns the original Result, ok=false, action=ActionDefault,
// plus the error, which the caller treats as a failed edit (capture discarded).
func (l Launcher) readEdited(in capture.Result, outPath string, action Action, ok bool) (capture.Result, Action, bool, error) {
	edited, err := os.ReadFile(outPath)
	if err != nil {
		return in, ActionDefault, false, fmt.Errorf("editor: read output: %w", err)
	}
	return capture.Result{
		Path:  "",
		Bytes: edited,
		Mime:  "image/png",
		Kind:  capture.KindImage,
	}, action, ok, nil
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
