//go:build windows

package winocr

import (
	"context"
	"fmt"
	"image"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/platform/windows/winocr/internal/winrt/foundation"
	"github.com/Rake-Pro/GoShareIt/platform/windows/winocr/internal/winrt/foundation/collections"
	"github.com/Rake-Pro/GoShareIt/platform/windows/winocr/internal/winrt/globalization"
	"github.com/Rake-Pro/GoShareIt/platform/windows/winocr/internal/winrt/graphics/imaging"
	wocr "github.com/Rake-Pro/GoShareIt/platform/windows/winocr/internal/winrt/media/ocr"
	"github.com/Rake-Pro/GoShareIt/platform/windows/winocr/internal/winrt/security/cryptography"
)

// User-facing reasons and hints.
const (
	reasonNoLanguage = "No OCR language is installed on Windows."
	hintNoLanguage   = "Settings > Time & language > Language & region > your language > Language options > install Optical character recognition, then restart GoShareIt."
	reasonNoAPI      = "Windows OCR is not available on this Windows version."
)

// fallbackMaxDim is used when OcrEngine.MaxImageDimension cannot be read:
// the value Windows has reported in practice, so an oversized capture is
// still downscaled instead of failing with an HRESULT.
const fallbackMaxDim = 2600

// Engine recognizes text with Windows.Media.Ocr, CGO off, through the
// generated bindings in internal/winrt. Every WinRT call runs on one worker
// thread.
//
// Reference counts: every static call in the generated bindings
// (OcrEngine*, SoftwareBitmapCreateCopyFromBuffer,
// CryptographicBufferCreateFromByteArray) gets its activation factory with
// RoGetActivationFactory and never releases it. Activation factories are
// per-process singletons cached by the WinRT runtime, so this only raises
// their reference count (a few per recognition); it allocates nothing and
// is not a leak. The generated code is left as generated (go-toast's has
// the same pattern); every instance object this package gets is released.
type Engine struct {
	langs []string
	w     worker
}

// New returns a Windows OCR engine. langs are BCP-47 tags tried in order;
// empty or none installed = the user's profile languages.
func New(langs []string) *Engine { return &Engine{langs: langs} }

// Probe reports the installed OCR languages. No language pack means
// unavailable, with the Settings path as the hint. A failed WinRT activation
// (an HRESULT, or a Go panic such as a failed MustQueryInterface) becomes a
// reason. recover cannot catch a fault inside a WinRT call itself (an access
// violation in foreign code ends the process); that is what the on-device
// test (go test ./platform/windows/winocr/) has to rule out.
func (e *Engine) Probe(ctx context.Context) ocr.Status {
	var st ocr.Status
	if err := e.w.do(ctx, func() { st = probe() }); err != nil {
		return ocr.Status{Reason: "Windows OCR did not answer in time."}
	}
	return st
}

func probe() (st ocr.Status) {
	defer func() {
		if r := recover(); r != nil {
			st = ocr.Status{Reason: reasonNoAPI}
		}
	}()
	langs, tags, err := availableLanguages()
	if err != nil {
		return ocr.Status{Reason: reasonNoAPI}
	}
	defer releaseLanguages(langs)
	if len(langs) == 0 {
		return ocr.Status{Reason: reasonNoLanguage, Hint: hintNoLanguage}
	}
	eng, err := wocr.OcrEngineTryCreateFromUserProfileLanguages()
	if err != nil || eng == nil {
		eng, err = wocr.OcrEngineTryCreateFromLanguage(langs[0])
	}
	if err != nil || eng == nil {
		return ocr.Status{Reason: "Windows OCR could not start for any installed language.", Hint: hintNoLanguage}
	}
	eng.Release()
	return ocr.Status{Available: true, Engine: ocr.EngineWindows, Langs: tags}
}

// availableLanguages returns the installed recognizer languages and their
// BCP-47 tags. The caller releases the languages.
func availableLanguages() ([]*globalization.Language, []string, error) {
	vec, err := wocr.OcrEngineGetAvailableRecognizerLanguages()
	if err != nil || vec == nil {
		return nil, nil, fmt.Errorf("winocr: recognizer languages: %w", err)
	}
	defer vec.Release()
	n, err := vec.GetSize()
	if err != nil {
		return nil, nil, err
	}
	var langs []*globalization.Language
	var tags []string
	for i := uint32(0); i < n; i++ {
		p, err := vec.GetAt(i)
		if err != nil || p == nil {
			continue
		}
		l := (*globalization.Language)(p)
		tag, err := l.GetLanguageTag()
		if err != nil {
			l.Release()
			continue
		}
		langs = append(langs, l)
		tags = append(tags, tag)
	}
	return langs, tags, nil
}

func releaseLanguages(langs []*globalization.Language) {
	for _, l := range langs {
		l.Release()
	}
}

// Recognize runs OcrEngine.RecognizeAsync over img on the worker thread.
// ctx cancellation cancels the async operation.
func (e *Engine) Recognize(ctx context.Context, img image.Image, opts ocr.Options) (ocr.Result, error) {
	langs := opts.Langs
	if len(langs) == 0 {
		langs = e.langs
	}
	var res ocr.Result
	var err error
	if derr := e.w.do(ctx, func() { res, err = recognize(ctx, img, langs) }); derr != nil {
		return ocr.Result{}, derr
	}
	return res, err
}

// engineFor creates an engine for the first configured tag that matches an
// installed recognizer language (ocr.MatchTag: exact, then language and
// script, then language), falling back to the profile languages and then the
// first installed language.
func engineFor(langs []string) (*wocr.OcrEngine, error) {
	avail, tags, err := availableLanguages()
	if err != nil {
		return nil, err
	}
	defer releaseLanguages(avail)
	if len(avail) == 0 {
		return nil, ocr.ErrUnavailable
	}
	for _, want := range langs {
		if i := ocr.MatchTag(want, tags); i >= 0 {
			if eng, err := wocr.OcrEngineTryCreateFromLanguage(avail[i]); err == nil && eng != nil {
				return eng, nil
			}
		}
	}
	if eng, err := wocr.OcrEngineTryCreateFromUserProfileLanguages(); err == nil && eng != nil {
		return eng, nil
	}
	if eng, err := wocr.OcrEngineTryCreateFromLanguage(avail[0]); err == nil && eng != nil {
		return eng, nil
	}
	return nil, ocr.ErrUnavailable
}

func recognize(ctx context.Context, img image.Image, langs []string) (res ocr.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = ocr.Result{}, fmt.Errorf("winocr: %v", r)
		}
	}()
	eng, err := engineFor(langs)
	if err != nil {
		return ocr.Result{}, err
	}
	defer eng.Release()
	res.Engine = ocr.EngineWindows
	if l, err := eng.GetRecognizerLanguage(); err == nil && l != nil {
		if tag, err := l.GetLanguageTag(); err == nil {
			res.Langs = []string{tag}
		}
		l.Release()
	}

	maxDim, err := wocr.OcrEngineGetMaxImageDimension()
	if err != nil || maxDim == 0 {
		maxDim = fallbackMaxDim
	}
	scale := chooseScale(img.Bounds(), maxDim)
	pix, w, h := toBGRA8Premultiplied(img, scale)
	buf, err := cryptography.CryptographicBufferCreateFromByteArray(uint32(len(pix)), pix)
	if err != nil || buf == nil {
		return ocr.Result{}, fmt.Errorf("winocr: buffer: %w", err)
	}
	defer buf.Release()
	bmp, err := imaging.SoftwareBitmapCreateCopyFromBuffer(buf, imaging.BitmapPixelFormatBgra8, int32(w), int32(h))
	if err != nil || bmp == nil {
		return ocr.Result{}, fmt.Errorf("winocr: bitmap: %w", err)
	}
	defer func() {
		_ = bmp.Close()
		bmp.Release()
	}()

	op, err := eng.RecognizeAsync(bmp)
	if err != nil || op == nil {
		return ocr.Result{}, fmt.Errorf("winocr: recognize: %w", err)
	}
	defer op.Release()
	p, err := await(ctx, op)
	if err != nil {
		return ocr.Result{}, err
	}
	r := (*wocr.OcrResult)(p)
	defer r.Release()

	vec, err := r.GetLines()
	if err != nil || vec == nil {
		return ocr.Result{}, fmt.Errorf("winocr: lines: %w", err)
	}
	defer vec.Release()
	off := img.Bounds().Min
	res.Lines, err = readLines(vec, scale, off)
	return res, err
}

func readLines(vec *collections.IVectorView, scale float64, off image.Point) ([]ocr.Line, error) {
	n, err := vec.GetSize()
	if err != nil {
		return nil, err
	}
	var lines []ocr.Line
	for i := uint32(0); i < n; i++ {
		p, err := vec.GetAt(i)
		if err != nil || p == nil {
			continue
		}
		line := (*wocr.OcrLine)(p)
		l := ocr.Line{}
		l.Text, _ = line.GetText()
		if words, err := line.GetWords(); err == nil && words != nil {
			l.Words = readWords(words, scale, off)
			words.Release()
		}
		line.Release()
		if len(l.Words) == 0 {
			continue
		}
		l.Rect = ocr.UnionRect(l.Words)
		if l.Text == "" {
			l.Text = ocr.JoinWords(l.Words)
		}
		lines = append(lines, l)
	}
	return lines, nil
}

func readWords(vec *collections.IVectorView, scale float64, off image.Point) []ocr.Word {
	n, err := vec.GetSize()
	if err != nil {
		return nil
	}
	var out []ocr.Word
	for i := uint32(0); i < n; i++ {
		p, err := vec.GetAt(i)
		if err != nil || p == nil {
			continue
		}
		word := (*wocr.OcrWord)(p)
		txt, _ := word.GetText()
		rc, rerr := word.GetBoundingRect()
		word.Release()
		if txt == "" || rerr != nil {
			continue
		}
		out = append(out, ocr.Word{Text: txt, Rect: mapBack(rc.X, rc.Y, rc.Width, rc.Height, scale, off), Conf: 1})
	}
	return out
}

// await polls the operation every 10 ms; when ctx ends it cancels the
// operation and returns ctx.Err().
func await(ctx context.Context, op *foundation.IAsyncOperation) (unsafe.Pointer, error) {
	itf, err := op.QueryInterface(ole.NewGUID(foundation.GUIDIAsyncInfo))
	if err != nil {
		return nil, fmt.Errorf("winocr: async info: %w", err)
	}
	info := (*foundation.IAsyncInfo)(unsafe.Pointer(itf))
	defer info.Release()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		st, err := info.GetStatus()
		if err != nil {
			return nil, err
		}
		switch st {
		case foundation.AsyncStatusCompleted:
			return op.GetResults()
		case foundation.AsyncStatusCanceled:
			return nil, context.Canceled
		case foundation.AsyncStatusError:
			code, _ := info.GetErrorCode()
			return nil, fmt.Errorf("winocr: recognition failed (HRESULT 0x%08x)", uint32(code.Value))
		}
		select {
		case <-ctx.Done():
			_ = info.Cancel()
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}
