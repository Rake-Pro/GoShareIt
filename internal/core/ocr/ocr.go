// Package ocr is the text-recognition seam. Pure Go, CGO off, unit-tested in
// the core CI job. Engines live in platform packages behind build tags and
// are chosen by internal/core/ocr/engines.
//
// Everything runs on the user's machine: Apple Vision on macOS, Windows OCR
// on Windows and the tesseract command (a local subprocess) on Linux.
package ocr

import (
	"context"
	"errors"
	"image"
)

// ErrUnavailable is returned by Recognize on an engine whose Probe reported
// Available=false. Callers never show it; they show Status.Reason.
var ErrUnavailable = errors.New("ocr: engine unavailable")

// Engine names reported in Result.Engine and Status.Engine.
const (
	EngineVision    = "apple-vision"
	EngineWindows   = "windows-media-ocr"
	EngineTesseract = "tesseract-cli"
)

// Word is one recognized word. Rect is in the input image's pixel space
// (top-left origin), already mapped back through any engine-side scaling.
type Word struct {
	Text string
	Rect image.Rectangle
	Conf float32 // 0..1; engines without a confidence report 1
}

// Line groups words in reading order. Text is the engine's own line text
// when it has one (Windows OcrLine.Text, Vision topCandidates[0].string),
// else JoinWords(Words).
type Line struct {
	Text  string
	Rect  image.Rectangle
	Words []Word
}

// Result is one recognition pass over one image.
type Result struct {
	Engine string   // EngineVision | EngineWindows | EngineTesseract
	Langs  []string // BCP-47 tags actually used (tesseract codes are mapped back)
	Lines  []Line
}

// Status is what the UI shows. Available=false must carry a user-facing
// Reason and, when the user can fix it, a Hint. Both are plain sentences.
type Status struct {
	Available bool
	Engine    string   // as Result.Engine, "" when unavailable
	Version   string   // "5.3.4" for tesseract, "" otherwise
	Langs     []string // recognizer languages installed/supported (BCP-47)
	Reason    string   // "Tesseract is not installed."
	Hint      string   // "Install it with your package manager: ..."
}

// Explain returns Reason and Hint as one sentence pair, for a hover text or
// a Settings status line.
func (s Status) Explain() string {
	switch {
	case s.Reason == "":
		return s.Hint
	case s.Hint == "":
		return s.Reason
	}
	return s.Reason + " " + s.Hint
}

// Options tunes one Recognize call.
type Options struct {
	Langs []string // empty = engine default (profile languages / auto-detect / eng)
	Fast  bool     // Vision .fast level; ignored elsewhere
}

// Engine recognizes text in an image.
type Engine interface {
	// Probe is cheap and side-effect free. Callers bound it with ctx (3 s).
	Probe(ctx context.Context) Status
	// Recognize honors ctx: tesseract kills the process, Windows calls
	// IAsyncInfo.Cancel, Vision returns early and lets the C call finish.
	Recognize(ctx context.Context, img image.Image, opts Options) (Result, error)
}

// Unavailable is the engine for builds and platforms without OCR. It keeps
// every call site branch-free: Probe explains, Recognize refuses.
type Unavailable struct{ Why, Hint string }

// Probe reports the engine as unavailable with its reason.
func (u Unavailable) Probe(context.Context) Status {
	return Status{Reason: u.Why, Hint: u.Hint}
}

// Recognize always fails with ErrUnavailable.
func (u Unavailable) Recognize(context.Context, image.Image, Options) (Result, error) {
	return Result{}, ErrUnavailable
}
