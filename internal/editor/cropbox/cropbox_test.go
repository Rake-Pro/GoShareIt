package cropbox

import (
	"image"
	"testing"
)

func TestPointAndAt(t *testing.T) {
	r := image.Rect(100, 100, 300, 200)
	want := map[Handle]image.Point{
		NW: {100, 100}, N: {200, 100}, NE: {300, 100}, E: {300, 150},
		SE: {300, 200}, S: {200, 200}, SW: {100, 200}, W: {100, 150},
	}
	for _, h := range Handles {
		if got := Point(r, h); got != want[h] {
			t.Errorf("Point(%d) = %v, want %v", h, got, want[h])
		}
		// Every handle is found at its centre and within the hit square.
		if got := At(r, want[h].Add(image.Pt(5, -5)), 6); got != h {
			t.Errorf("At near %d = %d", h, got)
		}
	}
	cases := []struct {
		p    image.Point
		want Handle
	}{
		{image.Pt(150, 102), N}, // along the top edge, away from the midpoint
		{image.Pt(298, 120), E}, // along the right edge
		{image.Pt(200, 150), Inside},
		{image.Pt(90, 150), None},    // outside, beyond the hit square
		{image.Pt(320, 220), None},   // past a corner
		{image.Pt(107, 107), Inside}, // one pixel past the corner square
	}
	for _, c := range cases {
		if got := At(r, c.p, 6); got != c.want {
			t.Errorf("At(%v) = %d, want %d", c.p, got, c.want)
		}
	}
	// A crop smaller than the hit squares still resizes from its corners.
	small := image.Rect(10, 10, 16, 16)
	if got := At(small, image.Pt(15, 15), 6); got != NW && got != SE {
		t.Errorf("tiny crop: At = %d, want a corner", got)
	}
}

func TestDrag(t *testing.T) {
	b := image.Rect(0, 0, 400, 300)
	r := image.Rect(100, 100, 300, 200)
	cases := []struct {
		name string
		h    Handle
		d    image.Point
		want image.Rectangle
	}{
		{"corner SE grows", SE, image.Pt(20, 30), image.Rect(100, 100, 320, 230)},
		{"corner NW shrinks", NW, image.Pt(10, 10), image.Rect(110, 110, 300, 200)},
		{"edge N moves only the top", N, image.Pt(50, -40), image.Rect(100, 60, 300, 200)},
		{"edge E moves only the right", E, image.Pt(-50, 99), image.Rect(100, 100, 250, 200)},
		{"edge stops at min size", W, image.Pt(500, 0), image.Rect(296, 100, 300, 200)},
		{"corner stops at the bounds", SE, image.Pt(500, 500), image.Rect(100, 100, 400, 300)},
		{"corner NW stops at the bounds", NW, image.Pt(-500, -500), image.Rect(0, 0, 300, 200)},
		{"move", Inside, image.Pt(30, -20), image.Rect(130, 80, 330, 180)},
		{"move slides along the border", Inside, image.Pt(500, -500), image.Rect(200, 0, 400, 100)},
		{"none is a no-op", None, image.Pt(10, 10), r},
	}
	for _, c := range cases {
		if got := Drag(r, c.h, c.d, 4, b); got != c.want {
			t.Errorf("%s: Drag = %v, want %v", c.name, got, c.want)
		}
	}
}
