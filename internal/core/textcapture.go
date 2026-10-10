package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/png" // capturers return PNG
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// TextUnavailableError is returned by CaptureText when text recognition
// cannot run here; nothing was captured. Status carries the reason and fix.
type TextUnavailableError struct{ Status ocr.Status }

func (e *TextUnavailableError) Error() string {
	return "capture text: " + e.Status.Explain()
}

// TextOutcome describes one Capture Text run.
type TextOutcome struct {
	Cancelled bool // the user backed out of the region overlay
	Busy      bool // another capture was on screen; the press was dropped
	Chars     int  // characters copied; 0 = no text found, clipboard untouched
	Lines     int
}

// defaultTextTimeout bounds one recognition when ocr.timeout_seconds is 0.
const defaultTextTimeout = 20 * time.Second

// textOff is the status while ocr.enabled is false.
var textOff = ocr.Status{Reason: "Text recognition is turned off.", Hint: "Turn it on in Settings > Text recognition."}

// TextStatus reports whether Capture Text can run, from the cached probe
// (no new probe); text recognition turned off in the config counts as
// unavailable. The tray greys its item out with this.
func (a *App) TextStatus() ocr.Status {
	if !a.cfg.OCREnabled() {
		return textOff
	}
	return a.OCRStatus()
}

// CaptureText is the "Capture Text" action: the user picks a region, the
// text in it is recognized on this machine and copied to the clipboard as
// plain text. No editor, no upload, no history, nothing saved. The host's
// clipboard writer holds the text on Linux, where the selection would
// otherwise die with a short-lived process. The text itself is never
// logged.
//
// When recognition is unavailable it returns a *TextUnavailableError before
// anything is captured. A cancelled overlay is quiet (Cancelled, nil).
func (a *App) CaptureText(ctx context.Context) (TextOutcome, error) {
	st := textOff
	if a.cfg.OCREnabled() {
		// On Linux an unavailable engine is probed again first: tesseract
		// may have been installed since.
		st = a.ocrStatusForEditor(ctx)
	}
	if !st.Available {
		return TextOutcome{}, &TextUnavailableError{Status: st}
	}
	// Same one-at-a-time guard as screenshots; released once the region is
	// captured, so a slow recognition never swallows the next press.
	if !a.capturing.CompareAndSwap(false, true) {
		a.log.Info().Msg("capture already in progress; capture text press ignored")
		return TextOutcome{Busy: true}, nil
	}
	// KeepClipboard: no capture path may leave an image on the clipboard
	// (the Windows snip fallback would), since "no text" must leave it as
	// it was.
	res, err := a.capturer.Capture(ctx, capture.Request{Mode: capture.RegionInteractive, KeepClipboard: true})
	a.capturing.Store(false)
	if errors.Is(err, capture.ErrCancelled) {
		a.log.Info().Msg("capture text cancelled by user")
		return TextOutcome{Cancelled: true}, nil
	}
	if err != nil {
		return TextOutcome{}, fmt.Errorf("capture: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(res.Bytes))
	if err != nil {
		return TextOutcome{}, fmt.Errorf("capture: decode image: %w", err)
	}

	timeout := time.Duration(a.cfg.OCR.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTextTimeout
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r, err := a.ocr.Recognize(rctx, img, ocr.Options{Langs: a.cfg.OCR.Languages})
	if err != nil {
		if errors.Is(rctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return TextOutcome{}, fmt.Errorf("text recognition: took longer than %s", timeout)
		}
		return TextOutcome{}, fmt.Errorf("text recognition: %w", err)
	}
	text := r.Text()
	if strings.TrimSpace(text) == "" {
		a.log.Info().Str("engine", r.Engine).Msg("capture text: no text found")
		return TextOutcome{}, nil
	}
	if err := a.clipboard.WriteText(text); err != nil {
		return TextOutcome{}, fmt.Errorf("clipboard: %w", err)
	}
	out := TextOutcome{Chars: utf8.RuneCountInString(text), Lines: len(r.Lines)}
	a.log.Info().Int("chars", out.Chars).Int("lines", out.Lines).Str("engine", r.Engine).Msg("capture text copied")
	return out, nil
}
