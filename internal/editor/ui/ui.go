//go:build darwin || windows || (linux && cgo)

// Package ui is the Gio-based annotation canvas for the GoShareIt editor
// helper. It is build-tagged for darwin, windows and linux with cgo because
// Gio requires cgo on macOS and Linux and a platform GPU backend everywhere;
// the CGO-disabled Linux host build excludes this package entirely. All pixel work is delegated to the
// pure-Go internal/editor/annotate package so it stays toolkit-independent and
// unit-testable.
//
// Threading contract: Gio's app.Main must own the process main goroutine (see
// cmd/goshareit-editor). Run is the window event loop and is expected to be
// started on a separate goroutine by main; it blocks until the user confirms,
// cancels, or closes the window, then returns the outcome.
package ui

import (
	"image"
	"image/color"
	"slices"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/internal/editor/annotate"
	"github.com/Rake-Pro/GoShareIt/internal/editor/cropbox"
	"github.com/Rake-Pro/GoShareIt/internal/editor/oplog"
)

// Action identifies which button the user confirmed out of the editor with.
// ActionCancel means the user skipped/cancelled/Esc'd/closed the window;
// ActionConfirm means the plain confirm button (run the default pipeline);
// ActionCopy/ActionSave/ActionUpload are the explicit per-capture overrides.
type Action int

const (
	ActionCancel Action = iota
	ActionConfirm
	ActionCopy
	ActionSave
	ActionUpload
)

// Tool identifies the active drawing tool.
type Tool string

const (
	// ToolSelect picks a placed shape to move, delete or restyle. It is
	// always in the toolbar (first), whatever editor.tools lists.
	ToolSelect    Tool = "select"
	ToolCrop      Tool = "crop"
	ToolArrow     Tool = "arrow"
	ToolRect      Tool = "rect"
	ToolEllip     Tool = "ellipse"
	ToolText      Tool = "text"
	ToolBlur      Tool = "blur"
	ToolPixelate  Tool = "pixelate"
	ToolHighlight Tool = "highlight"
	ToolStep      Tool = "step"
	ToolLine      Tool = "line"
	ToolFreehand  Tool = "freehand"
	// ToolRedact draws an opaque box: the safe way to hide text (blur and
	// pixelate can be reversed).
	ToolRedact Tool = "redact"
	// ToolSelectText is the text-selection mode over recognized words.
	ToolSelectText Tool = "select_text"
)

// Options configures the initial editor state. Color is the initial stroke
// color; Stroke the initial width; Tools restricts which tools appear in the
// toolbar (empty -> all P4a tools); Tool is the initially selected tool.
// Theme is the resolved theme ("light" or "dark"; anything else, including
// empty, falls back to dark) - callers resolve "system" before calling Run.
// ConfirmLabel is rendered on the confirm button ("" -> "Done"). Actions, when
// true, renders the explicit Copy/Save/Upload action row; CanUpload controls
// whether the Upload button is enabled or shown greyed-out/inert.
//
// The OCR fields come from the host's --ocr flags. OCRMode is OCROn (engine
// available; OCREngine set), OCROff (text tools greyed out with OCR's
// reason) or OCRHidden (no text tools; also the zero value, so an old host
// keeps today's UI). TextOut is where copied text is also written for the
// host (Linux clipboard handoff).
type Options struct {
	Tool         Tool
	Color        color.NRGBA
	Stroke       int
	Tools        []Tool
	Theme        string
	ConfirmLabel string
	Actions      bool
	CanUpload    bool

	OCRMode     string
	OCR         ocr.Status
	OCREngine   ocr.Engine
	OCRAuto     bool
	OCRLangs    []string
	OCRTimeout  time.Duration
	QuickRedact []ocr.Kind
	TextOut     string
}

// shapeKind mirrors Tool for committed shapes.
type shapeKind int

const (
	kArrow shapeKind = iota
	kRect
	kEllipse
	kText
	kCrop
	kBlur
	kPixelate
	kHighlight
	kStep
	kLine
	kFreehand
	kRedact
)

// shape is the UI-side concrete annotation, stored in original-image pixel
// coordinates. Crop shapes carry their rectangle in p0/p1 and are folded into
// the render crop rather than drawn.
type shape struct {
	kind   shapeKind
	p0, p1 image.Point
	col    color.NRGBA
	stroke int
	text   string
	num    int               // step-badge number (kStep)
	pts    []image.Point     // freehand polyline (kFreehand)
	rects  []image.Rectangle // opaque boxes (kRedact); one shape = one undo step

	// textOp/textSize cache the rendered text (kText) so the canvas shows
	// exactly what annotate will draw, at its real size.
	textOp   paint.ImageOp
	textSize image.Point
}

// Run shows the editor for img and returns the (possibly annotated) result.
// action reports which button the user confirmed with; on cancel, Esc, or
// window close it is ActionCancel and result is nil. err is non-nil only on a
// genuine Gio failure. Run blocks until the window closes and must be called
// on a goroutine other than the one running app.Main.
func Run(img image.Image, opts Options) (result image.Image, action Action, err error) {
	e := newEditor(img, opts)
	w := new(app.Window)
	e.win = w
	w.Option(
		app.Title("GoShareIt - Annotate"),
		app.Size(unit.Dp(1000), unit.Dp(720)),
		// Keep the action row usable even when the user shrinks the window.
		app.MinSize(unit.Dp(640), unit.Dp(400)),
	)
	// Recognition runs in the background from the first frame; nothing
	// waits for it. Select text as the default tool counts as asking for it
	// even with auto-run off.
	if (e.ocr.auto && (e.hasTool(ToolSelectText) || e.hasTool(ToolRedact))) || e.tool == ToolSelectText {
		e.startOCR()
	}
	defer func() {
		if e.ocr.cancel != nil {
			e.ocr.cancel()
		}
	}()
	return e.loop(w)
}

type editor struct {
	win    *app.Window // set by Run; recognition wakes it
	base   image.Image
	bounds image.Rectangle // base bounds, origin-normalized size

	tools   []Tool
	tool    Tool
	col     color.NRGBA
	stroke  int
	palette []color.NRGBA

	// shapes is the current list; hist records every change to it (add,
	// remove, modify) for undo/redo. The single crop is a kCrop entry that
	// later crops modify, so its handles and undo work like any shape.
	shapes []shape
	hist   oplog.Log[shape]
	crop   *image.Rectangle

	// Select tool: the selected shape (-1 none), the shape under the
	// pointer, and an in-progress move (moveOrig is the shape before it).
	selected   int
	hoverShape int
	moving     bool
	moveFrom   image.Point
	moveDelta  image.Point // offset applied so far
	moveOrig   shape

	// cropDrag is an in-progress handle drag (resize or move) of the crop.
	cropDrag cropDragState

	// in-progress drag (image coords)
	dragging    bool
	dragFrom    image.Point
	dragTo      image.Point
	freehandPts []image.Point // accumulated points for the active freehand stroke

	// text tool hover preview (image coords) and its cached rendering
	hover    image.Point
	hovering bool
	ghost    shape

	// view transform
	zoom       float64
	panX, panY float32
	lastOrigin f32.Point
	lastScale  float32
	lastSize   image.Point // canvas size at the last layout
	lastFit    float64     // fit-to-window scale at the last layout
	hoverPos   f32.Point   // pointer position in window pixels
	hitTolPx   float32     // extra hit slack for thin strokes, window pixels
	handlePx   int         // half the crop handle hit square, window pixels

	// text recognition, selection and the hint row
	ocr  ocrState
	sel  selState
	hint hintState

	// panning: middle-button drag, or Space held while dragging
	panning   bool
	panLast   f32.Point
	spaceDown bool

	// discardArmed is set by a first Esc/Cancel while annotations exist; a
	// second one confirms the discard.
	discardArmed bool

	// widgets
	th           *material.Theme
	theme        themePalette // resolved theme colors, applied to th.Palette and painted directly
	confirmLabel string       // rendered on the confirm button
	actions      bool         // render the explicit Copy/Save/Upload action row
	canUpload    bool         // Upload button enabled vs greyed-out/inert
	toolBtns     map[Tool]*widget.Clickable
	swatchBtn    []*widget.Clickable
	strokeInc    widget.Clickable
	strokeDec    widget.Clickable
	undoBtn      widget.Clickable
	redoBtn      widget.Clickable
	copyB        widget.Clickable
	saveB        widget.Clickable
	uploadB      widget.Clickable
	confirm      widget.Clickable
	cancelB      widget.Clickable
	textIn       widget.Editor
	toolRow      layout.List // scrollable tool/swatch row (toolbar row 1)

	// Select text / Redact row-2 buttons and the zoom readout.
	copyTextB    widget.Clickable
	copyAllB     widget.Clickable
	redactSelB   widget.Clickable
	quickRedactB widget.Clickable
	zoomBtn      widget.Clickable
	// Hover trackers for greyed-out buttons, which take no input themselves
	// (gtx.Disabled), so the hint row can show why they are greyed.
	toolHover  map[Tool]*gesture.Hover
	quickHover gesture.Hover
	hoverWhy   string // reason of the greyed button under the pointer this frame

	imgOp paint.ImageOp

	// outcome
	result image.Image
	action Action
	done   bool
	err    error // render failure on confirm; Run returns it
}

const canvasTag = "goshareit.canvas"

// cropDragState is a drag on a crop handle or inside the crop.
type cropDragState struct {
	active bool
	handle cropbox.Handle
	from   image.Point     // image point where the drag started
	orig   image.Rectangle // crop before the drag
}

// minCrop is the smallest crop side in image pixels; a shorter drag is a
// click, not a crop.
const minCrop = 4

// badgeRadius is the step-badge disc radius in image pixels for a stroke
// width; shared by the annotate render and the on-canvas preview so they line
// up. It grows with the stroke so badges keep pace with everything else on
// high-DPI captures, where the stroke default is larger.
func badgeRadius(stroke int) int {
	return max(14, 3*stroke)
}

// maxStroke caps the stroke width (the toolbar "+" stops here).
const maxStroke = 32

func newEditor(img image.Image, opts Options) *editor {
	b := img.Bounds()
	// Normalize coordinates so shape space matches a 0,0-origin base.
	norm := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			norm.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}

	// Unknown tool names are dropped (they would get a button that draws
	// nothing), and a default tool outside the enabled list falls back to the
	// first enabled one so the active tool always has a highlighted button.
	//
	// With OCR hidden (turned off in Settings, or an old host) Select text is
	// dropped the same way; Redact needs no OCR and stays.
	ocrMode := opts.OCRMode
	if ocrMode != OCROn && ocrMode != OCROff {
		ocrMode = OCRHidden
	}
	known := func(t Tool) bool { return toolLabel(t) != "" && !(t == ToolSelectText && ocrMode == OCRHidden) }
	var tools []Tool
	for _, t := range opts.Tools {
		if known(t) {
			tools = append(tools, t)
		}
	}
	if len(tools) == 0 {
		for _, t := range []Tool{
			ToolCrop, ToolArrow, ToolRect, ToolEllip, ToolText,
			ToolBlur, ToolPixelate, ToolHighlight, ToolStep, ToolLine, ToolFreehand,
			ToolRedact, ToolSelectText,
		} {
			if known(t) {
				tools = append(tools, t)
			}
		}
	}
	tool := opts.Tool
	if tool == "" {
		tool = ToolArrow
	}
	if !slices.Contains(tools, tool) || (tool == ToolSelectText && ocrMode != OCROn) {
		tool = tools[0]
		if tool == ToolSelectText && ocrMode != OCROn && len(tools) > 1 {
			tool = tools[1]
		}
	}
	// Select is not a drawing tool: it is always there, first in the row.
	tools = append([]Tool{ToolSelect}, slices.DeleteFunc(tools, func(t Tool) bool { return t == ToolSelect })...)
	col := opts.Color
	if col == (color.NRGBA{}) {
		col = color.NRGBA{R: 0xff, G: 0x3b, B: 0x30, A: 0xff}
	}
	stroke := opts.Stroke
	if stroke < 1 {
		stroke = 6
	}
	stroke = min(stroke, maxStroke)

	theme := darkTheme
	if opts.Theme == "light" {
		theme = lightTheme
	}
	confirmLabel := opts.ConfirmLabel
	if confirmLabel == "" {
		confirmLabel = "Done"
	}

	th := material.NewTheme()
	th.Palette = material.Palette{
		Bg:         theme.toolbarBg,
		Fg:         theme.fg,
		ContrastBg: theme.accent,
		ContrastFg: theme.contrastFg,
	}

	e := &editor{
		base:         norm,
		bounds:       norm.Bounds(),
		tools:        tools,
		tool:         tool,
		col:          col,
		stroke:       stroke,
		zoom:         1,
		th:           th,
		theme:        theme,
		confirmLabel: confirmLabel,
		actions:      opts.Actions,
		canUpload:    opts.CanUpload,
		palette:      defaultPalette(),
		imgOp:        paint.NewImageOp(norm),
		selected:     -1,
		hoverShape:   -1,
	}
	e.ocr = ocrState{
		mode:    ocrMode,
		status:  opts.OCR,
		engine:  opts.OCREngine,
		langs:   opts.OCRLangs,
		auto:    opts.OCRAuto && ocrMode == OCROn,
		timeout: opts.OCRTimeout,
		kinds:   opts.QuickRedact,
		textOut: opts.TextOut,
		pending: make(chan ocrOutcome, 1),
	}
	if ocrMode == OCROn && opts.OCREngine == nil {
		e.ocr.mode = OCROff
		e.ocr.status = ocr.Status{Reason: "Text recognition is not available in this build."}
	}
	e.toolBtns = make(map[Tool]*widget.Clickable, len(tools))
	e.toolHover = make(map[Tool]*gesture.Hover, len(tools))
	for _, t := range tools {
		e.toolBtns[t] = new(widget.Clickable)
		e.toolHover[t] = new(gesture.Hover)
	}
	e.swatchBtn = make([]*widget.Clickable, len(e.palette))
	for i := range e.palette {
		e.swatchBtn[i] = new(widget.Clickable)
	}
	e.textIn.SingleLine = true
	e.toolRow.Axis = layout.Horizontal
	return e
}

// themePalette holds the explicit colors for one theme mode. It is applied
// both to th.Palette (so stock material widgets pick it up) and painted
// directly onto the toolbar and canvas backgrounds, so the light-by-default
// Gio window surface never shows through.
type themePalette struct {
	toolbarBg  color.NRGBA
	canvasBg   color.NRGBA
	fg         color.NRGBA
	surfaceBg  color.NRGBA // subtle background for unselected/secondary buttons
	accent     color.NRGBA
	contrastFg color.NRGBA // text/icon color on top of accent
}

var darkTheme = themePalette{
	toolbarBg:  color.NRGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff},
	canvasBg:   color.NRGBA{R: 0x14, G: 0x14, B: 0x14, A: 0xff},
	fg:         color.NRGBA{R: 0xe8, G: 0xe8, B: 0xe8, A: 0xff},
	surfaceBg:  color.NRGBA{R: 0x2c, G: 0x2c, B: 0x2e, A: 0xff},
	accent:     color.NRGBA{R: 0x0a, G: 0x84, B: 0xff, A: 0xff},
	contrastFg: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
}

var lightTheme = themePalette{
	toolbarBg:  color.NRGBA{R: 0xf2, G: 0xf2, B: 0xf4, A: 0xff},
	canvasBg:   color.NRGBA{R: 0xd8, G: 0xd8, B: 0xdc, A: 0xff},
	fg:         color.NRGBA{R: 0x1c, G: 0x1c, B: 0x1e, A: 0xff},
	surfaceBg:  color.NRGBA{R: 0xe4, G: 0xe4, B: 0xe8, A: 0xff},
	accent:     color.NRGBA{R: 0x0a, G: 0x84, B: 0xff, A: 0xff},
	contrastFg: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
}

func defaultPalette() []color.NRGBA {
	return []color.NRGBA{
		{0xff, 0x3b, 0x30, 0xff}, // red
		{0xff, 0x9f, 0x0a, 0xff}, // orange
		{0xff, 0xd6, 0x0a, 0xff}, // yellow
		{0x34, 0xc7, 0x59, 0xff}, // green
		{0x0a, 0x84, 0xff, 0xff}, // blue
		{0x00, 0x00, 0x00, 0xff}, // black
		{0xff, 0xff, 0xff, 0xff}, // white
	}
}

func (e *editor) loop(w *app.Window) (image.Image, Action, error) {
	var ops op.Ops
	for {
		switch ev := w.Event().(type) {
		case app.DestroyEvent:
			if ev.Err != nil {
				return nil, ActionCancel, ev.Err
			}
			// Window closed without confirm -> cancel. Gio cannot veto a
			// native close, so this path has no discard confirmation.
			if e.action != ActionCancel {
				return e.result, e.action, nil
			}
			return nil, ActionCancel, e.err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, ev)
			e.handleInput(gtx)
			if e.done {
				ev.Frame(gtx.Ops)
				if e.action != ActionCancel {
					return e.result, e.action, nil
				}
				return nil, ActionCancel, e.err
			}
			e.layout(gtx)
			ev.Frame(gtx.Ops)
		}
	}
}

// handleInput drains queued pointer and key events for the canvas before
// layout registers the next frame's input areas. A finished recognition is
// picked up first.
func (e *editor) handleInput(gtx layout.Context) {
	e.pollOCR()

	// Escape abandons a drag in progress, then clears a text or shape
	// selection, then cancels (with a confirmation step when annotations
	// would be lost).
	for {
		ev, ok := gtx.Source.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			if e.cancelGesture() {
				continue // Esc first abandons a drag in progress
			}
			e.requestCancel()
			if e.done {
				return
			}
		}
	}

	// Shortcuts. They are only read while the text field does not have focus,
	// so typing (and the field's own undo/copy) keeps working there.
	if !gtx.Focused(&e.textIn) {
		for {
			ev, ok := gtx.Source.Event(shortcutFilters()...)
			if !ok {
				break
			}
			ke, ok := ev.(key.Event)
			if !ok {
				continue
			}
			if ke.Name == key.NameSpace {
				e.spaceDown = ke.State == key.Press
				continue
			}
			if ke.State != key.Press {
				continue
			}
			e.handleShortcut(gtx, ke)
			if e.done {
				return
			}
		}
		for {
			ev, ok := gtx.Source.Event(singleKeyFilters()...)
			if !ok {
				break
			}
			if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
				e.handleSingleKey(gtx, ke)
			}
		}
	} else {
		e.spaceDown = false
	}

	for {
		ev, ok := gtx.Source.Event(pointer.Filter{
			Target:  canvasTag,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Move | pointer.Leave | pointer.Cancel | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		e.handlePointer(pe)
	}
}

// handleShortcut applies one Ctrl/Cmd shortcut (or Enter) press.
func (e *editor) handleShortcut(gtx layout.Context, ke key.Event) {
	shift := ke.Modifiers.Contain(key.ModShift)
	switch ke.Name {
	case "Z":
		if shift {
			e.redoOne()
		} else {
			e.undo()
		}
	case "Y":
		e.redoOne()
	case "C":
		switch {
		case shift:
			// Copy all text, in any tool, once recognition has a result.
			e.copyAllText(gtx)
		case e.hasTextSelection():
			e.copySelection(gtx)
		case e.actions:
			e.confirmNow(ActionCopy)
		}
	case "S":
		if e.actions {
			e.confirmNow(ActionSave)
		}
	case "A":
		if e.tool == ToolSelectText {
			e.selectAllText()
		}
	case "R":
		e.quickRedact(gtx)
	case "0":
		e.zoomFit()
	case "1":
		e.zoom100()
	case "=", "+":
		e.zoomStep(1.25)
	case "-":
		e.zoomStep(0.8)
	case key.NameReturn, key.NameEnter:
		// A drag still held (crop handle, move) counts as released.
		e.finishGesture()
		e.confirmNow(ActionConfirm)
	}
}

// requestCancel handles Esc and the Cancel button: a text selection, then a
// shape selection, is cleared first (neither arms the discard);
// with annotations on the canvas the first press only arms the discard (the
// hint row asks to confirm), the second one cancels.
func (e *editor) requestCancel() {
	if e.hasTextSelection() {
		e.clearSelection()
		return
	}
	if e.selected >= 0 {
		e.deselect()
		return
	}
	if len(e.shapes) > 0 && !e.discardArmed {
		e.discardArmed = true
		return
	}
	e.action = ActionCancel
	e.done = true
}

// zoomAround multiplies the zoom by factor keeping the window point pos
// fixed on screen.
func (e *editor) zoomAround(pos f32.Point, factor float64) {
	before := e.zoom
	e.zoom = min(max(e.zoom*factor, 0.05), 20)
	if e.lastScale <= 0 {
		return
	}
	ix := (pos.X - e.lastOrigin.X) / e.lastScale
	iy := (pos.Y - e.lastOrigin.Y) / e.lastScale
	s2 := e.lastScale * float32(e.zoom/before)
	bw, bh := float32(e.bounds.Dx()), float32(e.bounds.Dy())
	e.panX = pos.X - ix*s2 - (float32(e.lastSize.X)-bw*s2)/2
	e.panY = pos.Y - iy*s2 - (float32(e.lastSize.Y)-bh*s2)/2
	// Several zoom steps can arrive before the next layout; keep the
	// transform in step so each one zooms from the current view.
	e.lastScale = s2
	e.lastOrigin = f32.Pt(pos.X-ix*s2, pos.Y-iy*s2)
}

// zoomFit fits the whole image in the window and centres it.
func (e *editor) zoomFit() {
	e.zoom, e.panX, e.panY = 1, 0, 0
}

// zoom100 shows one image pixel per window pixel, centred.
func (e *editor) zoom100() {
	if e.lastFit > 0 {
		e.zoom = min(max(1/e.lastFit, 0.05), 20)
	}
	e.panX, e.panY = 0, 0
}

// zoomStep zooms by factor around the canvas centre.
func (e *editor) zoomStep(factor float64) {
	e.zoomAround(f32.Pt(float32(e.lastSize.X)/2, float32(e.lastSize.Y)/2), factor)
}

// scalePercent is the current zoom as a percentage of image pixels.
func (e *editor) scalePercent() int {
	return int(float64(e.lastScale)*100 + 0.5)
}

func (e *editor) handlePointer(pe pointer.Event) {
	if pe.Kind == pointer.Press {
		e.discardArmed = false
	}
	switch pe.Kind {
	case pointer.Move:
		e.hover = e.toImage(pe.Position)
		e.hoverPos = pe.Position
		e.hovering = true
		if e.tool == ToolSelect {
			e.hoverShape = e.hitShape(e.hover)
		}
	case pointer.Leave:
		e.hovering = false
		e.hoverShape = -1
	case pointer.Cancel:
		// The grab was taken away mid-gesture: drop the drag, keep nothing.
		e.cancelGesture()
		e.panning = false
	case pointer.Scroll:
		// Zoom around the cursor: keep the image point under the pointer
		// fixed by moving the pan offset along with the scale.
		e.zoomAround(pe.Position, 1.0-float64(pe.Scroll.Y)*0.0015)
	case pointer.Press:
		if pe.Buttons.Contain(pointer.ButtonTertiary) || (e.spaceDown && pe.Buttons.Contain(pointer.ButtonPrimary)) {
			e.panning = true
			e.panLast = pe.Position
			return
		}
		if !pe.Buttons.Contain(pointer.ButtonPrimary) {
			return // right-click and other buttons do not draw
		}
		ip := e.toImage(pe.Position)
		switch e.tool {
		case ToolSelectText:
			e.selPress(ip)
			return
		case ToolSelect:
			e.pressSelect(ip)
			return
		case ToolCrop:
			if h := e.cropHandleAt(pe.Position); h != cropbox.None {
				e.cropDrag = cropDragState{active: true, handle: h, from: ip, orig: *e.crop}
				return
			}
		}
		e.dragging = true
		e.dragFrom = ip
		e.dragTo = ip
		switch e.tool {
		case ToolText:
			// Click-to-place single-line text from the toolbar field. After
			// placing, clear the field so the next placement starts fresh.
			txt := e.textIn.Text()
			e.dragging = false
			if txt != "" {
				sh := shape{kind: kText, p0: ip, col: e.col, stroke: e.stroke, text: txt}
				sh.textOp, sh.textSize = textImage(txt, e.col, e.stroke)
				e.push(sh)
				e.textIn.SetText("")
			}
		case ToolStep:
			// Click-to-place an auto-incrementing badge, numbered after the
			// highest badge still placed (undo frees its number; a badge
			// deleted from the middle leaves a gap rather than a duplicate).
			e.dragging = false
			e.push(shape{kind: kStep, p0: ip, col: e.col, stroke: e.stroke, num: e.nextStep()})
		case ToolFreehand:
			e.freehandPts = []image.Point{ip}
		}
	case pointer.Drag:
		if e.panning {
			e.panX += pe.Position.X - e.panLast.X
			e.panY += pe.Position.Y - e.panLast.Y
			e.panLast = pe.Position
			return
		}
		ip := e.toImage(pe.Position)
		e.hoverPos = pe.Position
		switch {
		case e.tool == ToolSelectText:
			e.selDrag(ip)
		case e.moving:
			e.dragMove(ip)
		case e.cropDrag.active:
			r := cropbox.Drag(e.cropDrag.orig, e.cropDrag.handle, ip.Sub(e.cropDrag.from), minCrop, e.bounds)
			e.crop = &r // shown live; recorded on release
		case e.dragging:
			e.dragTo = ip
			if e.tool == ToolFreehand {
				e.freehandPts = append(e.freehandPts, e.dragTo)
			}
		}
	case pointer.Release:
		if e.panning {
			e.panning = false
			return
		}
		if e.tool == ToolSelectText {
			e.selRelease()
			return
		}
		if e.dragging {
			e.dragTo = e.toImage(pe.Position)
		}
		e.finishGesture()
	}
}

// finishGesture records a drag that is still in progress, as its release
// would: a move, a crop handle drag, or a drawing drag. Enter calls it too,
// so a crop being dragged when the editor is confirmed is applied.
func (e *editor) finishGesture() {
	switch {
	case e.moving:
		e.moving = false
		if i := e.selected; i >= 0 && i < len(e.shapes) && e.moveDelta != (image.Point{}) {
			e.shapes = e.hist.Do(e.shapes, oplog.Op[shape]{Kind: oplog.Modify, Index: i, Before: e.moveOrig, After: e.shapes[i]})
		}
	case e.cropDrag.active:
		e.cropDrag.active = false
		if e.crop != nil && *e.crop != e.cropDrag.orig {
			e.setCrop(*e.crop)
		}
	case e.dragging:
		e.dragging = false
		e.commitDrag()
	}
}

// cancelGesture abandons a drag in progress (Esc, or the pointer grab taken
// away) and restores what it changed. It reports whether there was one.
func (e *editor) cancelGesture() bool {
	switch {
	case e.moving:
		e.moving = false
		if e.selected >= 0 && e.selected < len(e.shapes) {
			e.shapes[e.selected] = e.moveOrig
		}
	case e.cropDrag.active:
		e.cropDrag.active = false
		e.recomputeCrop()
	case e.dragging:
		e.dragging = false
		e.freehandPts = nil
	default:
		return false
	}
	return true
}

func (e *editor) commitDrag() {
	from, to := e.dragFrom, e.dragTo
	switch e.tool {
	case ToolArrow:
		if from == to {
			return
		}
		e.push(shape{kind: kArrow, p0: from, p1: to, col: e.col, stroke: e.stroke})
	case ToolRect:
		r := rectOf(from, to)
		if r.Dx() < 1 || r.Dy() < 1 {
			return
		}
		e.push(shape{kind: kRect, p0: r.Min, p1: r.Max, col: e.col, stroke: e.stroke})
	case ToolEllip:
		r := rectOf(from, to)
		if r.Dx() < 1 || r.Dy() < 1 {
			return
		}
		e.push(shape{kind: kEllipse, p0: r.Min, p1: r.Max, col: e.col, stroke: e.stroke})
	case ToolLine:
		if from == to {
			return
		}
		e.push(shape{kind: kLine, p0: from, p1: to, col: e.col, stroke: e.stroke})
	case ToolBlur, ToolPixelate, ToolHighlight:
		r := rectOf(from, to)
		if r.Dx() < 1 || r.Dy() < 1 {
			return
		}
		e.push(shape{kind: rectToolKind(e.tool), p0: r.Min, p1: r.Max, col: e.col, stroke: e.stroke})
	case ToolFreehand:
		pts := e.freehandPts
		e.freehandPts = nil
		if len(pts) == 0 {
			return
		}
		cp := make([]image.Point, len(pts))
		copy(cp, pts)
		e.push(shape{kind: kFreehand, pts: cp, col: e.col, stroke: e.stroke})
	case ToolRedact:
		r := rectOf(from, to).Intersect(e.bounds)
		if r.Dx() < 1 || r.Dy() < 1 {
			return
		}
		col := e.col
		col.A = 0xff
		e.push(shape{kind: kRedact, rects: []image.Rectangle{r}, col: col})
	case ToolCrop:
		// A drag outside the current crop starts a new one, replacing it
		// (one undo step back to the old crop).
		r := rectOf(from, to).Intersect(e.bounds)
		if r.Dx() < minCrop || r.Dy() < minCrop {
			return
		}
		e.setCrop(r)
	}
}

// setCrop records r as the crop: the first crop is added, later ones modify
// that same entry, so undo walks back through the crop's earlier shapes.
func (e *editor) setCrop(r image.Rectangle) {
	sh := shape{kind: kCrop, p0: r.Min, p1: r.Max}
	if i := e.cropIndex(); i >= 0 {
		e.shapes = e.hist.Do(e.shapes, oplog.Op[shape]{Kind: oplog.Modify, Index: i, Before: e.shapes[i], After: sh})
		e.recomputeCrop()
		return
	}
	e.push(sh)
}

// cropIndex returns the index of the crop entry, or -1.
func (e *editor) cropIndex() int {
	for i := len(e.shapes) - 1; i >= 0; i-- {
		if e.shapes[i].kind == kCrop {
			return i
		}
	}
	return -1
}

func rectToolKind(t Tool) shapeKind {
	switch t {
	case ToolBlur:
		return kBlur
	case ToolPixelate:
		return kPixelate
	case ToolHighlight:
		return kHighlight
	}
	return kRect
}

// nextStep is the number for the next step badge: one past the highest
// badge still placed.
func (e *editor) nextStep() int {
	n := 0
	for _, s := range e.shapes {
		if s.kind == kStep {
			n = max(n, s.num)
		}
	}
	return n + 1
}

// push adds a new shape on top (one undo step).
func (e *editor) push(s shape) {
	e.shapes = e.hist.Do(e.shapes, oplog.Op[shape]{Kind: oplog.Add, Index: len(e.shapes), After: s})
	if s.kind == kCrop {
		e.recomputeCrop()
	}
}

func (e *editor) undo() {
	e.cancelGesture()
	shapes, op, ok := e.hist.Undo(e.shapes)
	if !ok {
		return
	}
	e.shapes = shapes
	// The discard prompt counts annotations; an undo changes the count, so
	// the next Esc/Cancel asks again (or, with none left, just cancels).
	e.discardArmed = false
	e.afterHistory(op, false)
}

func (e *editor) redoOne() {
	e.cancelGesture()
	shapes, op, ok := e.hist.Redo(e.shapes)
	if !ok {
		return
	}
	e.shapes = shapes
	e.afterHistory(op, true)
}

// afterHistory refreshes derived state after an undo or redo of op. In the
// Select tool the shape the step touched is selected, so a move or a delete
// that is undone shows what came back.
func (e *editor) afterHistory(op oplog.Op[shape], redo bool) {
	e.recomputeCrop()
	e.selected, e.hoverShape = -1, -1
	if e.tool != ToolSelect {
		return
	}
	switch {
	case op.Kind == oplog.Modify,
		op.Kind == oplog.Remove && !redo,
		op.Kind == oplog.Add && redo:
		if op.Index < len(e.shapes) && e.shapes[op.Index].kind != kCrop {
			e.selected = op.Index
		}
	}
}

// recomputeCrop sets e.crop to the most recent crop shape still in the stack.
func (e *editor) recomputeCrop() {
	e.crop = nil
	for _, s := range e.shapes {
		if s.kind == kCrop {
			r := rectOf(s.p0, s.p1)
			e.crop = &r
		}
	}
}

// toImage maps a window-space point to base-image pixel coordinates using the
// transform recorded during the last layout pass.
func (e *editor) toImage(p f32.Point) image.Point {
	s := e.lastScale
	if s == 0 {
		s = 1
	}
	x := (p.X - e.lastOrigin.X) / s
	y := (p.Y - e.lastOrigin.Y) / s
	return image.Pt(int(x), int(y))
}

// confirmNow renders the annotations and marks the editor done with the given
// action. Reused by the plain confirm button, each explicit action button and
// the keyboard shortcuts. A render failure ends the editor with that error
// (Run returns it), so the host reports it instead of leaving a dead button.
func (e *editor) confirmNow(action Action) {
	img, err := annotate.Render(e.base, e.crop, e.buildShapes())
	if err != nil {
		e.err = err
		e.action = ActionCancel
		e.done = true
		return
	}
	e.result = img
	e.action = action
	e.done = true
}

// textImage renders s exactly as annotate will (the default Go Regular face
// sized by stroke) onto a transparent image, for the on-canvas text preview.
func textImage(s string, col color.NRGBA, stroke int) (paint.ImageOp, image.Point) {
	sz := annotate.TextSize(s, stroke)
	if sz.X < 1 || sz.Y < 1 {
		return paint.ImageOp{}, image.Point{}
	}
	img, err := annotate.Render(image.NewRGBA(image.Rectangle{Max: sz}), nil, []annotate.Shape{
		annotate.Text{Text: s, Color: col, Stroke: stroke},
	})
	if err != nil {
		return paint.ImageOp{}, image.Point{}
	}
	// Linear filtering: the glyphs are anti-aliased and scale smoothly.
	return paint.NewImageOp(img), sz
}

// buildShapes converts UI shapes to annotate shapes, translating into
// cropped-image space when a crop is active.
func (e *editor) buildShapes() []annotate.Shape {
	var off image.Point
	if e.crop != nil {
		off = e.crop.Min
	}
	out := make([]annotate.Shape, 0, len(e.shapes))
	for _, s := range e.shapes {
		if a := toAnnotate(s, off); a != nil {
			out = append(out, a)
		}
	}
	return out
}

// toAnnotate converts one UI shape to its annotate shape, shifted by -off.
// A crop is not drawn and converts to nil.
func toAnnotate(s shape, off image.Point) annotate.Shape {
	switch s.kind {
	case kArrow:
		return annotate.Arrow{From: s.p0.Sub(off), To: s.p1.Sub(off), Color: s.col, Stroke: s.stroke}
	case kRect:
		return annotate.Rectangle{Rect: rectOf(s.p0, s.p1).Sub(off), Color: s.col, Stroke: s.stroke}
	case kEllipse:
		return annotate.Ellipse{Rect: rectOf(s.p0, s.p1).Sub(off), Color: s.col, Stroke: s.stroke}
	case kText:
		return annotate.Text{At: s.p0.Sub(off), Text: s.text, Color: s.col, Stroke: s.stroke}
	case kLine:
		return annotate.Line{From: s.p0.Sub(off), To: s.p1.Sub(off), Color: s.col, Stroke: s.stroke}
	case kBlur:
		// Stroke acts as a blur-strength multiplier; annotate enforces a
		// region-scaled redaction floor on top.
		return annotate.BlurRegion{Rect: rectOf(s.p0, s.p1).Sub(off), Radius: s.stroke}
	case kPixelate:
		// Stroke scales the mosaic block size.
		return annotate.Pixelate{Rect: rectOf(s.p0, s.p1).Sub(off), Block: s.stroke * 3}
	case kHighlight:
		return annotate.Highlight{Rect: rectOf(s.p0, s.p1).Sub(off), Color: s.col, Alpha: 0x60}
	case kStep:
		return annotate.StepBadge{Center: s.p0.Sub(off), Number: s.num, Color: s.col, Radius: badgeRadius(s.stroke)}
	case kFreehand:
		pts := make([]image.Point, len(s.pts))
		for i, p := range s.pts {
			pts[i] = p.Sub(off)
		}
		return annotate.Freehand{Points: pts, Color: s.col, Stroke: s.stroke}
	case kRedact:
		rs := make([]image.Rectangle, len(s.rects))
		for i, r := range s.rects {
			rs[i] = r.Sub(off)
		}
		return annotate.Redact{Rects: rs, Color: s.col}
	}
	return nil // kCrop: folded into the crop rect, not drawn
}

func rectOf(a, b image.Point) image.Rectangle {
	return image.Rect(a.X, a.Y, b.X, b.Y).Canon()
}
