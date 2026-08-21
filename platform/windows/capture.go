//go:build windows

// Package windows provides the Windows platform backends for the GoShareIt core
// seams. All files are guarded by the windows build tag so the portable linux
// build excludes them. P2 implements still-image screenshots only; recording
// (video/GIF) is deferred to P3 and reported as unsupported, mirroring darwin.
package windows

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/kbinani/screenshot"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/windows"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/region"
)

// ErrCaptureCancelled is returned when the user dismisses an interactive capture
// without taking a snip. Windows has no equivalent of `screencapture`'s "exit 0
// with no file" signal: ms-screenclip simply never places a new image on the
// clipboard, so we detect cancellation as a polling timeout. The pipeline treats
// this as a no-op rather than an error.
var ErrCaptureCancelled = errors.New("windows capture: cancelled by user")

// ErrUnsupportedMode is returned for modes not implemented in Phase 2.
var ErrUnsupportedMode = errors.New("windows capture: mode not implemented in P2")

// snipTimeout bounds how long RegionInteractive waits for the user to complete
// an ms-screenclip snip before giving up with ErrCaptureCancelled.
const snipTimeout = 60 * time.Second

// snipPollInterval is how often the clipboard is polled while waiting for a snip.
const snipPollInterval = 300 * time.Millisecond

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procGetForeground      = user32.NewProc("GetForegroundWindow")
	procGetWindowRect      = user32.NewProc("GetWindowRect")
	procSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
)

// rect mirrors the Win32 RECT structure for GetWindowRect.
type rect struct {
	left, top, right, bottom int32
}

// Capturer captures still images via Win32 GDI (through kbinani/screenshot) and,
// for interactive selection, the built-in Windows 10/11 screen snip tool.
type Capturer struct {
	// Region, when set, is the app's own overlay (goshareit-editor --region)
	// used for interactive selection; the Windows snip UI is only a fallback.
	Region region.Selector

	mu sync.Mutex
	// lastRegion holds the most recent overlay-selected rect for LastRegion
	// replay; the ms-screenclip fallback cannot report a rect, so it stays
	// empty on that path and LastRegion falls back to RegionInteractive.
	lastRegion image.Rectangle
}

// NewCapturer returns a Windows GDI/screen-snip backed Capturer.
func NewCapturer() *Capturer {
	// Best-effort: opt into DPI awareness so captured rects match physical
	// pixels on high-DPI displays. Ignored on failure (already-aware process).
	_, _, _ = procSetProcessDPIAware.Call()
	return &Capturer{}
}

// Capabilities advertises only the still-image modes supported in P2.
func (c *Capturer) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{
		capture.RegionInteractive,
		capture.FullScreen,
		capture.ActiveWindow,
		capture.WindowPick,
		capture.LastRegion,
	}}
}

// Capture performs the requested capture and returns PNG bytes.
func (c *Capturer) Capture(ctx context.Context, r capture.Request) (capture.Result, error) {
	switch r.Mode {
	case capture.VideoRegion, capture.VideoFull, capture.GIF:
		return capture.Result{}, fmt.Errorf("%w: %s", ErrUnsupportedMode, r.Mode)
	}

	pngBytes, err := c.grab(ctx, r.Mode)
	if err != nil {
		return capture.Result{}, err
	}

	res := capture.Result{
		Bytes: pngBytes,
		Mime:  "image/png",
		Kind:  capture.KindImage,
	}

	if r.SaveLocal {
		path, err := c.savePath(r)
		if err != nil {
			return capture.Result{}, err
		}
		if err := os.WriteFile(path, pngBytes, 0o644); err != nil {
			return capture.Result{}, fmt.Errorf("windows capture: write file: %w", err)
		}
		res.Path = path
	}
	return res, nil
}

// grab dispatches on mode and returns encoded PNG bytes.
func (c *Capturer) grab(ctx context.Context, mode capture.Mode) ([]byte, error) {
	switch mode {
	case capture.FullScreen:
		return c.captureVirtualScreen()
	case capture.ActiveWindow, capture.WindowPick:
		// Windows has no "click a window" picker analogous to screencapture -w,
		// so both ActiveWindow and WindowPick map to the current foreground
		// window. The user is expected to focus the target before triggering.
		return c.captureForegroundWindow()
	case capture.RegionInteractive:
		return c.captureInteractive(ctx)
	case capture.LastRegion:
		c.mu.Lock()
		last := c.lastRegion
		c.mu.Unlock()
		if last.Dx() > 0 && last.Dy() > 0 {
			return encodePNG(mustCaptureRect(last))
		}
		// No stored rect: fall back to interactive selection. TODO(P2) above.
		return c.captureInteractive(ctx)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedMode, mode)
	}
}

// captureVirtualScreen grabs the union of all active displays (the full virtual
// desktop) via GDI BitBlt.
func (c *Capturer) captureVirtualScreen() ([]byte, error) {
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		return nil, fmt.Errorf("windows capture: no active displays")
	}
	bounds := screenshot.GetDisplayBounds(0)
	for i := 1; i < n; i++ {
		bounds = bounds.Union(screenshot.GetDisplayBounds(i))
	}
	img, err := screenshot.Capture(bounds.Min.X, bounds.Min.Y, bounds.Dx(), bounds.Dy())
	if err != nil {
		return nil, fmt.Errorf("windows capture: virtual screen BitBlt: %w", err)
	}
	return encodePNG(img)
}

// captureForegroundWindow grabs the rect of the current foreground window.
func (c *Capturer) captureForegroundWindow() ([]byte, error) {
	hwnd, _, _ := procGetForeground.Call()
	if hwnd == 0 {
		return nil, fmt.Errorf("windows capture: no foreground window")
	}
	var rc rect
	ret, _, err := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	if ret == 0 {
		return nil, fmt.Errorf("windows capture: GetWindowRect: %v", err)
	}
	r := image.Rect(int(rc.left), int(rc.top), int(rc.right), int(rc.bottom))
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return nil, fmt.Errorf("windows capture: empty foreground window rect")
	}
	img, err := screenshot.CaptureRect(r)
	if err != nil {
		return nil, fmt.Errorf("windows capture: window BitBlt: %w", err)
	}
	return encodePNG(img)
}

// captureInteractive launches the Windows screen-snip tool (ms-screenclip:),
// which lets the user select a region and places the result on the clipboard.
// It is the Windows analog of `screencapture -i -c`. We snapshot the clipboard
// image beforehand, launch the snip UI, then poll the clipboard until a NEW
// image appears or snipTimeout elapses (treated as user cancellation).
func (c *Capturer) captureInteractive(ctx context.Context) ([]byte, error) {
	if c.Region != nil {
		rect, ok, err := c.Region.Select(ctx)
		switch {
		case err != nil:
			log.Warn().Err(err).Msg("region overlay failed; falling back to Windows snip UI")
		case !ok:
			return nil, ErrCaptureCancelled
		default:
			// Let the overlay window disappear before grabbing the pixels.
			time.Sleep(150 * time.Millisecond)
			img, err := screenshot.CaptureRect(rect)
			if err != nil {
				return nil, fmt.Errorf("windows capture: region BitBlt: %w", err)
			}
			c.mu.Lock()
			c.lastRegion = rect
			c.mu.Unlock()
			return encodePNG(img)
		}
	}
	if err := clipboardInit(); err != nil {
		return nil, fmt.Errorf("windows capture: clipboard init: %w", err)
	}

	before := readClipboardImage()

	// explorer.exe handles the ms-screenclip: protocol and returns immediately;
	// the snip overlay runs asynchronously and writes to the clipboard.
	if err := exec.Command("explorer.exe", "ms-screenclip:").Start(); err != nil {
		return nil, fmt.Errorf("windows capture: launch ms-screenclip: %w", err)
	}

	deadline := time.Now().Add(snipTimeout)
	ticker := time.NewTicker(snipPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			cur := readClipboardImage()
			if len(cur) > 0 && !bytes.Equal(cur, before) {
				return cur, nil
			}
			if time.Now().After(deadline) {
				return nil, ErrCaptureCancelled
			}
		}
	}
}

// readClipboardImage returns the current clipboard image as PNG bytes, or nil.
func readClipboardImage() []byte {
	b, ok := readImage()
	if !ok {
		return nil
	}
	return b
}

// savePath returns the target file path for a saved capture, creating SaveDir.
func (c *Capturer) savePath(r capture.Request) (string, error) {
	name := fmt.Sprintf("goshareit_%d.png", time.Now().UnixNano())
	dir := os.TempDir()
	if r.SaveDir != "" {
		if err := os.MkdirAll(r.SaveDir, 0o755); err != nil {
			return "", fmt.Errorf("windows capture: create save dir: %w", err)
		}
		dir = r.SaveDir
	}
	return filepath.Join(dir, name), nil
}

// encodePNG encodes an image to PNG bytes.
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("windows capture: png encode: %w", err)
	}
	return buf.Bytes(), nil
}

// mustCaptureRect is a small helper for LastRegion replay; on error it returns a
// 1x1 transparent image so encodePNG still produces valid output. The caller's
// fallback to interactive selection covers the meaningful failure path.
func mustCaptureRect(r image.Rectangle) image.Image {
	if img, err := screenshot.CaptureRect(r); err == nil {
		return img
	}
	return image.NewRGBA(image.Rect(0, 0, 1, 1))
}
