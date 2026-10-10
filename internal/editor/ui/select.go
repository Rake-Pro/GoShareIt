//go:build darwin || windows || (linux && cgo)

package ui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/op"

	"github.com/Rake-Pro/GoShareIt/internal/editor/annotate"
	"github.com/Rake-Pro/GoShareIt/internal/editor/cropbox"
	"github.com/Rake-Pro/GoShareIt/internal/editor/oplog"
)

// hitTol is the hit slack in image pixels: a few window pixels at the
// current zoom, so thin strokes stay easy to pick.
func (e *editor) hitTol() float64 {
	if e.lastScale <= 0 {
		return 4
	}
	return float64(e.hitTolPx / e.lastScale)
}

// hitShape returns the index of the topmost selectable shape under the
// image point ip, or -1.
func (e *editor) hitShape(ip image.Point) int {
	list := make([]annotate.Shape, len(e.shapes))
	for i, s := range e.shapes {
		list[i] = toAnnotate(s, image.Point{})
	}
	return annotate.HitTest(list, ip, e.hitTol())
}

// selectedBounds is the image-space box of the selected shape.
func (e *editor) selectedBounds() (image.Rectangle, bool) {
	if e.selected < 0 || e.selected >= len(e.shapes) {
		return image.Rectangle{}, false
	}
	a := toAnnotate(e.shapes[e.selected], image.Point{})
	if a == nil {
		return image.Rectangle{}, false
	}
	return annotate.Bounds(a), true
}

// overSelection reports whether ip is on the selected shape's box (grown by
// the hit slack), where a drag moves it.
func (e *editor) overSelection(ip image.Point) bool {
	b, ok := e.selectedBounds()
	if !ok {
		return false
	}
	t := int(e.hitTol() + 0.5)
	return ip.In(b.Inset(-t))
}

// pressSelect selects the shape under ip (or keeps the selection when the
// press is on it) and starts a move; a press on nothing deselects.
func (e *editor) pressSelect(ip image.Point) {
	if !e.overSelection(ip) {
		i := e.hitShape(ip)
		if i != e.selected {
			e.hist.Seal()
		}
		e.selected = i
		if i < 0 {
			return
		}
	}
	e.moving = true
	e.moveFrom = ip
	e.moveDelta = image.Point{}
	e.moveOrig = e.shapes[e.selected]
}

// dragMove shows the selected shape moved to follow the pointer. A move
// that would take the shape entirely off the image is not applied, so it
// can always be grabbed again.
func (e *editor) dragMove(ip image.Point) {
	d := ip.Sub(e.moveFrom)
	moved := translateShape(e.moveOrig, d)
	if a := toAnnotate(moved, image.Point{}); a == nil || !annotate.Bounds(a).Overlaps(e.bounds) {
		return
	}
	e.shapes[e.selected] = moved
	e.moveDelta = d
}

// deselect clears the shape selection.
func (e *editor) deselect() {
	e.selected = -1
	e.hist.Seal()
}

// deleteSelected removes the selected shape (one undo step).
func (e *editor) deleteSelected() {
	i := e.selected
	if i < 0 || i >= len(e.shapes) || e.moving {
		return
	}
	e.shapes = e.hist.Do(e.shapes, oplog.Op[shape]{Kind: oplog.Remove, Index: i, Before: e.shapes[i]})
	e.selected, e.hoverShape = -1, -1
	e.discardArmed = false
}

// nudge moves the selected shape by d image pixels. A run of nudges on one
// shape is one undo step.
func (e *editor) nudge(d image.Point) {
	i := e.selected
	if i < 0 || i >= len(e.shapes) || e.moving {
		return
	}
	moved := translateShape(e.shapes[i], d)
	if a := toAnnotate(moved, image.Point{}); a == nil || !annotate.Bounds(a).Overlaps(e.bounds) {
		return
	}
	e.shapes = e.hist.DoMerge(e.shapes, oplog.Op[shape]{Kind: oplog.Modify, Index: i, Before: e.shapes[i], After: moved}, "nudge")
}

// setColor sets the drawing colour and, with a shape selected, recolours it.
func (e *editor) setColor(c color.NRGBA) {
	e.col = c
	e.restyleSelected(func(s *shape) bool {
		want := c
		if s.kind == kRedact {
			want.A = 0xff // Redact is always opaque
		}
		if s.col == want {
			return false
		}
		s.col = want
		return true
	})
}

// setStroke sets the stroke width and, with a shape selected, applies it.
func (e *editor) setStroke(n int) {
	n = min(max(n, 1), maxStroke)
	e.stroke = n
	e.restyleSelected(func(s *shape) bool {
		if s.kind == kRedact || s.stroke == n {
			return false // Redact has no stroke
		}
		s.stroke = n
		return true
	})
}

// restyleSelected applies change to a copy of the selected shape and
// records it when it changed anything. Steps of the same kind on one shape
// fold into one undo step.
func (e *editor) restyleSelected(change func(*shape) bool) {
	i := e.selected
	if e.tool != ToolSelect || i < 0 || i >= len(e.shapes) || e.moving {
		return
	}
	s := e.shapes[i]
	if !change(&s) {
		return
	}
	if s.kind == kText {
		s.textOp, s.textSize = textImage(s.text, s.col, s.stroke)
	}
	e.shapes = e.hist.DoMerge(e.shapes, oplog.Op[shape]{Kind: oplog.Modify, Index: i, Before: e.shapes[i], After: s}, "style")
}

// translateShape returns s moved by d. Point and box slices are copied, so
// the original (kept in the undo history) is never changed.
func translateShape(s shape, d image.Point) shape {
	s.p0 = s.p0.Add(d)
	s.p1 = s.p1.Add(d)
	if s.pts != nil {
		pts := make([]image.Point, len(s.pts))
		for i, p := range s.pts {
			pts[i] = p.Add(d)
		}
		s.pts = pts
	}
	if s.rects != nil {
		rs := make([]image.Rectangle, len(s.rects))
		for i, r := range s.rects {
			rs[i] = r.Add(d)
		}
		s.rects = rs
	}
	return s
}

// screenRect maps an image rectangle to window pixels.
func (e *editor) screenRect(r image.Rectangle) image.Rectangle {
	a, b := e.screen(r.Min), e.screen(r.Max)
	return image.Rect(int(a.X+0.5), int(a.Y+0.5), int(b.X+0.5), int(b.Y+0.5))
}

// cropHandleAt returns the crop handle under the window point pos, or None
// (no crop, another tool, or off the crop). Hit areas are in window pixels
// sized in dp, so they are the same physical size at any zoom and scale.
func (e *editor) cropHandleAt(pos f32.Point) cropbox.Handle {
	if e.tool != ToolCrop || e.crop == nil {
		return cropbox.None
	}
	return cropbox.At(e.screenRect(*e.crop), image.Pt(int(pos.X+0.5), int(pos.Y+0.5)), e.handlePx)
}

// handleCursor is the resize or move cursor for a crop handle.
func handleCursor(h cropbox.Handle) pointer.Cursor {
	switch h {
	case cropbox.NW, cropbox.SE:
		return pointer.CursorNorthWestSouthEastResize
	case cropbox.NE, cropbox.SW:
		return pointer.CursorNorthEastSouthWestResize
	case cropbox.N, cropbox.S:
		return pointer.CursorNorthSouthResize
	case cropbox.E, cropbox.W:
		return pointer.CursorEastWestResize
	case cropbox.Inside:
		return pointer.CursorAllScroll
	}
	return pointer.CursorCrosshair
}

// selectCursor is the Select tool's cursor: grabbing while moving, a move
// cursor over the selection, a hand over another shape.
func (e *editor) selectCursor() pointer.Cursor {
	switch {
	case e.moving:
		return pointer.CursorGrabbing
	case e.hovering && e.overSelection(e.hover):
		return pointer.CursorAllScroll
	case e.hovering && e.hoverShape >= 0:
		return pointer.CursorPointer
	}
	return pointer.CursorDefault
}

// drawSelection outlines the selected shape: a 1 px dashed accent box with
// small square marks at the corners (visual only).
func (e *editor) drawSelection(ops *op.Ops, mark int) {
	b, ok := e.selectedBounds()
	if !ok {
		return
	}
	r := e.screenRect(b).Inset(-3)
	acc := e.theme.accent
	dashedRect(ops, r, image.Rectangle{Max: e.lastSize}, acc)
	for _, c := range []image.Point{r.Min, {r.Max.X, r.Min.Y}, r.Max, {r.Min.X, r.Max.Y}} {
		sq := image.Rectangle{Min: c.Sub(image.Pt(mark/2, mark/2)), Max: c.Add(image.Pt(mark-mark/2, mark-mark/2))}
		fillRect(ops, f32.Pt(float32(sq.Min.X), float32(sq.Min.Y)), f32.Pt(float32(sq.Max.X), float32(sq.Max.Y)), acc)
	}
}

// drawCropHandles draws the eight crop handles as white squares with a dark
// edge, size window pixels across.
func (e *editor) drawCropHandles(ops *op.Ops, size int) {
	if e.crop == nil {
		return
	}
	r := e.screenRect(*e.crop)
	for _, h := range cropbox.Handles {
		c := cropbox.Point(r, h)
		sq := image.Rectangle{Min: c.Sub(image.Pt(size/2, size/2)), Max: c.Add(image.Pt(size-size/2, size-size/2))}
		fillRect(ops, f32.Pt(float32(sq.Min.X-1), float32(sq.Min.Y-1)), f32.Pt(float32(sq.Max.X+1), float32(sq.Max.Y+1)), color.NRGBA{A: 0xc0})
		fillRect(ops, f32.Pt(float32(sq.Min.X), float32(sq.Min.Y)), f32.Pt(float32(sq.Max.X), float32(sq.Max.Y)), color.NRGBA{0xff, 0xff, 0xff, 0xff})
	}
}

// dashedRect draws a 1 px dashed outline of r (4 on, 4 off), only where it
// crosses view (the canvas), so a shape zoomed far past the window costs a
// window's worth of dashes, not the whole outline's.
func dashedRect(ops *op.Ops, r, view image.Rectangle, col color.NRGBA) {
	const on, period = 4, 8
	// span returns the first dash start at or after lo (keeping the dash
	// phase anchored at from), and the end of the visible span.
	span := func(from, to, lo, hi int) (int, int) {
		start := from
		if lo > from {
			start = from + (lo-from)/period*period
		}
		return start, min(to, hi)
	}
	x0, x1 := span(r.Min.X, r.Max.X, view.Min.X, view.Max.X)
	for x := x0; x < x1; x += period {
		end := min(x+on, r.Max.X)
		if r.Min.Y >= view.Min.Y-1 && r.Min.Y <= view.Max.Y {
			fillRect(ops, f32.Pt(float32(x), float32(r.Min.Y)), f32.Pt(float32(end), float32(r.Min.Y+1)), col)
		}
		if r.Max.Y >= view.Min.Y && r.Max.Y <= view.Max.Y+1 {
			fillRect(ops, f32.Pt(float32(x), float32(r.Max.Y-1)), f32.Pt(float32(end), float32(r.Max.Y)), col)
		}
	}
	y0, y1 := span(r.Min.Y, r.Max.Y, view.Min.Y, view.Max.Y)
	for y := y0; y < y1; y += period {
		end := min(y+on, r.Max.Y)
		if r.Min.X >= view.Min.X-1 && r.Min.X <= view.Max.X {
			fillRect(ops, f32.Pt(float32(r.Min.X), float32(y)), f32.Pt(float32(r.Min.X+1), float32(end)), col)
		}
		if r.Max.X >= view.Min.X && r.Max.X <= view.Max.X+1 {
			fillRect(ops, f32.Pt(float32(r.Max.X-1), float32(y)), f32.Pt(float32(r.Max.X), float32(end)), col)
		}
	}
}
