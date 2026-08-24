//go:build darwin || windows

// Package ui is the Gio-based annotation canvas for the GoShareIt editor
// helper. It is build-tagged for darwin and windows only because Gio requires
// cgo on macOS and a platform GPU backend on both; the Linux/CGO-disabled host
// build excludes this package entirely. All pixel work is delegated to the
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

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Rake-Pro/GoShareIt/internal/editor/annotate"
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
)

// Options configures the initial editor state. Color is the initial stroke
// color; Stroke the initial width; Tools restricts which tools appear in the
// toolbar (empty -> all P4a tools); Tool is the initially selected tool.
// Theme is the resolved theme ("light" or "dark"; anything else, including
// empty, falls back to dark) - callers resolve "system" before calling Run.
// ConfirmLabel is rendered on the confirm button ("" -> "Done"). Actions, when
// true, renders the explicit Copy/Save/Upload action row; CanUpload controls
// whether the Upload button is enabled or shown greyed-out/inert.
type Options struct {
	Tool         Tool
	Color        color.NRGBA
	Stroke       int
	Tools        []Tool
	Theme        string
	ConfirmLabel string
	Actions      bool
	CanUpload    bool
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
	num    int           // step-badge number (kStep)
	pts    []image.Point // freehand polyline (kFreehand)
}

// Run shows the editor for img and returns the (possibly annotated) result.
// action reports which button the user confirmed with; on cancel, Esc, or
// window close it is ActionCancel and result is nil. err is non-nil only on a
// genuine Gio failure. Run blocks until the window closes and must be called
// on a goroutine other than the one running app.Main.
func Run(img image.Image, opts Options) (result image.Image, action Action, err error) {
	e := newEditor(img, opts)
	w := new(app.Window)
	w.Option(
		app.Title("GoShareIt - Annotate"),
		app.Size(unit.Dp(1000), unit.Dp(720)),
		// Keep the action row usable even when the user shrinks the window.
		app.MinSize(unit.Dp(640), unit.Dp(400)),
	)
	return e.loop(w)
}

type editor struct {
	base   image.Image
	bounds image.Rectangle // base bounds, origin-normalized size

	tools   []Tool
	tool    Tool
	col     color.NRGBA
	stroke  int
	palette []color.NRGBA

	shapes []shape
	redo   []shape
	crop   *image.Rectangle

	// in-progress drag (image coords)
	dragging    bool
	dragFrom    image.Point
	dragTo      image.Point
	freehandPts []image.Point // accumulated points for the active freehand stroke

	// view transform
	zoom       float64
	panX, panY float32
	lastOrigin f32.Point
	lastScale  float32

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

	imgOp paint.ImageOp

	// outcome
	result image.Image
	action Action
	done   bool
}

const canvasTag = "goshareit.canvas"

// badgeRadius is the step-badge disc radius in image pixels; shared by the
// annotate render and the on-canvas preview so they line up.
const badgeRadius = 14

func newEditor(img image.Image, opts Options) *editor {
	b := img.Bounds()
	// Normalize coordinates so shape space matches a 0,0-origin base.
	norm := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			norm.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}

	tools := opts.Tools
	if len(tools) == 0 {
		tools = []Tool{
			ToolCrop, ToolArrow, ToolRect, ToolEllip, ToolText,
			ToolBlur, ToolPixelate, ToolHighlight, ToolStep, ToolLine, ToolFreehand,
		}
	}
	tool := opts.Tool
	if tool == "" {
		tool = ToolArrow
	}
	col := opts.Color
	if col == (color.NRGBA{}) {
		col = color.NRGBA{R: 0xff, G: 0x3b, B: 0x30, A: 0xff}
	}
	stroke := opts.Stroke
	if stroke < 1 {
		stroke = 6
	}

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
	}
	e.toolBtns = make(map[Tool]*widget.Clickable, len(tools))
	for _, t := range tools {
		e.toolBtns[t] = new(widget.Clickable)
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
			// Window closed without confirm -> cancel.
			if e.action != ActionCancel {
				return e.result, e.action, nil
			}
			return nil, ActionCancel, nil
		case app.FrameEvent:
			gtx := app.NewContext(&ops, ev)
			e.handleInput(gtx)
			if e.done {
				ev.Frame(gtx.Ops)
				if e.action != ActionCancel {
					return e.result, e.action, nil
				}
				return nil, ActionCancel, nil
			}
			e.layout(gtx)
			ev.Frame(gtx.Ops)
		}
	}
}

// handleInput drains queued pointer and key events for the canvas before
// layout registers the next frame's input areas.
func (e *editor) handleInput(gtx layout.Context) {
	// Escape cancels.
	for {
		ev, ok := gtx.Source.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			e.action = ActionCancel
			e.done = true
			return
		}
	}

	for {
		ev, ok := gtx.Source.Event(pointer.Filter{
			Target: canvasTag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll,
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

func (e *editor) handlePointer(pe pointer.Event) {
	switch pe.Kind {
	case pointer.Scroll:
		// Zoom around the cursor.
		factor := 1.0 - float64(pe.Scroll.Y)*0.0015
		e.zoom *= factor
		if e.zoom < 0.05 {
			e.zoom = 0.05
		}
		if e.zoom > 20 {
			e.zoom = 20
		}
	case pointer.Press:
		ip := e.toImage(pe.Position)
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
				e.push(shape{kind: kText, p0: ip, col: e.col, stroke: e.stroke, text: txt})
				e.textIn.SetText("")
			}
		case ToolStep:
			// Click-to-place an auto-incrementing badge. The number is derived
			// from the count of step shapes currently in the stack so undo/redo
			// stays consistent (a removed step frees its number for the next).
			e.dragging = false
			e.push(shape{kind: kStep, p0: ip, col: e.col, stroke: e.stroke, num: e.stepCount() + 1})
		case ToolFreehand:
			e.freehandPts = []image.Point{ip}
		}
	case pointer.Drag:
		if e.dragging {
			e.dragTo = e.toImage(pe.Position)
			if e.tool == ToolFreehand {
				e.freehandPts = append(e.freehandPts, e.dragTo)
			}
		}
	case pointer.Release:
		if !e.dragging {
			return
		}
		e.dragging = false
		e.dragTo = e.toImage(pe.Position)
		e.commitDrag()
	}
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
	case ToolCrop:
		r := rectOf(from, to).Intersect(e.bounds)
		if r.Dx() < 1 || r.Dy() < 1 {
			return
		}
		cr := r
		e.crop = &cr
		e.push(shape{kind: kCrop, p0: r.Min, p1: r.Max})
	}
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

// stepCount returns how many step badges are currently committed.
func (e *editor) stepCount() int {
	n := 0
	for _, s := range e.shapes {
		if s.kind == kStep {
			n++
		}
	}
	return n
}

func (e *editor) push(s shape) {
	e.shapes = append(e.shapes, s)
	e.redo = e.redo[:0]
	if s.kind == kCrop {
		e.recomputeCrop()
	}
}

func (e *editor) undo() {
	if len(e.shapes) == 0 {
		return
	}
	last := e.shapes[len(e.shapes)-1]
	e.shapes = e.shapes[:len(e.shapes)-1]
	e.redo = append(e.redo, last)
	e.recomputeCrop()
}

func (e *editor) redoOne() {
	if len(e.redo) == 0 {
		return
	}
	last := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.shapes = append(e.shapes, last)
	e.recomputeCrop()
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
// action. Reused by the plain confirm button and each explicit action button.
func (e *editor) confirmNow(action Action) error {
	img, err := annotate.Render(e.base, e.crop, e.buildShapes())
	if err != nil {
		return err
	}
	e.result = img
	e.action = action
	e.done = true
	return nil
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
		switch s.kind {
		case kArrow:
			out = append(out, annotate.Arrow{From: s.p0.Sub(off), To: s.p1.Sub(off), Color: s.col, Stroke: s.stroke})
		case kRect:
			out = append(out, annotate.Rectangle{Rect: rectOf(s.p0, s.p1).Sub(off), Color: s.col, Stroke: s.stroke})
		case kEllipse:
			out = append(out, annotate.Ellipse{Rect: rectOf(s.p0, s.p1).Sub(off), Color: s.col, Stroke: s.stroke})
		case kText:
			out = append(out, annotate.Text{At: s.p0.Sub(off), Text: s.text, Color: s.col, Stroke: s.stroke})
		case kLine:
			out = append(out, annotate.Line{From: s.p0.Sub(off), To: s.p1.Sub(off), Color: s.col, Stroke: s.stroke})
		case kBlur:
			// Stroke acts as a blur-strength multiplier; annotate enforces a
			// region-scaled redaction floor on top.
			out = append(out, annotate.BlurRegion{Rect: rectOf(s.p0, s.p1).Sub(off), Radius: s.stroke})
		case kPixelate:
			// Stroke scales the mosaic block size.
			out = append(out, annotate.Pixelate{Rect: rectOf(s.p0, s.p1).Sub(off), Block: s.stroke * 3})
		case kHighlight:
			out = append(out, annotate.Highlight{Rect: rectOf(s.p0, s.p1).Sub(off), Color: s.col, Alpha: 0x60})
		case kStep:
			out = append(out, annotate.StepBadge{Center: s.p0.Sub(off), Number: s.num, Color: s.col, Radius: badgeRadius})
		case kFreehand:
			pts := make([]image.Point, len(s.pts))
			for i, p := range s.pts {
				pts[i] = p.Sub(off)
			}
			out = append(out, annotate.Freehand{Points: pts, Color: s.col, Stroke: s.stroke})
		case kCrop:
			// folded into the crop rect, not drawn
		}
	}
	return out
}

func rectOf(a, b image.Point) image.Rectangle {
	return image.Rect(a.X, a.Y, b.X, b.Y).Canon()
}
