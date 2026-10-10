//go:build darwin && cgo

// Package vision is the macOS OCR engine: Apple Vision's
// VNRecognizeTextRequest (revision 3, accurate level, language correction) in
// a small Objective-C shim (vision.m / vision.h), following the cgo pattern of
// platform/darwin/recorder.m. It is a separate package so the editor does not
// link the AVFoundation recorder. Recognition runs entirely on device.
//
// Threading: performRequests:error: is synchronous and has no thread
// affinity; it runs on the calling goroutine (never the main thread) inside
// an @autoreleasepool. Vision has no cancel for a synchronous request, so
// Recognize runs the C call on an inner goroutine and returns ctx.Err()
// early when ctx ends; the C call then finishes on its own and its result is
// dropped.
package vision

/*
#cgo LDFLAGS: -framework Foundation -framework Vision -framework CoreGraphics
#include <stdlib.h>
#include "vision.h"
*/
import "C"

import (
	"context"
	"errors"
	"image"
	"image/draw"
	"math"
	"strings"
	"sync"
	"unsafe"

	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// shim serializes calls into the C shim, whose last-error buffer is global.
var shim sync.Mutex

// Engine recognizes text with Apple Vision.
type Engine struct {
	langs []string

	// supported caches Probe's language list for mapping configured tags.
	supportedOnce sync.Once
	supported     []string
}

// New returns a Vision engine. langs are BCP-47 tags; empty = automatic
// language detection. Configured tags are matched against the tags Vision
// lists ("en" -> "en-US", script-aware), see visionLangs.
func New(langs []string) *Engine { return &Engine{langs: langs} }

// Probe lists the languages Vision supports (revision 3); an empty list
// means unavailable.
func (e *Engine) Probe(context.Context) ocr.Status {
	shim.Lock()
	defer shim.Unlock()
	var out *C.char
	n := C.gsi_vision_languages(&out)
	if out != nil {
		defer C.free(unsafe.Pointer(out))
	}
	if n <= 0 || out == nil {
		why := C.GoString(C.gsi_vision_last_error())
		if why == "" {
			why = "Apple Vision reported no text recognition languages."
		}
		return ocr.Status{Reason: why}
	}
	langs := make([]string, 0, int(n))
	p := unsafe.Pointer(out)
	for i := 0; i < int(n); i++ {
		s := C.GoString((*C.char)(p))
		if s == "" {
			break
		}
		langs = append(langs, s)
		p = unsafe.Add(p, len(s)+1)
	}
	return ocr.Status{Available: true, Engine: ocr.EngineVision, Langs: langs}
}

// Recognize converts img to RGBA, hands a C copy of the pixels to Vision and
// copies the lines and words back. It returns ctx.Err() as soon as ctx ends.
func (e *Engine) Recognize(ctx context.Context, img image.Image, opts ocr.Options) (ocr.Result, error) {
	want := opts.Langs
	if len(want) == 0 {
		want = e.langs
	}
	langs := e.visionLangs(ctx, want)
	rgba := toRGBA(img)
	type outcome struct {
		res ocr.Result
		err error
	}
	ch := make(chan outcome, 1)
	go func() {
		res, err := recognize(rgba, langs, opts.Fast)
		ch <- outcome{res, err}
	}()
	select {
	case <-ctx.Done():
		return ocr.Result{}, ctx.Err()
	case o := <-ch:
		if o.err != nil {
			return ocr.Result{}, o.err
		}
		o.res.Langs = langs // nil = automatic detection
		off := img.Bounds().Min
		for i := range o.res.Lines {
			o.res.Lines[i].Rect = o.res.Lines[i].Rect.Add(off)
			for j := range o.res.Lines[i].Words {
				o.res.Lines[i].Words[j].Rect = o.res.Lines[i].Words[j].Rect.Add(off)
			}
		}
		return o.res, nil
	}
}

// visionLangs maps the configured tags to tags Vision lists as supported
// (exact, then same language and script, then same language). Tags Vision
// does not list are dropped; when none is left, recognition falls back to
// automatic detection with a warning, instead of failing every capture.
func (e *Engine) visionLangs(ctx context.Context, want []string) []string {
	if len(want) == 0 {
		return nil
	}
	e.supportedOnce.Do(func() { e.supported = e.Probe(ctx).Langs })
	got := ocr.MatchTags(want, e.supported)
	if len(got) == 0 {
		log.Warn().Strs("configured", want).Msg("none of the configured OCR languages is supported by Apple Vision; using automatic language detection")
		return nil
	}
	if len(got) < len(want) {
		log.Warn().Strs("configured", want).Strs("used", got).Msg("some configured OCR languages are not supported by Apple Vision")
	}
	return got
}

// toRGBA returns img as a zero-origin *image.RGBA, reusing it when it
// already is one.
func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	b := img.Bounds()
	r := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(r, r.Rect, img, b.Min, draw.Src)
	return r
}

func recognize(img *image.RGBA, langs []string, fast bool) (ocr.Result, error) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w == 0 || h == 0 {
		return ocr.Result{Engine: ocr.EngineVision}, nil
	}
	// A C copy keeps Go memory out of CoreGraphics' hands (the shim copies
	// again into a CFData the CGImage owns).
	pix := C.CBytes(img.Pix[:img.Stride*h])
	defer C.free(pix)

	var clangs *C.char
	if len(langs) > 0 {
		clangs = C.CString(strings.Join(langs, "\x00") + "\x00")
		defer C.free(unsafe.Pointer(clangs))
	}
	cfast := C.int(0)
	if fast {
		cfast = 1
	}

	shim.Lock()
	defer shim.Unlock()
	var res *C.gsi_vision_result
	rc := C.gsi_vision_recognize((*C.uchar)(pix), C.int(w), C.int(h), C.int(img.Stride),
		clangs, C.int(len(langs)), cfast, &res)
	if rc != 0 || res == nil {
		msg := C.GoString(C.gsi_vision_last_error())
		if msg == "" {
			msg = "text recognition failed"
		}
		return ocr.Result{}, errors.New("vision: " + msg)
	}
	defer C.gsi_vision_free(res)

	out := ocr.Result{Engine: ocr.EngineVision}
	if res.line_count == 0 {
		return out, nil
	}
	bounds := image.Rect(0, 0, w, h)
	lines := unsafe.Slice(res.lines, int(res.line_count))
	for _, cl := range lines {
		l := ocr.Line{
			Text: C.GoString(cl.text),
			Rect: pxRect(cl.x, cl.y, cl.w, cl.h).Intersect(bounds),
		}
		if cl.word_count > 0 {
			for _, cw := range unsafe.Slice(cl.words, int(cl.word_count)) {
				l.Words = append(l.Words, ocr.Word{
					Text: C.GoString(cw.text),
					Rect: pxRect(cw.x, cw.y, cw.w, cw.h).Intersect(bounds),
					Conf: float32(cw.conf),
				})
			}
		}
		out.Lines = append(out.Lines, l)
	}
	return out, nil
}

// pxRect rounds a float pixel box outward.
func pxRect(x, y, w, h C.float) image.Rectangle {
	return image.Rect(
		int(math.Floor(float64(x))), int(math.Floor(float64(y))),
		int(math.Ceil(float64(x+w))), int(math.Ceil(float64(y+h))),
	)
}
