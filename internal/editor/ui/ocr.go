//go:build darwin || windows || (linux && cgo)

package ui

import (
	"context"
	"errors"
	"image"
	"image/color"
	"io"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/internal/editor/textsel"
)

// OCR modes passed by the host with --ocr.
const (
	OCROn     = "on"     // engine available: recognize and enable the text tools
	OCROff    = "off"    // no engine: text tools greyed out with the reason
	OCRHidden = "hidden" // ocr.enabled: false (or an old host): no text tools
)

// defaultOCRTimeout bounds one recognition when the host passes none.
const defaultOCRTimeout = 20 * time.Second

// ocrOutcome is what the recognition goroutine hands back to the UI loop.
type ocrOutcome struct {
	res ocr.Result
	err error
}

// ocrState is the editor's text-recognition state. Recognition runs on its
// own goroutine; pollOCR moves the outcome in on the UI goroutine, so every
// other field is only touched there.
type ocrState struct {
	mode    string
	status  ocr.Status
	engine  ocr.Engine
	langs   []string
	auto    bool
	timeout time.Duration
	kinds   []ocr.Kind
	textOut string

	started bool
	running bool
	pending chan ocrOutcome
	cancel  context.CancelFunc

	result *ocr.Result
	full   textsel.Layout // every recognized word
	err    error

	// masked is full without the words under Redact boxes (see words);
	// cover is the Redact rectangles it was built for.
	masked textsel.Layout
	cover  []image.Rectangle
	maskOK bool
}

// selState is the text selection (in the Select tool) in reading order.
// A press on text only anchors it; the selection starts once the pointer
// has moved, so a plain click never leaves a word selected.
type selState struct {
	dragging      bool
	from          image.Point // image point of the press
	moved         bool
	anchor, focus textsel.Ref
	refs          []textsel.Ref
	lastPress     time.Time
	lastRef       textsel.Ref
}

// hintState is a transient hint-row message ("Copied 12 characters").
type hintState struct {
	text  string
	until time.Time
}

// doubleClick is the second-press window that selects a whole line.
const doubleClick = 350 * time.Millisecond

// startOCR begins background recognition once. It never blocks the UI: the
// result arrives through e.ocr.pending and wakes the window.
func (e *editor) startOCR() {
	if e.ocr.mode != OCROn || e.ocr.started || e.ocr.engine == nil {
		return
	}
	e.ocr.started, e.ocr.running = true, true
	timeout := e.ocr.timeout
	if timeout <= 0 {
		timeout = defaultOCRTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	e.ocr.cancel = cancel
	eng, img, opts, ch, w := e.ocr.engine, e.base, ocr.Options{Langs: e.ocr.langs}, e.ocr.pending, e.win
	go func() {
		defer cancel()
		// The engine runs on its own goroutine so a call that ignores ctx
		// (a hung WinRT or C call) still ends "Recognizing text..." at the
		// timeout; its late result is dropped.
		inner := make(chan ocrOutcome, 1)
		go func() {
			res, err := eng.Recognize(ctx, img, opts)
			inner <- ocrOutcome{res, err}
		}()
		var o ocrOutcome
		select {
		case o = <-inner:
		case <-ctx.Done():
			o = ocrOutcome{err: ctx.Err()}
		}
		ch <- o
		if w != nil {
			w.Invalidate()
		}
	}()
}

// pollOCR picks up a finished recognition without blocking, and says once
// in the hint row what was found.
func (e *editor) pollOCR(gtx layout.Context) {
	select {
	case o := <-e.ocr.pending:
		e.ocr.running = false
		if o.err != nil {
			e.ocr.err = o.err
			return
		}
		e.ocr.result = &o.res
		e.ocr.full = textsel.New(o.res)
		e.ocr.maskOK = false
		e.setHintFor(gtx, e.foundText(), 4*time.Second)
	default:
	}
}

// foundText is the one-time message when recognition finishes. Dragging
// over text selects it only in Select, so other tools point there.
func (e *editor) foundText() string {
	n := 0
	for _, ln := range e.words().Lines {
		if len(ln.Words) > 0 {
			n++
		}
	}
	if n == 0 {
		return e.noTextReason()
	}
	found := countLabel(n, "line", "lines") + " of text found."
	if e.tool != ToolSelect {
		return found + " Press V to select text."
	}
	return found + " Drag over text to select, " + shortcutMod() + "+C copies."
}

// words is the recognized text that can be selected and copied: every word
// except those mostly under a Redact box (textsel.Mask), so redacted text
// never reaches the clipboard. It is rebuilt whenever the Redact boxes
// change (drawn, moved, deleted, undone or redone); a text selection made
// against the old words is cleared then.
func (e *editor) words() textsel.Layout {
	var cover []image.Rectangle
	for _, sh := range e.shapes {
		if sh.kind == kRedact {
			cover = append(cover, sh.rects...)
		}
	}
	if e.ocr.maskOK && slices.Equal(cover, e.ocr.cover) {
		return e.ocr.masked
	}
	if e.ocr.maskOK {
		e.clearSelection()
	}
	e.ocr.cover, e.ocr.masked, e.ocr.maskOK = cover, e.ocr.full.Mask(cover), true
	return e.ocr.masked
}

// ocrAvailable reports whether the text tools can act now, and why not.
func (e *editor) ocrAvailable() (bool, string) {
	switch {
	case e.ocr.mode != OCROn:
		if r := e.ocr.status.Explain(); r != "" {
			return false, r
		}
		return false, "Text recognition is not available."
	case e.ocr.err != nil:
		if errors.Is(e.ocr.err, context.DeadlineExceeded) {
			return false, "Text recognition took too long and was stopped."
		}
		return false, "Text recognition failed: " + e.ocr.err.Error()
	case e.ocr.running:
		return false, "Recognizing text..."
	}
	return true, ""
}

// hasText reports whether there is recognized text outside Redact boxes.
func (e *editor) hasText() bool { return e.ocr.result != nil && !e.words().Empty() }

// noTextReason says why there is no text to select or copy.
func (e *editor) noTextReason() string {
	if e.ocr.result != nil && e.ocr.result.WordCount() > 0 {
		return "All recognized text is under Redact boxes."
	}
	return "No text was found in this image."
}

// copyTextState is the Copy text button's label, whether it can act now,
// and why not. With auto-run off it is enabled before recognition: the
// first click starts it.
func (e *editor) copyTextState() (label string, ok bool, why string) {
	switch {
	case e.ocr.mode == OCROn && !e.ocr.started:
		return "Copy text", true, ""
	case e.ocr.running:
		return "Reading text...", false, "Reading the text in the image..."
	}
	if ok, why := e.ocrAvailable(); !ok {
		if e.ocr.mode != OCROn {
			why = "Copy text is not available: " + why
		}
		return "Copy text", false, why
	}
	if !e.hasText() {
		return "Copy text", false, e.noTextReason()
	}
	return "Copy text", true, ""
}

// copyTextAction is the Copy text button: the selected text when there is a
// selection, otherwise all recognized text.
func (e *editor) copyTextAction(gtx layout.Context) {
	if e.hasTextSelection() {
		e.copySelection(gtx)
		return
	}
	e.copyAllText(gtx)
}

// redactSelState reports whether Redact selection can act now, and why not.
func (e *editor) redactSelState() (bool, string) {
	if ok, why := e.ocrAvailable(); !ok {
		if e.ocr.mode != OCROn {
			why = "Redact selection is not available: " + why
		}
		return false, why
	}
	if !e.hasTextSelection() {
		return false, "Drag over text in the image to select it, then Redact selection covers it."
	}
	return true, ""
}

// quickRedactEnabled reports whether Quick redact can act now, and why not.
func (e *editor) quickRedactEnabled() (bool, string) {
	if len(e.ocr.kinds) == 0 {
		return false, "Quick redact has nothing to look for; choose what it hides in Settings > Text recognition."
	}
	if e.ocr.mode == OCROn && !e.ocr.started {
		return true, ""
	}
	ok, why := e.ocrAvailable()
	if !ok && e.ocr.mode != OCROn {
		why = "Quick redact is not available: " + why
	}
	return ok, why
}

// hasTextSelection reports whether text is selected. It syncs the Redact
// mask first, so a selection made before the boxes changed does not count.
func (e *editor) hasTextSelection() bool {
	if e.ocr.result != nil {
		e.words()
	}
	return len(e.sel.refs) > 0
}

func (e *editor) clearSelection() {
	e.sel.refs = nil
	e.sel.dragging = false
}

func (e *editor) selectedText() string {
	return e.words().Text(e.sel.refs)
}

// textAt returns the recognized word at ip, or near it (textsel.HitWord).
func (e *editor) textAt(ip image.Point) (textsel.Ref, bool) {
	if e.ocr.result == nil {
		return textsel.Ref{}, false
	}
	return e.words().HitWord(ip)
}

// selPress starts a text selection at ip, which is on text (see textAt). A
// second press on the same word within doubleClick selects its line.
func (e *editor) selPress(ip image.Point) {
	ref, ok := e.textAt(ip)
	if !ok {
		e.clearSelection()
		return
	}
	now := time.Now()
	if now.Sub(e.sel.lastPress) < doubleClick && ref == e.sel.lastRef {
		e.sel.refs = e.words().LineRefs(ref.Line)
		e.sel.dragging = false
		e.sel.lastPress = time.Time{}
		return
	}
	e.sel.lastPress, e.sel.lastRef = now, ref
	e.sel.dragging, e.sel.moved = true, false
	e.sel.from = ip
	e.sel.anchor, e.sel.focus = ref, ref
	e.sel.refs = nil
}

func (e *editor) selDrag(ip image.Point) {
	if !e.sel.dragging {
		return
	}
	if !e.sel.moved {
		d := ip.Sub(e.sel.from)
		if t := int(e.hitTol() + 0.5); max(d.X, -d.X, d.Y, -d.Y) < max(t, 1) {
			return
		}
		e.sel.moved = true
	}
	if ref, ok := e.words().HitWord(ip); ok {
		e.sel.focus = ref
	}
	e.sel.refs = e.words().Range(e.sel.anchor, e.sel.focus)
}

func (e *editor) selRelease() { e.sel.dragging = false }

// selectAllText selects every recognized word (Ctrl/Cmd+A in Select), or
// says in the hint row why it cannot.
func (e *editor) selectAllText(gtx layout.Context) {
	if why := e.textActionBlocked(gtx); why != "" {
		e.setHint(gtx, why)
		return
	}
	if !e.hasText() {
		e.setHint(gtx, e.noTextReason())
		return
	}
	if e.selected >= 0 {
		e.deselect()
	}
	e.sel.refs = e.words().All()
}

// copyText puts txt on the clipboard and, for the Linux host handoff, into
// the --text-out file. The editor stays open.
func (e *editor) copyText(gtx layout.Context, txt string) {
	if txt == "" {
		return
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(txt))})
	if e.ocr.textOut != "" {
		_ = os.WriteFile(e.ocr.textOut, []byte(txt), 0o600)
	}
	n := utf8.RuneCountInString(txt)
	msg := "Copied " + itoa(n) + " characters"
	if n == 1 {
		msg = "Copied 1 character"
	}
	e.setHint(gtx, msg)
}

func (e *editor) copySelection(gtx layout.Context) { e.copyText(gtx, e.selectedText()) }

// copyAllText copies every recognized word (Copy all, Ctrl/Cmd+Shift+C).
// When there is nothing to copy it says why in the hint row.
func (e *editor) copyAllText(gtx layout.Context) {
	if why := e.textActionBlocked(gtx); why != "" {
		e.setHint(gtx, why)
		return
	}
	if !e.hasText() {
		e.setHint(gtx, e.noTextReason())
		return
	}
	e.copyText(gtx, e.words().AllText())
}

// textActionBlocked returns why a text action cannot run now ("" when it
// can): recognition off, unavailable, failed or still running. With
// auto-run off it starts recognition and asks to press again.
func (e *editor) textActionBlocked(gtx layout.Context) string {
	if e.ocr.mode == OCRHidden {
		return "Text recognition is turned off (Settings > Text recognition)."
	}
	if e.ocr.mode == OCROn && !e.ocr.started {
		e.startOCR()
		return "Recognizing text... press again when it is done."
	}
	if ok, why := e.ocrAvailable(); !ok {
		return why
	}
	if e.ocr.result == nil {
		return "Recognizing text..."
	}
	return ""
}

// redactPad grows each redaction box so anti-aliased glyph edges are covered.
const redactPad = 2

// redactSelection covers the selected words with one opaque Redact shape
// (one undo step) and clears the selection.
func (e *editor) redactSelection(gtx layout.Context) {
	rects := e.words().Rects(e.sel.refs)
	if len(rects) == 0 {
		return
	}
	e.pushRedact(rects)
	e.clearSelection()
	e.setHint(gtx, "Redacted the selection")
}

// quickRedact hides every match of the configured kinds in one Redact shape
// (one undo step). It never claims completeness.
func (e *editor) quickRedact(gtx layout.Context) {
	if e.ocr.mode == OCRHidden {
		e.setHint(gtx, "Text recognition is turned off (Settings > Text recognition).")
		return
	}
	if ok, why := e.quickRedactEnabled(); !ok {
		e.setHint(gtx, why)
		return
	}
	if why := e.textActionBlocked(gtx); why != "" {
		e.setHint(gtx, why)
		return
	}
	hits := ocr.FindSensitive(*e.ocr.result, e.ocr.kinds)
	var rects []image.Rectangle
	counts := map[ocr.Kind]int{}
	for _, h := range hits {
		rects = append(rects, h.Rects...)
		counts[h.Kind]++
	}
	if len(rects) == 0 {
		e.setHint(gtx, "Nothing to redact: found no "+kindList(e.ocr.kinds)+".")
		return
	}
	e.pushRedact(rects)
	e.clearSelection()
	e.setHint(gtx, "Redacted "+countLabel(len(hits), "item", "items")+" ("+countsText(counts)+"). Check the image: Quick redact can miss text.")
}

func (e *editor) pushRedact(rects []image.Rectangle) {
	out := make([]image.Rectangle, 0, len(rects))
	for _, r := range rects {
		if r = r.Inset(-redactPad).Intersect(e.bounds); !r.Empty() {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return
	}
	col := e.col
	col.A = 0xff
	e.push(shape{kind: kRedact, rects: out, col: col})
}

var kindNames = map[ocr.Kind][2]string{
	ocr.KindEmail: {"email", "emails"},
	ocr.KindPhone: {"phone number", "phone numbers"},
	ocr.KindToken: {"key or token", "keys and tokens"},
	ocr.KindURL:   {"link", "links"},
	ocr.KindIP:    {"IP address", "IP addresses"},
}

func countLabel(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}

func countsText(counts map[ocr.Kind]int) string {
	var parts []string
	for _, k := range ocr.AllKinds {
		if n := counts[k]; n > 0 {
			parts = append(parts, countLabel(n, kindNames[k][0], kindNames[k][1]))
		}
	}
	return strings.Join(parts, ", ")
}

func kindList(kinds []ocr.Kind) string {
	var parts []string
	for _, k := range kinds {
		parts = append(parts, kindNames[k][1])
	}
	return strings.Join(parts, " or ")
}

// setHint shows a transient message in the hint row for 1.5 s.
func (e *editor) setHint(gtx layout.Context, msg string) {
	e.setHintFor(gtx, msg, 1500*time.Millisecond)
}

// setHintFor shows a transient message in the hint row for d.
func (e *editor) setHintFor(gtx layout.Context, msg string, d time.Duration) {
	e.hint = hintState{text: msg, until: gtx.Now.Add(d)}
	gtx.Execute(op.InvalidateCmd{At: e.hint.until})
}

// drawTextOverlay fills the selected words. While the pointer is over text
// or a text selection is being dragged, every recognized word is also
// outlined faintly (the "show text" affordance).
func (e *editor) drawTextOverlay(ops *op.Ops) {
	if e.ocr.result == nil {
		return
	}
	showAll := e.hoverText || e.sel.dragging
	selected := map[textsel.Ref]bool{}
	for _, r := range e.sel.refs {
		selected[r] = true
	}
	acc := e.theme.accent
	for li, l := range e.words().Lines {
		for wi, w := range l.Words {
			sel := selected[textsel.Ref{Line: li, Word: wi}]
			if !sel && !showAll {
				continue
			}
			a, b := e.screen(w.Rect.Min), e.screen(w.Rect.Max)
			alpha := uint8(0x18)
			if sel {
				alpha = 0x80
			}
			fillRect(ops, a, b, color.NRGBA{acc.R, acc.G, acc.B, alpha})
			if showAll {
				strokeRect(ops, a, b, 1, color.NRGBA{acc.R, acc.G, acc.B, 0x90})
			}
		}
	}
}
