// Package flow is the pure geometry behind the editor toolbar's wrapping
// rows: items of known sizes are placed left to right and wrap to the next
// row when the width runs out, so no button is ever cut off or scrolled out
// of view. It has no GUI dependency (the Gio wrapper lives in
// internal/editor/ui), so the wrapping rules are unit-tested on every
// platform.
package flow

import "image"

// Width is the width of widths on one row with gap between neighbours.
func Width(widths []int, gap int) int {
	w := 0
	for i, x := range widths {
		if i > 0 {
			w += gap
		}
		w += x
	}
	return w
}

// Fits reports whether widths fit on one row of maxW with gap between them.
func Fits(widths []int, maxW, gap int) bool { return Width(widths, gap) <= maxW }

// Wrap places items of the given sizes greedily left to right, starting a
// new row when the next item would pass maxW. gap separates items in a row
// and rowGap separates rows; items are centred vertically in their row.
// With right, each row is shifted to end at maxW. An item wider than maxW
// gets a row of its own and is never split. It returns each item's top-left
// corner and the bounding size (maxW wide when right is set).
func Wrap(sizes []image.Point, maxW, gap, rowGap int, right bool) ([]image.Point, image.Point) {
	pos := make([]image.Point, len(sizes))
	var total image.Point
	y := 0
	for start := 0; start < len(sizes); {
		end, w, h := start, 0, 0
		for end < len(sizes) {
			add := sizes[end].X
			if end > start {
				add += gap
			}
			if end > start && w+add > maxW {
				break
			}
			w += add
			h = max(h, sizes[end].Y)
			end++
		}
		x := 0
		if right {
			x = max(maxW-w, 0)
		}
		for i := start; i < end; i++ {
			pos[i] = image.Pt(x, y+(h-sizes[i].Y)/2)
			x += sizes[i].X + gap
		}
		if start > 0 {
			total.Y += rowGap
		}
		total.Y += h
		total.X = max(total.X, w)
		y = total.Y + rowGap
		start = end
	}
	if right && len(sizes) > 0 {
		total.X = max(total.X, maxW)
	}
	return pos, total
}

// Bar places a left group and a right group of items. When both fit on one
// row with groupGap between them, the left group starts at 0 and the right
// group ends at maxW. Otherwise each group wraps on rows of its own (Wrap):
// the left group first, then the right group right-aligned below it. The
// returned positions hold the left items, then the right items. When one
// group is empty the other simply wraps (Wrap; the right group stays
// right-aligned).
func Bar(left, right []image.Point, maxW, gap, groupGap, rowGap int) ([]image.Point, image.Point) {
	switch {
	case len(right) == 0:
		return Wrap(left, maxW, gap, rowGap, false)
	case len(left) == 0:
		return Wrap(right, maxW, gap, rowGap, true)
	}
	lw, rw := Width(xs(left), gap), Width(xs(right), gap)
	if lw+groupGap+rw <= maxW {
		h := 0
		for _, s := range left {
			h = max(h, s.Y)
		}
		for _, s := range right {
			h = max(h, s.Y)
		}
		pos := make([]image.Point, 0, len(left)+len(right))
		x := 0
		for _, s := range left {
			pos = append(pos, image.Pt(x, (h-s.Y)/2))
			x += s.X + gap
		}
		x = max(maxW-rw, lw+groupGap)
		for _, s := range right {
			pos = append(pos, image.Pt(x, (h-s.Y)/2))
			x += s.X + gap
		}
		return pos, image.Pt(maxW, h)
	}
	lp, ls := Wrap(left, maxW, gap, rowGap, false)
	rp, rs := Wrap(right, maxW, gap, rowGap, true)
	off := ls.Y + rowGap
	for i := range rp {
		rp[i].Y += off
	}
	return append(lp, rp...), image.Pt(max(ls.X, rs.X), off+rs.Y)
}

// MaxHeight is the tallest Bar any of the left-group variants needs with
// right at this width. The toolbar reserves it, so switching tools (each
// tool adds its own controls) never changes the toolbar height and the
// canvas never re-fits.
func MaxHeight(variants [][]image.Point, right []image.Point, maxW, gap, groupGap, rowGap int) int {
	h := 0
	for _, left := range variants {
		_, total := Bar(left, right, maxW, gap, groupGap, rowGap)
		h = max(h, total.Y)
	}
	return h
}

// Stretch sizes a flexible item (the annotation text field): when the row
// has at least lo to spare after used, the item takes the spare width up to
// hi; otherwise it gets fallback (capped at maxW) and its group wraps.
func Stretch(used, maxW, lo, hi, fallback int) int {
	if spare := maxW - used; spare >= lo {
		return min(spare, hi)
	}
	return min(fallback, maxW)
}

func xs(sizes []image.Point) []int {
	out := make([]int, len(sizes))
	for i, s := range sizes {
		out[i] = s.X
	}
	return out
}
