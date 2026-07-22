//go:build darwin || windows

// Package region is the Gio-based interactive screen-region selector for
// GoShareIt. It shows a dimmed, borderless fullscreen overlay; the user drags a
// rectangle (press-drag-release) and confirms with the mouse release on a
// non-empty box or with Enter. Esc, an empty selection on Enter, or closing the
// window cancels.
//
// It is build-tagged for darwin and windows only because Gio requires cgo on
// macOS and a platform GPU backend on both; the Linux/CGO-disabled host build
// excludes this package entirely.
//
// Coordinate mapping: the overlay is a single fullscreen window on the PRIMARY
// display. Gio pointer positions and FrameEvent.Size are in device pixels with a
// top-left origin, so for the primary display the window-pixel rectangle equals
// the screen-pixel rectangle and is returned verbatim. The window->screen
// mapping, multi-monitor spanning, and per-monitor DPI scaling are the on-device
// risks: on a secondary monitor or a non-primary-origin layout the returned rect
// would be offset. For v1 we target the primary display only; see the package
// MUST-VERIFY notes. All pixel math (rectOf/canon) is pure and unit-testable.
//
// Threading contract: Gio's app.Main must own the process main goroutine (see
// cmd/goshareit-editor). Run is the window event loop and is expected to be
// started on a separate goroutine by main; it blocks until the user confirms,
// cancels, or closes the window, then returns the outcome.
package region

import (
	"image"
	"image/color"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Run shows the region overlay and returns the selected screen-pixel rectangle.
// ok is true only when the user confirms a non-empty selection; on Esc, an empty
// selection, or window close it is false and rect is the zero rectangle. err is
// non-nil only on a genuine Gio failure. Run blocks until the window closes and
// must be called on a goroutine other than the one running app.Main.
func Run() (rect image.Rectangle, ok bool, err error) {
	s := &selector{th: material.NewTheme()}
	w := new(app.Window)
	w.Option(
		app.Title("GoShareIt - Select Region"),
		app.Fullscreen.Option(),
		app.Decorated(false),
	)
	return s.loop(w)
}

const overlayTag = "goshareit.region.overlay"

type selector struct {
	th *material.Theme

	dragging bool
	from     image.Point
	to       image.Point

	winSize image.Point

	result    image.Rectangle
	confirmed bool
	done      bool
}

func (s *selector) loop(w *app.Window) (image.Rectangle, bool, error) {
	var ops op.Ops
	for {
		switch ev := w.Event().(type) {
		case app.DestroyEvent:
			if ev.Err != nil {
				return image.Rectangle{}, false, ev.Err
			}
			if s.confirmed {
				return s.result, true, nil
			}
			return image.Rectangle{}, false, nil
		case app.FrameEvent:
			gtx := app.NewContext(&ops, ev)
			s.winSize = ev.Size
			s.handleInput(gtx)
			if s.done {
				ev.Frame(gtx.Ops)
				if s.confirmed {
					return s.result, true, nil
				}
				return image.Rectangle{}, false, nil
			}
			s.layout(gtx)
			ev.Frame(gtx.Ops)
		}
	}
}

// handleInput drains queued key and pointer events before layout re-registers
// the input area for the next frame.
func (s *selector) handleInput(gtx layout.Context) {
	for {
		ev, ok := gtx.Source.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		switch ke.Name {
		case key.NameEscape:
			s.cancel()
			return
		case key.NameReturn, key.NameEnter:
			if s.confirm() {
				return
			}
			// Enter with no selection cancels.
			s.cancel()
			return
		}
	}

	for {
		ev, ok := gtx.Source.Event(pointer.Filter{
			Target: overlayTag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		s.handlePointer(pe)
		if s.done {
			return
		}
	}
}

func (s *selector) handlePointer(pe pointer.Event) {
	switch pe.Kind {
	case pointer.Press:
		s.dragging = true
		s.from = toPoint(pe.Position)
		s.to = s.from
	case pointer.Drag:
		if s.dragging {
			s.to = toPoint(pe.Position)
		}
	case pointer.Release:
		if !s.dragging {
			return
		}
		s.dragging = false
		s.to = toPoint(pe.Position)
		// Release on a non-empty box confirms; an empty box (a bare click) is a
		// no-op so the user can retry without leaving the overlay.
		s.confirm()
	}
}

// confirm finalizes the current selection if it is non-empty, returning true.
func (s *selector) confirm() bool {
	r := s.selection()
	if r.Empty() {
		return false
	}
	s.result = r
	s.confirmed = true
	s.done = true
	return true
}

func (s *selector) cancel() {
	s.confirmed = false
	s.done = true
}

// selection returns the current drag rectangle clamped to the window in screen
// pixels (top-left origin). It is empty when nothing meaningful is selected.
func (s *selector) selection() image.Rectangle {
	r := rectOf(s.from, s.to)
	r = r.Intersect(image.Rectangle{Max: s.winSize})
	if r.Dx() < 1 || r.Dy() < 1 {
		return image.Rectangle{}
	}
	return r
}

func (s *selector) layout(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max

	sel := s.activeRect()
	dim := color.NRGBA{0x00, 0x00, 0x00, 0xa0}
	if sel.Empty() {
		paint.FillShape(gtx.Ops, dim, clip.Rect{Max: size}.Op())
	} else {
		// Dim everything except the selection by filling the four surrounding
		// bands, leaving the selected region at full brightness.
		fillRect(gtx.Ops, image.Rect(0, 0, size.X, sel.Min.Y), dim)                 // top
		fillRect(gtx.Ops, image.Rect(0, sel.Max.Y, size.X, size.Y), dim)            // bottom
		fillRect(gtx.Ops, image.Rect(0, sel.Min.Y, sel.Min.X, sel.Max.Y), dim)      // left
		fillRect(gtx.Ops, image.Rect(sel.Max.X, sel.Min.Y, size.X, sel.Max.Y), dim) // right
		strokeRect(gtx.Ops, sel, 2, color.NRGBA{0xff, 0xff, 0xff, 0xff})
		s.drawReadout(gtx, sel)
	}

	// Register the whole window as the input area for the next frame.
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, event.Tag(overlayTag))
	pointer.CursorCrosshair.Add(gtx.Ops)
	area.Pop()

	return layout.Dimensions{Size: size}
}

// activeRect is the rectangle to draw: the in-progress drag while dragging, else
// nothing.
func (s *selector) activeRect() image.Rectangle {
	if !s.dragging {
		return image.Rectangle{}
	}
	return s.selection()
}

// drawReadout renders the "WxH" dimensions label just above (or below) the box.
func (s *selector) drawReadout(gtx layout.Context, sel image.Rectangle) {
	lbl := material.Label(s.th, unit.Sp(14), itoa(sel.Dx())+" x "+itoa(sel.Dy()))
	lbl.Color = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	lbl.Alignment = text.Start

	y := sel.Min.Y - gtx.Dp(unit.Dp(22))
	if y < 0 {
		y = sel.Max.Y + gtx.Dp(unit.Dp(4))
	}
	off := op.Offset(image.Pt(sel.Min.X, y)).Push(gtx.Ops)
	lbl.Layout(gtx)
	off.Pop()
}

func fillRect(ops *op.Ops, r image.Rectangle, col color.NRGBA) {
	r = r.Canon()
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}
	paint.FillShape(ops, col, clip.Rect(r).Op())
}

func strokeRect(ops *op.Ops, r image.Rectangle, width float32, col color.NRGBA) {
	min := f32.Pt(float32(r.Min.X), float32(r.Min.Y))
	max := f32.Pt(float32(r.Max.X), float32(r.Max.Y))
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(min)
	p.LineTo(f32.Pt(max.X, min.Y))
	p.LineTo(max)
	p.LineTo(f32.Pt(min.X, max.Y))
	p.LineTo(min)
	paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func toPoint(p f32.Point) image.Point {
	return image.Pt(int(p.X), int(p.Y))
}

// rectOf builds a canonical (non-negative size) rectangle from two corners.
func rectOf(a, b image.Point) image.Rectangle {
	return image.Rect(a.X, a.Y, b.X, b.Y).Canon()
}

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
