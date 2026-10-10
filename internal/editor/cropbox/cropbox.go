// Package cropbox is the geometry of the editor's crop rectangle handles:
// which handle a pointer is on and how a drag on it resizes or moves the
// rectangle. Pure Go, no GUI toolkit, unit-tested with CGO off.
package cropbox

import "image"

// Handle is a part of the crop rectangle a drag can grab.
type Handle int

const (
	None Handle = iota
	NW
	N
	NE
	E
	SE
	S
	SW
	W
	// Inside moves the whole rectangle.
	Inside
)

// Handles lists the eight resize handles: the corners and the edge
// midpoints, clockwise from the top-left.
var Handles = [8]Handle{NW, N, NE, E, SE, S, SW, W}

// Point returns the centre of handle h on r.
func Point(r image.Rectangle, h Handle) image.Point {
	mx, my := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	switch h {
	case NW:
		return r.Min
	case N:
		return image.Pt(mx, r.Min.Y)
	case NE:
		return image.Pt(r.Max.X, r.Min.Y)
	case E:
		return image.Pt(r.Max.X, my)
	case SE:
		return r.Max
	case S:
		return image.Pt(mx, r.Max.Y)
	case SW:
		return image.Pt(r.Min.X, r.Max.Y)
	case W:
		return image.Pt(r.Min.X, my)
	}
	return image.Pt(mx, my)
}

// At returns what of r lies under p. half is half the hit square's side.
// Corners win over edges (so a small crop still resizes from a corner),
// edge handles are also hit anywhere along their edge, then the inside;
// None is outside. r, p and half share one space: the editor passes window
// pixels, so the hit area is the same physical size at any zoom and on
// HiDPI screens.
func At(r image.Rectangle, p image.Point, half int) Handle {
	near := func(c image.Point) bool {
		return abs(p.X-c.X) <= half && abs(p.Y-c.Y) <= half
	}
	for _, h := range []Handle{NW, NE, SE, SW, N, E, S, W} {
		if near(Point(r, h)) {
			return h
		}
	}
	inX := p.X >= r.Min.X && p.X <= r.Max.X
	inY := p.Y >= r.Min.Y && p.Y <= r.Max.Y
	switch {
	case inX && abs(p.Y-r.Min.Y) <= half:
		return N
	case inX && abs(p.Y-r.Max.Y) <= half:
		return S
	case inY && abs(p.X-r.Min.X) <= half:
		return W
	case inY && abs(p.X-r.Max.X) <= half:
		return E
	case inX && inY:
		return Inside
	}
	return None
}

// Drag returns r after dragging handle h by d. A corner moves its two
// sides, an edge its one side; a side stops minSize short of the opposite
// one and at the bounds. Inside moves the whole rectangle and slides along
// the border of bounds instead of shrinking.
func Drag(r image.Rectangle, h Handle, d image.Point, minSize int, bounds image.Rectangle) image.Rectangle {
	r = r.Canon()
	if h == Inside {
		r = r.Add(d)
		if r.Min.X < bounds.Min.X {
			r = r.Add(image.Pt(bounds.Min.X-r.Min.X, 0))
		}
		if r.Max.X > bounds.Max.X {
			r = r.Add(image.Pt(bounds.Max.X-r.Max.X, 0))
		}
		if r.Min.Y < bounds.Min.Y {
			r = r.Add(image.Pt(0, bounds.Min.Y-r.Min.Y))
		}
		if r.Max.Y > bounds.Max.Y {
			r = r.Add(image.Pt(0, bounds.Max.Y-r.Max.Y))
		}
		return r.Intersect(bounds)
	}
	switch h {
	case NW, W, SW:
		r.Min.X = clamp(r.Min.X+d.X, bounds.Min.X, r.Max.X-minSize)
	case NE, E, SE:
		r.Max.X = clamp(r.Max.X+d.X, r.Min.X+minSize, bounds.Max.X)
	}
	switch h {
	case NW, N, NE:
		r.Min.Y = clamp(r.Min.Y+d.Y, bounds.Min.Y, r.Max.Y-minSize)
	case SW, S, SE:
		r.Max.Y = clamp(r.Max.Y+d.Y, r.Min.Y+minSize, bounds.Max.Y)
	}
	return r.Intersect(bounds)
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
