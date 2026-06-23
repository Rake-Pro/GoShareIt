package gifrec

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// colorCapturer returns distinct solid-color PNG frames of a fixed size.
type colorCapturer struct {
	w, h int
	n    int32
}

func (c *colorCapturer) Capture(_ context.Context, _ capture.Request) (capture.Result, error) {
	i := atomic.AddInt32(&c.n, 1)
	img := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	col := color.RGBA{R: uint8(i * 40), G: uint8(i * 20), B: uint8(255 - i*10), A: 255}
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			img.Set(x, y, col)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return capture.Result{}, err
	}
	return capture.Result{Bytes: buf.Bytes(), Mime: "image/png", Kind: capture.KindImage}, nil
}

func (c *colorCapturer) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{capture.FullScreen}}
}

func TestRecordGIF(t *testing.T) {
	const w, h = 16, 12
	cap := &colorCapturer{w: w, h: h}
	r := New(cap, 30, 100)

	if err := r.Start(context.Background(), capture.GIF); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !r.Recording() {
		t.Fatal("Recording() should be true after Start")
	}
	time.Sleep(200 * time.Millisecond)

	res, err := r.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if r.Recording() {
		t.Fatal("Recording() should be false after Stop")
	}
	if res.Mime != "image/gif" {
		t.Errorf("Mime = %q, want image/gif", res.Mime)
	}
	if res.Kind != capture.KindVideo {
		t.Errorf("Kind = %v, want KindVideo", res.Kind)
	}

	g, err := gif.DecodeAll(bytes.NewReader(res.Bytes))
	if err != nil {
		t.Fatalf("DecodeAll: %v", err)
	}
	if len(g.Image) < 2 {
		t.Fatalf("frames = %d, want >=2", len(g.Image))
	}
	for i, im := range g.Image {
		if b := im.Bounds(); b.Dx() != w || b.Dy() != h {
			t.Errorf("frame %d bounds = %v, want %dx%d", i, b, w, h)
		}
	}
}

func TestRecordGIFRegion(t *testing.T) {
	const w, h = 40, 30
	cap := &colorCapturer{w: w, h: h}
	r := New(cap, 30, 100)

	rect := image.Rect(5, 6, 25, 21) // 20x15 crop
	if err := r.StartRegion(context.Background(), capture.GIF, rect); err != nil {
		t.Fatalf("StartRegion: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	res, err := r.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	g, err := gif.DecodeAll(bytes.NewReader(res.Bytes))
	if err != nil {
		t.Fatalf("DecodeAll: %v", err)
	}
	if len(g.Image) < 1 {
		t.Fatalf("frames = %d, want >=1", len(g.Image))
	}
	for i, im := range g.Image {
		if b := im.Bounds(); b.Dx() != rect.Dx() || b.Dy() != rect.Dy() {
			t.Errorf("frame %d bounds = %v, want %dx%d", i, b, rect.Dx(), rect.Dy())
		}
	}
}

func TestStartNonGIFMode(t *testing.T) {
	r := New(&colorCapturer{w: 4, h: 4}, 10, 10)
	if err := r.Start(context.Background(), capture.VideoFull); err == nil {
		t.Fatal("expected error for non-GIF mode")
	}
	if r.Recording() {
		t.Fatal("should not be recording after failed Start")
	}
}

func TestStopWithoutStart(t *testing.T) {
	r := New(&colorCapturer{w: 4, h: 4}, 10, 10)
	if _, err := r.Stop(context.Background()); err != capture.ErrNotRecording {
		t.Fatalf("Stop without Start = %v, want ErrNotRecording", err)
	}
}

func TestDoubleStart(t *testing.T) {
	r := New(&colorCapturer{w: 4, h: 4}, 10, 10)
	if err := r.Start(context.Background(), capture.GIF); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer r.Stop(context.Background())
	if err := r.Start(context.Background(), capture.GIF); err != capture.ErrAlreadyRecording {
		t.Fatalf("second Start = %v, want ErrAlreadyRecording", err)
	}
}
