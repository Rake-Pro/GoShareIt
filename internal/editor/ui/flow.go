//go:build darwin || windows || (linux && cgo)

package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/Rake-Pro/GoShareIt/internal/editor/flow"
)

// measured is a widget laid out into a recording, to be placed later.
type measured struct {
	call op.CallOp
	size image.Point
}

// measure lays w out with no minimum size and records it, so its size is
// known before it is placed.
func measure(gtx layout.Context, w layout.Widget) measured {
	gtx.Constraints.Min = image.Point{}
	m := op.Record(gtx.Ops)
	dims := w(gtx)
	return measured{call: m.Stop(), size: dims.Size}
}

// measureAll measures each widget in order.
func measureAll(gtx layout.Context, ws []layout.Widget) []measured {
	out := make([]measured, len(ws))
	for i, w := range ws {
		out[i] = measure(gtx, w)
	}
	return out
}

func sizesOf(ms []measured) []image.Point {
	out := make([]image.Point, len(ms))
	for i, m := range ms {
		out[i] = m.size
	}
	return out
}

// place replays measured widgets at pos and reports the bounding size.
func place(gtx layout.Context, ms []measured, pos []image.Point, total image.Point) layout.Dimensions {
	for i, m := range ms {
		st := op.Offset(pos[i]).Push(gtx.Ops)
		m.call.Add(gtx.Ops)
		st.Pop()
	}
	return layout.Dimensions{Size: total}
}

// layoutWrap is a flow layout: the widgets go left to right and wrap to the
// next row when the width runs out (see flow.Wrap). Nothing scrolls or is
// cut off.
func layoutWrap(gtx layout.Context, ws ...layout.Widget) layout.Dimensions {
	ms := measureAll(gtx, ws)
	pos, total := flow.Wrap(sizesOf(ms), gtx.Constraints.Max.X, 0, 0, false)
	return place(gtx, ms, pos, total)
}
