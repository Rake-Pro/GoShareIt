//go:build linux

package linux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kbinani/screenshot"
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/region"
)

// ErrCaptureCancelled is returned when the user dismisses an interactive
// capture (the region overlay or the portal picker) without selecting
// anything. The pipeline treats it as a no-op rather than an error.
var ErrCaptureCancelled = fmt.Errorf("linux capture: %w", capture.ErrCancelled)

// ErrUnsupportedMode is returned for modes the still-image Capturer does not
// implement (video and GIF belong to the Recorder).
var ErrUnsupportedMode = errors.New("linux capture: mode not supported")

// Capturer captures still images: directly from the X server on X11, through
// the XDG desktop portal on Wayland.
type Capturer struct {
	// Region, when set, is the app's own overlay (goshareit-editor --region)
	// used for interactive selection. Without it a Wayland session falls back
	// to the portal's interactive picker and X11 has no region mode.
	Region region.Selector

	wayland bool

	mu sync.Mutex
	// lastRegion holds the most recent overlay-selected rect for LastRegion
	// replay, in capture coordinates (xinerama-relative on X11, portal image
	// pixels on Wayland).
	lastRegion image.Rectangle
}

// NewCapturer returns a Capturer for the current session type.
func NewCapturer() *Capturer {
	return &Capturer{wayland: IsWayland()}
}

// Capabilities advertises the still-image modes.
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

	img, err := c.grab(ctx, r.Mode)
	if err != nil {
		return capture.Result{}, err
	}
	pngBytes, err := encodePNG(img)
	if err != nil {
		return capture.Result{}, err
	}

	res := capture.Result{
		Bytes: pngBytes,
		Mime:  "image/png",
		Kind:  capture.KindImage,
	}
	if r.SaveLocal {
		path, err := savePath(r)
		if err != nil {
			return capture.Result{}, err
		}
		if err := os.WriteFile(path, pngBytes, 0o644); err != nil {
			return capture.Result{}, fmt.Errorf("linux capture: write file: %w", err)
		}
		res.Path = path
	}
	return res, nil
}

// grab dispatches on mode.
func (c *Capturer) grab(ctx context.Context, mode capture.Mode) (image.Image, error) {
	switch mode {
	case capture.FullScreen:
		return c.fullScreen(ctx)
	case capture.ActiveWindow, capture.WindowPick:
		if c.wayland {
			// Wayland clients cannot see other windows; the compositor's own
			// picker (window mode included) is the only window capture.
			return portalScreenshot(ctx, true)
		}
		// X11 has no "click a window" picker, so both modes capture the
		// focused window, like the Windows backend.
		r, err := activeWindowRect()
		if err != nil {
			return nil, err
		}
		origin, err := primaryOrigin()
		if err != nil {
			return nil, err
		}
		return captureRect(r.Sub(origin))
	case capture.RegionInteractive:
		return c.interactive(ctx)
	case capture.LastRegion:
		c.mu.Lock()
		last := c.lastRegion
		c.mu.Unlock()
		if last.Empty() {
			return c.interactive(ctx)
		}
		if c.wayland {
			frame, err := portalScreenshot(ctx, false)
			if err != nil {
				return nil, err
			}
			return crop(frame, last)
		}
		return captureRect(last)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedMode, mode)
	}
}

// fullScreen grabs the whole desktop: the union of the xinerama screens on
// X11, the portal's stitched image on Wayland.
func (c *Capturer) fullScreen(ctx context.Context) (image.Image, error) {
	if c.wayland {
		return portalScreenshot(ctx, false)
	}
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		return nil, errors.New("linux capture: no X11 displays found (is DISPLAY set?)")
	}
	bounds := screenshot.GetDisplayBounds(0)
	for i := 1; i < n; i++ {
		bounds = bounds.Union(screenshot.GetDisplayBounds(i))
	}
	return captureRect(bounds)
}

// interactive runs the app's own region overlay over a frozen frame and crops
// the selection from that same frame, so the overlay never ends up in the
// capture. Without an overlay, Wayland falls back to the portal picker.
func (c *Capturer) interactive(ctx context.Context) (image.Image, error) {
	if c.Region == nil {
		if c.wayland {
			return portalScreenshot(ctx, true)
		}
		return nil, errors.New("linux capture: no region selector wired")
	}
	frame, err := c.freeze(ctx)
	var screen image.Image // stays a true nil interface when the grab failed
	if err != nil {
		log.Warn().Err(err).Msg("region: could not freeze the screen; overlay will be plain")
		frame = nil
	} else {
		screen = frame
	}
	rect, ok, err := c.Region.Select(ctx, screen)
	switch {
	case err != nil:
		if c.wayland {
			log.Warn().Err(err).Msg("region overlay failed; falling back to the portal picker")
			return portalScreenshot(ctx, true)
		}
		return nil, err
	case !ok:
		return nil, ErrCaptureCancelled
	case frame != nil:
		img, err := crop(frame, rect)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.lastRegion = rect
		c.mu.Unlock()
		return img, nil
	default:
		if c.wayland {
			return nil, errors.New("linux capture: no frozen frame to crop the selection from")
		}
		// Let the overlay window disappear, then grab live.
		time.Sleep(150 * time.Millisecond)
		img, err := captureRect(rect)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.lastRegion = rect
		c.mu.Unlock()
		return img, nil
	}
}

// freeze returns the frame the region overlay draws as its backdrop: the
// first xinerama screen on X11 (where the fullscreen overlay opens), the
// whole desktop on Wayland.
func (c *Capturer) freeze(ctx context.Context) (image.Image, error) {
	if c.wayland {
		return portalScreenshot(ctx, false)
	}
	if screenshot.NumActiveDisplays() <= 0 {
		return nil, errors.New("linux capture: no X11 displays found (is DISPLAY set?)")
	}
	return captureRect(screenshot.GetDisplayBounds(0))
}

// FreezePrimaryDisplay returns a frame for the record-region overlay's
// backdrop (the host's record-region path).
func FreezePrimaryDisplay() (image.Image, error) {
	return NewCapturer().freeze(context.Background())
}

func captureRect(r image.Rectangle) (image.Image, error) {
	if r.Empty() {
		return nil, fmt.Errorf("linux capture: empty rectangle %v", r)
	}
	img, err := screenshot.CaptureRect(r)
	if err != nil {
		return nil, fmt.Errorf("linux capture: grab %v: %w", r, err)
	}
	return img, nil
}

// crop returns the part of frame inside r as a new image.
func crop(frame image.Image, r image.Rectangle) (image.Image, error) {
	r = r.Intersect(frame.Bounds())
	if r.Empty() {
		return nil, fmt.Errorf("linux capture: region %v outside the frozen frame %v", r, frame.Bounds())
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), frame, r.Min, draw.Src)
	return out, nil
}

func savePath(r capture.Request) (string, error) {
	name := fmt.Sprintf("goshareit_%d.png", time.Now().UnixNano())
	dir := os.TempDir()
	if r.SaveDir != "" {
		if err := os.MkdirAll(r.SaveDir, 0o755); err != nil {
			return "", fmt.Errorf("linux capture: create save dir: %w", err)
		}
		dir = r.SaveDir
	}
	return filepath.Join(dir, name), nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("linux capture: png encode: %w", err)
	}
	return buf.Bytes(), nil
}
