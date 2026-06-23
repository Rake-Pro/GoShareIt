package annotate

import (
	"image"
	"image/color"
	"testing"
)

func blank(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	white := color.RGBA{255, 255, 255, 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, white)
		}
	}
	return img
}

func countChanged(t *testing.T, got image.Image, base *image.RGBA) int {
	t.Helper()
	n := 0
	b := got.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			gr, gg, gb, ga := got.At(x, y).RGBA()
			br, bg, bb, ba := base.At(x, y).RGBA()
			if gr != br || gg != bg || gb != bb || ga != ba {
				n++
			}
		}
	}
	return n
}

// anyChangedInRect reports whether any pixel inside r differs from white.
func anyChangedInRect(got image.Image, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := got.At(x, y).RGBA()
			if !(cr == 0xffff && cg == 0xffff && cb == 0xffff) {
				return true
			}
		}
	}
	return false
}

func TestCropBounds(t *testing.T) {
	base := blank(100, 80)
	out := Crop(base, image.Rect(10, 20, 60, 50))
	if got := out.Bounds(); got != image.Rect(0, 0, 50, 30) {
		t.Fatalf("crop bounds = %v, want 0,0,50,30", got)
	}
}

func TestCropClampsToImage(t *testing.T) {
	base := blank(40, 40)
	out := Crop(base, image.Rect(20, 20, 200, 200))
	if got := out.Bounds(); got != image.Rect(0, 0, 20, 20) {
		t.Fatalf("crop bounds = %v, want 0,0,20,20", got)
	}
}

func TestRenderCropApplied(t *testing.T) {
	base := blank(100, 100)
	crop := image.Rect(25, 25, 75, 75)
	out, err := Render(base, &crop, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Bounds(); got != image.Rect(0, 0, 50, 50) {
		t.Fatalf("render crop bounds = %v, want 0,0,50,50", got)
	}
}

func TestRenderDoesNotMutateBase(t *testing.T) {
	base := blank(50, 50)
	shapes := []Shape{Rectangle{Rect: image.Rect(5, 5, 45, 45), Color: color.RGBA{255, 0, 0, 255}, Stroke: 3}}
	if _, err := Render(base, nil, shapes); err != nil {
		t.Fatal(err)
	}
	if n := countChanged(t, base, blank(50, 50)); n != 0 {
		t.Fatalf("base mutated: %d pixels changed", n)
	}
}

func TestRectangleDrawsOutlineNotFill(t *testing.T) {
	base := blank(60, 60)
	r := image.Rect(10, 10, 50, 50)
	shapes := []Shape{Rectangle{Rect: r, Color: color.RGBA{255, 0, 0, 255}, Stroke: 2}}
	out, _ := Render(base, nil, shapes)

	if !anyChangedInRect(out, image.Rect(10, 10, 14, 14)) {
		t.Error("expected border pixels near top-left corner")
	}
	// Center should remain white (outline, not fill).
	if anyChangedInRect(out, image.Rect(28, 28, 32, 32)) {
		t.Error("center should be unchanged for an outline rectangle")
	}
}

func TestArrowChangesPixelsAlongLineAndHead(t *testing.T) {
	base := blank(100, 100)
	shapes := []Shape{Arrow{From: image.Pt(10, 10), To: image.Pt(80, 80), Color: color.RGBA{0, 0, 255, 255}, Stroke: 3}}
	out, _ := Render(base, nil, shapes)

	if !anyChangedInRect(out, image.Rect(40, 40, 50, 50)) {
		t.Error("expected line pixels at midpoint")
	}
	// Arrowhead region near the tip.
	if !anyChangedInRect(out, image.Rect(60, 60, 81, 81)) {
		t.Error("expected arrowhead pixels near tip")
	}
	if n := countChanged(t, out, base); n < 50 {
		t.Errorf("arrow changed too few pixels: %d", n)
	}
}

func TestEllipseDrawsOnPerimeter(t *testing.T) {
	base := blank(80, 80)
	r := image.Rect(10, 10, 70, 50)
	shapes := []Shape{Ellipse{Rect: r, Color: color.RGBA{0, 128, 0, 255}, Stroke: 2}}
	out, _ := Render(base, nil, shapes)

	// Rightmost point of the ellipse: center y=30, x near 70.
	if !anyChangedInRect(out, image.Rect(66, 26, 71, 34)) {
		t.Error("expected ellipse pixels at right extent")
	}
	// Center should be empty.
	if anyChangedInRect(out, image.Rect(38, 28, 42, 32)) {
		t.Error("ellipse center should be unchanged")
	}
}

func TestTextDrawsPixels(t *testing.T) {
	base := blank(200, 60)
	shapes := []Shape{Text{At: image.Pt(10, 10), Text: "Hi", Color: color.RGBA{0, 0, 0, 255}, Stroke: 2}}
	out, _ := Render(base, nil, shapes)
	if n := countChanged(t, out, base); n == 0 {
		t.Error("expected text to change pixels")
	}
	// Text should be in the upper-left area, not the far bottom-right.
	if !anyChangedInRect(out, image.Rect(10, 10, 80, 50)) {
		t.Error("expected text pixels near placement origin")
	}
}

func TestTextEmptyStringNoPanic(t *testing.T) {
	base := blank(40, 40)
	shapes := []Shape{Text{At: image.Pt(5, 5), Text: "", Color: color.RGBA{0, 0, 0, 255}, Stroke: 1}}
	if _, err := Render(base, nil, shapes); err != nil {
		t.Fatal(err)
	}
}

func TestRenderNilShapeSkipped(t *testing.T) {
	base := blank(20, 20)
	out, err := Render(base, nil, []Shape{nil})
	if err != nil {
		t.Fatal(err)
	}
	if n := countChanged(t, out, base); n != 0 {
		t.Fatalf("nil shape should change nothing, changed %d", n)
	}
}
