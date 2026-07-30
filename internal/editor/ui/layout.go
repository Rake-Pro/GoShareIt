//go:build darwin || windows

package ui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (e *editor) layout(gtx layout.Context) layout.Dimensions {
	e.handleWidgets(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return e.layoutToolbar(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return e.layoutCanvas(gtx)
		}),
	)
}

// handleWidgets reacts to toolbar button clicks.
func (e *editor) handleWidgets(gtx layout.Context) {
	for t, b := range e.toolBtns {
		if b.Clicked(gtx) {
			e.tool = t
			// Selecting the text tool focuses the toolbar field so the user can
			// type immediately, then click the canvas to place the text.
			if t == ToolText {
				gtx.Execute(key.FocusCmd{Tag: &e.textIn})
			}
		}
	}
	for i, b := range e.swatchBtn {
		if b.Clicked(gtx) {
			e.col = e.palette[i]
		}
	}
	if e.strokeInc.Clicked(gtx) && e.stroke < 32 {
		e.stroke++
	}
	if e.strokeDec.Clicked(gtx) && e.stroke > 1 {
		e.stroke--
	}
	if e.undoBtn.Clicked(gtx) {
		e.undo()
	}
	if e.redoBtn.Clicked(gtx) {
		e.redoOne()
	}
	if e.cancelB.Clicked(gtx) {
		e.confirmed = false
		e.done = true
	}
	if e.confirm.Clicked(gtx) {
		// Render error is surfaced by leaving done set with no result; the
		// helper treats a nil confirmed result as a failure path.
		_ = e.confirmNow()
	}
}

// layoutToolbar renders two rows: a horizontally scrollable row of tool
// buttons + color swatches (whose width varies with the configured tool set),
// and a fixed action row (stroke, text field, undo/redo/cancel/confirm). The
// split guarantees Confirm/Cancel stay visible at any window width - a single
// flex row used to push them out of view on narrower screens. The whole row
// gets an explicit themed background fill spanning the full window width, so
// the window's default (light) surface never shows through.
func (e *editor) layoutToolbar(gtx layout.Context) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := e.layoutToolbarContent(gtx)
	call := macro.Stop()
	bgSize := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
	paint.FillShape(gtx.Ops, e.theme.toolbarBg, clip.Rect{Max: bgSize}.Op())
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: bgSize}
}

func (e *editor) layoutToolbarContent(gtx layout.Context) layout.Dimensions {
	th := e.th
	inset := layout.UniformInset(unit.Dp(8))
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Row 1: tools + swatches, scrollable.
		items := make([]layout.Widget, 0, len(e.tools)+len(e.palette))
		for _, t := range e.tools {
			t := t
			btn := e.subtleButton(e.toolBtns[t], toolLabel(t))
			if e.tool == t {
				btn = e.accentButton(e.toolBtns[t], toolLabel(t))
			}
			items = append(items, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(4)).Layout(gtx, btn.Layout)
			})
		}
		for i := range e.palette {
			i := i
			items = append(items, func(gtx layout.Context) layout.Dimensions {
				return e.layoutSwatch(gtx, i)
			})
		}

		// Row 2: stroke controls | text input | undo/redo | cancel/confirm,
		// visually grouped with spacers.
		dec := e.subtleButton(&e.strokeDec, "-")
		inc := e.subtleButton(&e.strokeInc, "+")
		undo := e.subtleButton(&e.undoBtn, "Undo")
		redo := e.subtleButton(&e.redoBtn, "Redo")
		cancel := e.subtleButton(&e.cancelB, "Cancel")
		ok := e.accentButton(&e.confirm, e.confirmLabel)

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return e.toolRow.Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
					return items[i](gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					rigidBtn(dec),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(th, itoa(e.stroke)+" px")
						lbl.Color = e.theme.fg
						return layout.UniformInset(unit.Dp(6)).Layout(gtx, lbl.Layout)
					}),
					rigidBtn(inc),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return e.layoutTextField(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					rigidBtn(undo),
					rigidBtn(redo),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					rigidBtn(cancel),
					rigidBtn(ok),
				)
			}),
		)
	})
}

// styledButton applies uniform geometry (corner radius, text size) plus the
// given background/foreground on top of material.Button's defaults.
func (e *editor) styledButton(btn *widget.Clickable, label string, bg, fg color.NRGBA) material.ButtonStyle {
	b := material.Button(e.th, btn, label)
	b.Background = bg
	b.Color = fg
	b.CornerRadius = unit.Dp(6)
	b.TextSize = unit.Sp(13)
	return b
}

// accentButton is the primary style: accent-filled with contrast text. Used
// for the selected tool and Confirm.
func (e *editor) accentButton(btn *widget.Clickable, label string) material.ButtonStyle {
	return e.styledButton(btn, label, e.theme.accent, e.theme.contrastFg)
}

// subtleButton is the secondary style: a flat themed surface with regular
// text. Used for unselected tools, undo/redo, and Cancel.
func (e *editor) subtleButton(btn *widget.Clickable, label string) material.ButtonStyle {
	return e.styledButton(btn, label, e.theme.surfaceBg, e.theme.fg)
}

func rigidBtn(b material.ButtonStyle) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(4)).Layout(gtx, b.Layout)
	})
}

// layoutTextField renders the annotation text input with a subtle themed
// background pill so it reads as an input rather than bare text.
func (e *editor) layoutTextField(gtx layout.Context) layout.Dimensions {
	ed := material.Editor(e.th, &e.textIn, "text...")
	ed.Color = e.theme.fg
	ed.HintColor = mutedColor(e.theme.fg)
	return layoutPill(gtx, e.theme.surfaceBg, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(6)).Layout(gtx, ed.Layout)
	})
}

// layoutPill paints a rounded background behind w, sized to w's own
// dimensions (stretched to at least the incoming width).
func layoutPill(gtx layout.Context, bg color.NRGBA, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	rr := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: 6, SW: 6, NE: 6, NW: 6}
	paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// mutedColor returns c with reduced alpha, for secondary text like hints.
func mutedColor(c color.NRGBA) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 0xa0}
}

func (e *editor) layoutSwatch(gtx layout.Context, i int) layout.Dimensions {
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		sz := gtx.Dp(unit.Dp(22))
		return e.swatchBtn[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			d := image.Pt(sz, sz)
			rr := clip.RRect{Rect: image.Rectangle{Max: d}, SE: 4, SW: 4, NE: 4, NW: 4}
			paint.FillShape(gtx.Ops, e.palette[i], rr.Op(gtx.Ops))
			// A pure-white swatch is invisible against a light toolbar/canvas
			// background; give it a hairline border in light mode regardless
			// of selection.
			if e.theme == lightTheme && e.palette[i] == (color.NRGBA{0xff, 0xff, 0xff, 0xff}) {
				hairline := clip.Stroke{Path: rr.Path(gtx.Ops), Width: 1}.Op()
				paint.FillShape(gtx.Ops, color.NRGBA{0, 0, 0, 0x40}, hairline)
			}
			if colorsEqual(e.palette[i], e.col) {
				// Ring uses the theme fg color so it stays visible against
				// both the dark palette swatch colors and the theme bg.
				border := clip.Stroke{Path: rr.Path(gtx.Ops), Width: 2}.Op()
				paint.FillShape(gtx.Ops, e.theme.fg, border)
			}
			return layout.Dimensions{Size: d}
		})
	})
}

func (e *editor) layoutCanvas(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	// Background.
	paint.FillShape(gtx.Ops, e.theme.canvasBg, clip.Rect{Max: size}.Op())

	// Fit-to-window scale, modified by user zoom.
	bw := float32(e.bounds.Dx())
	bh := float32(e.bounds.Dy())
	if bw == 0 || bh == 0 {
		return layout.Dimensions{Size: size}
	}
	fit := math.Min(float64(size.X)/float64(bw), float64(size.Y)/float64(bh))
	scale := float32(fit * e.zoom)
	dispW := bw * scale
	dispH := bh * scale
	origin := f32.Pt(
		(float32(size.X)-dispW)/2+e.panX,
		(float32(size.Y)-dispH)/2+e.panY,
	)
	e.lastScale = scale
	e.lastOrigin = origin

	// Draw the image.
	{
		stack := op.Affine(f32.Affine2D{}.
			Scale(f32.Pt(0, 0), f32.Pt(scale, scale)).
			Offset(origin)).Push(gtx.Ops)
		e.imgOp.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}

	// Draw committed shapes and the in-progress drag in screen space.
	for _, s := range e.shapes {
		e.drawShape(gtx.Ops, s)
	}
	if e.dragging {
		e.drawShape(gtx.Ops, e.previewShape())
	}

	// Register the canvas input area for the next frame.
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, event.Tag(canvasTag))
	pointer.CursorCrosshair.Add(gtx.Ops)
	area.Pop()

	return layout.Dimensions{Size: size}
}

func (e *editor) previewShape() shape {
	kind := kArrow
	switch e.tool {
	case ToolRect:
		kind = kRect
	case ToolEllip:
		kind = kEllipse
	case ToolCrop:
		kind = kCrop
	case ToolLine:
		kind = kLine
	case ToolBlur:
		kind = kBlur
	case ToolPixelate:
		kind = kPixelate
	case ToolHighlight:
		kind = kHighlight
	case ToolFreehand:
		return shape{kind: kFreehand, pts: e.freehandPts, col: e.col, stroke: e.stroke}
	}
	return shape{kind: kind, p0: e.dragFrom, p1: e.dragTo, col: e.col, stroke: e.stroke}
}

func (e *editor) screen(p image.Point) f32.Point {
	return f32.Pt(
		e.lastOrigin.X+float32(p.X)*e.lastScale,
		e.lastOrigin.Y+float32(p.Y)*e.lastScale,
	)
}

func (e *editor) drawShape(ops *op.Ops, s shape) {
	w := float32(s.stroke) * e.lastScale
	if w < 1 {
		w = 1
	}
	switch s.kind {
	case kArrow:
		a, b := e.screen(s.p0), e.screen(s.p1)
		strokeLine(ops, a, b, w, s.col)
		drawArrowHead(ops, a, b, w, s.col)
	case kRect:
		r := rectOf(s.p0, s.p1)
		strokeRect(ops, e.screen(r.Min), e.screen(r.Max), w, s.col)
	case kEllipse:
		r := rectOf(s.p0, s.p1)
		min, max := e.screen(r.Min), e.screen(r.Max)
		el := clip.Ellipse{Min: image.Pt(int(min.X), int(min.Y)), Max: image.Pt(int(max.X), int(max.Y))}
		paint.FillShape(ops, s.col, clip.Stroke{Path: el.Path(ops), Width: w}.Op())
	case kText:
		// Approximate preview marker; final raster is produced by annotate.
		p := e.screen(s.p0)
		dot := clip.Rect{Min: image.Pt(int(p.X), int(p.Y)), Max: image.Pt(int(p.X)+4, int(p.Y)+4)}
		paint.FillShape(ops, s.col, dot.Op())
	case kLine:
		strokeLine(ops, e.screen(s.p0), e.screen(s.p1), w, s.col)
	case kFreehand:
		if len(s.pts) >= 2 {
			var p clip.Path
			p.Begin(ops)
			p.MoveTo(e.screen(s.pts[0]))
			for _, pt := range s.pts[1:] {
				p.LineTo(e.screen(pt))
			}
			paint.FillShape(ops, s.col, clip.Stroke{Path: p.End(), Width: w}.Op())
		} else if len(s.pts) == 1 {
			c := e.screen(s.pts[0])
			dot := clip.Rect{Min: image.Pt(int(c.X), int(c.Y)), Max: image.Pt(int(c.X)+int(w)+1, int(c.Y)+int(w)+1)}
			paint.FillShape(ops, s.col, dot.Op())
		}
	case kBlur, kPixelate:
		// Approximate preview: translucent fill plus outline. The real pixel
		// effect (box-blur / mosaic) is applied by annotate on Confirm.
		r := rectOf(s.p0, s.p1)
		min, max := e.screen(r.Min), e.screen(r.Max)
		fillRect(ops, min, max, color.NRGBA{0x80, 0x80, 0x80, 0x60})
		strokeRect(ops, min, max, 2, color.NRGBA{0xff, 0xff, 0xff, 0xc0})
	case kHighlight:
		// Approximate preview: translucent tint in the chosen color; annotate
		// composites the same color over the region on Confirm.
		r := rectOf(s.p0, s.p1)
		min, max := e.screen(r.Min), e.screen(r.Max)
		tint := color.NRGBA{s.col.R, s.col.G, s.col.B, 0x60}
		fillRect(ops, min, max, tint)
	case kStep:
		// Filled disc preview; the centered number is rasterized by annotate.
		c := e.screen(s.p0)
		rad := float32(badgeRadius) * e.lastScale
		if rad < 3 {
			rad = 3
		}
		el := clip.Ellipse{
			Min: image.Pt(int(c.X-rad), int(c.Y-rad)),
			Max: image.Pt(int(c.X+rad), int(c.Y+rad)),
		}
		paint.FillShape(ops, s.col, el.Op(ops))
	case kCrop:
		r := rectOf(s.p0, s.p1)
		strokeRect(ops, e.screen(r.Min), e.screen(r.Max), 2, color.NRGBA{0xff, 0xff, 0xff, 0xff})
	}
}

func strokeLine(ops *op.Ops, a, b f32.Point, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(a)
	p.LineTo(b)
	spec := p.End()
	paint.FillShape(ops, col, clip.Stroke{Path: spec, Width: width}.Op())
}

func fillRect(ops *op.Ops, min, max f32.Point, col color.NRGBA) {
	r := image.Rect(int(min.X), int(min.Y), int(max.X), int(max.Y)).Canon()
	paint.FillShape(ops, col, clip.Rect(r).Op())
}

func strokeRect(ops *op.Ops, min, max f32.Point, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(min)
	p.LineTo(f32.Pt(max.X, min.Y))
	p.LineTo(max)
	p.LineTo(f32.Pt(min.X, max.Y))
	p.LineTo(min)
	spec := p.End()
	paint.FillShape(ops, col, clip.Stroke{Path: spec, Width: width}.Op())
}

func drawArrowHead(ops *op.Ops, from, to f32.Point, width float32, col color.NRGBA) {
	dx := float64(to.X - from.X)
	dy := float64(to.Y - from.Y)
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l, dy/l
	head := 6*float64(width) + l*0.12
	if head < 8 {
		head = 8
	}
	const ang = 0.5
	cosA, sinA := math.Cos(ang), math.Sin(ang)
	rx1 := -(ux*cosA - uy*sinA)
	ry1 := -(ux*sinA + uy*cosA)
	rx2 := -(ux*cosA + uy*sinA)
	ry2 := -(-ux*sinA + uy*cosA)
	p1 := f32.Pt(to.X+float32(rx1*head), to.Y+float32(ry1*head))
	p2 := f32.Pt(to.X+float32(rx2*head), to.Y+float32(ry2*head))
	strokeLine(ops, to, p1, width, col)
	strokeLine(ops, to, p2, width, col)
}

func toolLabel(t Tool) string {
	switch t {
	case ToolCrop:
		return "Crop"
	case ToolArrow:
		return "Arrow"
	case ToolRect:
		return "Rect"
	case ToolEllip:
		return "Ellipse"
	case ToolText:
		return "Text"
	case ToolBlur:
		return "Blur"
	case ToolPixelate:
		return "Pixelate"
	case ToolHighlight:
		return "Highlight"
	case ToolStep:
		return "Step"
	case ToolLine:
		return "Line"
	case ToolFreehand:
		return "Freehand"
	}
	return string(t)
}

func colorsEqual(a, b color.NRGBA) bool { return a == b }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
