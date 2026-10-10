package annotate

import (
	"image"
	"image/color"
	"testing"
)

func TestHitPerShape(t *testing.T) {
	red := color.NRGBA{R: 0xff, A: 0xff}
	cases := []struct {
		name string
		s    Shape
		p    image.Point
		tol  float64
		want bool
	}{
		{"line on stroke", Line{From: image.Pt(10, 10), To: image.Pt(110, 10), Stroke: 1}, image.Pt(60, 10), 0, true},
		{"thin line within tolerance", Line{From: image.Pt(10, 10), To: image.Pt(110, 10), Stroke: 1}, image.Pt(60, 13), 3, true},
		{"thin line outside tolerance", Line{From: image.Pt(10, 10), To: image.Pt(110, 10), Stroke: 1}, image.Pt(60, 15), 3, false},
		{"thick line by its width", Line{From: image.Pt(10, 10), To: image.Pt(110, 10), Stroke: 12}, image.Pt(60, 15), 0, true},
		{"line past its end", Line{From: image.Pt(10, 10), To: image.Pt(110, 10), Stroke: 2}, image.Pt(120, 10), 3, false},
		{"diagonal line", Line{From: image.Pt(0, 0), To: image.Pt(100, 100), Stroke: 1}, image.Pt(52, 50), 2, true},
		{"arrow shaft", Arrow{From: image.Pt(10, 50), To: image.Pt(200, 50), Color: red, Stroke: 2}, image.Pt(80, 51), 1, true},
		{"arrow head barb", Arrow{From: image.Pt(10, 50), To: image.Pt(200, 50), Color: red, Stroke: 2}, image.Pt(180, 58), 0, true},
		{"beside arrow", Arrow{From: image.Pt(10, 50), To: image.Pt(200, 50), Color: red, Stroke: 2}, image.Pt(80, 70), 3, false},
		{"freehand segment", Freehand{Points: []image.Point{{0, 0}, {50, 0}, {50, 50}}, Stroke: 2}, image.Pt(50, 25), 1, true},
		{"freehand single dot", Freehand{Points: []image.Point{{20, 20}}, Stroke: 4}, image.Pt(21, 21), 0, true},
		{"freehand miss", Freehand{Points: []image.Point{{0, 0}, {50, 0}, {50, 50}}, Stroke: 2}, image.Pt(20, 30), 3, false},
		{"rect edge", Rectangle{Rect: image.Rect(10, 10, 110, 60), Stroke: 2}, image.Pt(10, 30), 0, true},
		{"rect edge with tolerance", Rectangle{Rect: image.Rect(10, 10, 110, 60), Stroke: 1}, image.Pt(13, 30), 3, true},
		{"rect inside is not its stroke", Rectangle{Rect: image.Rect(10, 10, 110, 60), Stroke: 2}, image.Pt(60, 35), 2, false},
		{"ellipse ring", Ellipse{Rect: image.Rect(0, 0, 100, 50), Stroke: 2}, image.Pt(50, 0), 1, true},
		{"ellipse centre", Ellipse{Rect: image.Rect(0, 0, 100, 50), Stroke: 2}, image.Pt(50, 25), 2, false},
		{"blur inside", BlurRegion{Rect: image.Rect(0, 0, 40, 40), Radius: 4}, image.Pt(20, 20), 0, true},
		{"highlight outside", Highlight{Rect: image.Rect(0, 0, 40, 40)}, image.Pt(50, 20), 2, false},
		{"step disc", StepBadge{Center: image.Pt(50, 50), Number: 1, Radius: 14}, image.Pt(60, 58), 0, true},
		{"step outside", StepBadge{Center: image.Pt(50, 50), Number: 1, Radius: 14}, image.Pt(70, 70), 2, false},
		{"redact second box", Redact{Rects: []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(50, 50, 80, 60)}}, image.Pt(60, 55), 0, true},
		{"redact gap", Redact{Rects: []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(50, 50, 80, 60)}}, image.Pt(30, 30), 2, false},
		{"text box", Text{At: image.Pt(10, 10), Text: "Hello", Stroke: 2}, image.Pt(20, 20), 0, true},
		{"left of text", Text{At: image.Pt(10, 10), Text: "Hello", Stroke: 2}, image.Pt(2, 20), 2, false},
	}
	for _, c := range cases {
		if got := Hit(c.s, c.p, c.tol); got != c.want {
			t.Errorf("%s: Hit(%v) = %v, want %v", c.name, c.p, got, c.want)
		}
	}
}

func TestHitTestTopmostAndFrameInterior(t *testing.T) {
	shapes := []Shape{
		Rectangle{Rect: image.Rect(0, 0, 200, 200), Stroke: 2},           // 0: big frame
		Line{From: image.Pt(50, 100), To: image.Pt(150, 100), Stroke: 2}, // 1: inside the frame
		nil, // 2: a crop slot
		BlurRegion{Rect: image.Rect(90, 90, 110, 110), Radius: 3}, // 3: over the line
	}
	if got := HitTest(shapes, image.Pt(100, 100), 2); got != 3 {
		t.Errorf("overlap: got %d, want the topmost (3)", got)
	}
	if got := HitTest(shapes, image.Pt(60, 100), 2); got != 1 {
		t.Errorf("line inside frame: got %d, want 1", got)
	}
	if got := HitTest(shapes, image.Pt(30, 160), 2); got != 0 {
		t.Errorf("frame interior: got %d, want 0", got)
	}
	if got := HitTest(shapes, image.Pt(300, 300), 2); got != -1 {
		t.Errorf("outside everything: got %d, want -1", got)
	}
}

func TestBoundsCoversShape(t *testing.T) {
	cases := []struct {
		s    Shape
		want image.Rectangle // must be contained in Bounds
	}{
		{Line{From: image.Pt(10, 20), To: image.Pt(110, 20), Stroke: 4}, image.Rect(10, 18, 111, 23)},
		{Arrow{From: image.Pt(10, 50), To: image.Pt(200, 50), Stroke: 2}, image.Rect(10, 45, 201, 56)},
		{Freehand{Points: []image.Point{{5, 5}, {40, 30}}, Stroke: 2}, image.Rect(5, 5, 41, 31)},
		{Rectangle{Rect: image.Rect(10, 10, 50, 50), Stroke: 6}, image.Rect(7, 7, 53, 53)},
		{StepBadge{Center: image.Pt(30, 30), Radius: 10}, image.Rect(20, 20, 41, 41)},
		{Redact{Rects: []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(50, 50, 80, 60)}}, image.Rect(0, 0, 80, 60)},
	}
	for i, c := range cases {
		if b := Bounds(c.s); !c.want.In(b) {
			t.Errorf("case %d: Bounds = %v does not cover %v", i, b, c.want)
		}
	}
	if b := Bounds(Text{At: image.Pt(5, 5), Text: "Hi", Stroke: 1}); b.Min != image.Pt(5, 5) || b.Dx() < 1 || b.Dy() < 1 {
		t.Errorf("text bounds = %v", b)
	}
}
