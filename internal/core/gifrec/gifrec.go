// Package gifrec implements capture.Recorder by sampling screenshots from a
// Capturer on a ticker and encoding them into an animated GIF with image/gif.
// It is pure Go (no ffmpeg, no cgo) so it works on platforms with no external
// encoder available (notably macOS).
package gifrec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"sync"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// Recorder records an animated GIF by periodically capturing full-screen PNG
// frames and encoding them on Stop.
type Recorder struct {
	cap       capture.Capturer
	fps       int
	maxFrames int

	mu        sync.Mutex
	recording bool
	frames    []image.Image
	rect      image.Rectangle // empty => full frame
	cancel    context.CancelFunc
	done      chan struct{}
}

// New builds a GIF Recorder over the given Capturer. fps<=0 defaults to 10 and
// is clamped to <=30; maxFrames<=0 defaults to 300 (bounds memory, ~30s @10fps).
func New(c capture.Capturer, fps int, maxFrames int) *Recorder {
	if fps <= 0 {
		fps = 10
	}
	if fps > 30 {
		fps = 30
	}
	if maxFrames <= 0 {
		maxFrames = 300
	}
	return &Recorder{cap: c, fps: fps, maxFrames: maxFrames}
}

// Start begins sampling frames for the GIF mode. Only capture.GIF is accepted.
func (r *Recorder) Start(ctx context.Context, mode capture.Mode) error {
	return r.StartRegion(ctx, mode, image.Rectangle{})
}

// StartRegion begins sampling frames cropped to rect (screen pixel coords,
// top-left origin). An empty rect records the full frame, equivalent to Start.
// Only capture.GIF is accepted.
func (r *Recorder) StartRegion(ctx context.Context, mode capture.Mode, rect image.Rectangle) error {
	if mode != capture.GIF {
		return fmt.Errorf("gifrec: unsupported mode %s (only GIF)", mode)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recording {
		return capture.ErrAlreadyRecording
	}
	r.recording = true
	r.frames = nil
	r.rect = rect.Canon()

	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	done := make(chan struct{})
	r.done = done

	fps := r.fps
	maxFrames := r.maxFrames
	intervalMS := 1000 / fps

	go r.loop(runCtx, done, intervalMS, maxFrames, r.rect)
	return nil
}

func (r *Recorder) loop(ctx context.Context, done chan struct{}, intervalMS, maxFrames int, rect image.Rectangle) {
	defer close(done)
	tick := time.NewTicker(time.Duration(intervalMS) * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			res, err := r.cap.Capture(ctx, capture.Request{Mode: capture.FullScreen})
			if err != nil {
				continue
			}
			img, err := png.Decode(bytes.NewReader(res.Bytes))
			if err != nil {
				continue
			}
			if !rect.Empty() {
				img = cropImage(img, rect)
			}
			r.mu.Lock()
			if len(r.frames) < maxFrames {
				r.frames = append(r.frames, img)
			}
			full := len(r.frames) >= maxFrames
			r.mu.Unlock()
			if full {
				return
			}
		}
	}
}

// Stop ends sampling and encodes the collected frames into an animated GIF.
func (r *Recorder) Stop(ctx context.Context) (capture.Result, error) {
	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return capture.Result{}, capture.ErrNotRecording
	}
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	r.mu.Lock()
	frames := r.frames
	r.frames = nil
	r.recording = false
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()

	if len(frames) == 0 {
		return capture.Result{}, errors.New("gifrec: no frames captured")
	}

	delay := 100 / r.fps
	if delay < 2 {
		delay = 2
	}

	out := &gif.GIF{
		Image:     make([]*image.Paletted, 0, len(frames)),
		Delay:     make([]int, 0, len(frames)),
		LoopCount: 0,
	}
	for _, f := range frames {
		p := toPaletted(f)
		out.Image = append(out.Image, p)
		out.Delay = append(out.Delay, delay)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil {
		return capture.Result{}, fmt.Errorf("gifrec: encode gif: %w", err)
	}
	return capture.Result{
		Bytes: buf.Bytes(),
		Mime:  "image/gif",
		Kind:  capture.KindVideo,
	}, nil
}

// cropImage returns the portion of src inside rect (intersected with src's
// bounds) as a new origin-normalized RGBA image. Pure Go pixel copy; if the
// intersection is empty it returns src unchanged.
func cropImage(src image.Image, rect image.Rectangle) image.Image {
	sb := src.Bounds()
	// rect is in screen pixels with a top-left origin; src frames are full-screen
	// captures whose bounds also originate at sb.Min, so translate rect into src
	// space before intersecting.
	r := rect.Add(sb.Min).Intersect(sb)
	if r.Empty() {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}

// toPaletted converts an arbitrary image into a paletted image using the Plan9
// palette with Floyd-Steinberg dithering.
func toPaletted(src image.Image) *image.Paletted {
	b := src.Bounds()
	dst := image.NewPaletted(b, palette.Plan9)
	draw.FloydSteinberg.Draw(dst, b, src, b.Min)
	return dst
}

// Recording reports whether a recording is in progress.
func (r *Recorder) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

// Capabilities advertises GIF support.
func (r *Recorder) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{capture.GIF}}
}
