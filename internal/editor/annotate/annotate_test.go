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

// halfSplit returns a w x h image with the left half black and the right half
// white, producing a sharp vertical edge at x = w/2.
func halfSplit(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	return img
}

// rowVariance measures how non-uniform a single row is across [x0,x1).
func rowVariance(img image.Image, y, x0, x1 int) float64 {
	var sum, sumSq, n float64
	for x := x0; x < x1; x++ {
		r, _, _, _ := img.At(x, y).RGBA()
		v := float64(r >> 8)
		sum += v
		sumSq += v * v
		n++
	}
	if n == 0 {
		return 0
	}
	mean := sum / n
	return sumSq/n - mean*mean
}

func TestBlurSmoothsSharpEdge(t *testing.T) {
	base := halfSplit(60, 20)
	// Variance of the raw edge row across the transition band.
	before := rowVariance(base, 10, 20, 40)
	shapes := []Shape{BlurRegion{Rect: image.Rect(10, 5, 50, 15), Radius: 4}}
	out, _ := Render(base, nil, shapes)
	after := rowVariance(out, 10, 20, 40)
	if after >= before {
		t.Fatalf("blur did not reduce edge variance: before=%.1f after=%.1f", before, after)
	}
	// Intermediate gray must appear right at the former hard edge.
	r, g, b, _ := out.At(30, 10).RGBA()
	if r>>8 == 0 || r>>8 == 0xff {
		t.Errorf("expected blended gray at edge, got r=%d g=%d b=%d", r>>8, g>>8, b>>8)
	}
}

func TestPixelateBlocksAreUniform(t *testing.T) {
	base := halfSplit(40, 40)
	block := 8
	shapes := []Shape{Pixelate{Rect: image.Rect(0, 0, 40, 40), Block: block}}
	out, _ := Render(base, nil, shapes)
	// Every pixel in a block must equal the block's top-left pixel.
	for by := 0; by < 40; by += block {
		for bx := 0; bx < 40; bx += block {
			wr, wg, wb, _ := out.At(bx, by).RGBA()
			for y := by; y < by+block && y < 40; y++ {
				for x := bx; x < bx+block && x < 40; x++ {
					r, g, b, _ := out.At(x, y).RGBA()
					if r != wr || g != wg || b != wb {
						t.Fatalf("block at %d,%d not uniform at %d,%d", bx, by, x, y)
					}
				}
			}
		}
	}
}

func TestHighlightTintsButKeepsDetail(t *testing.T) {
	base := blank(40, 40) // white
	shapes := []Shape{Highlight{Rect: image.Rect(5, 5, 35, 35), Color: color.RGBA{0xff, 0xd6, 0x0a, 0xff}, Alpha: 0x60}}
	out, _ := Render(base, nil, shapes)
	r, g, b, _ := out.At(20, 20).RGBA()
	r8, g8, b8 := r>>8, g>>8, b>>8
	// Tint toward yellow: blue channel drops below the others.
	if !(b8 < r8 && b8 < g8) {
		t.Errorf("expected yellow tint (b lowest), got r=%d g=%d b=%d", r8, g8, b8)
	}
	// Underlying white still shows through: not fully saturated to the color.
	if b8 == 0x0a {
		t.Errorf("overlay fully replaced pixel; detail lost (b=%d)", b8)
	}
	// Outside the region remains untouched white.
	if anyChangedInRect(out, image.Rect(0, 0, 4, 4)) {
		t.Error("highlight leaked outside its rect")
	}
}

func TestStepBadgeDrawsDiscAndDigit(t *testing.T) {
	base := blank(60, 60)
	shapes := []Shape{StepBadge{Center: image.Pt(30, 30), Number: 1, Color: color.RGBA{0xff, 0x3b, 0x30, 0xff}, Radius: 14}}
	out, _ := Render(base, nil, shapes)
	// Disc fills its center.
	if !anyChangedInRect(out, image.Rect(28, 28, 32, 32)) {
		t.Error("expected filled disc at badge center")
	}
	// Outside the radius stays white.
	if anyChangedInRect(out, image.Rect(0, 0, 4, 4)) {
		t.Error("badge drew outside its disc")
	}
	// A contrasting digit pixel (white on red) exists somewhere in the center band.
	foundDigit := false
	for y := 22; y < 38 && !foundDigit; y++ {
		for x := 22; x < 38; x++ {
			r, g, b, _ := out.At(x, y).RGBA()
			if r>>8 > 0xf0 && g>>8 > 0xf0 && b>>8 > 0xf0 {
				foundDigit = true
				break
			}
		}
	}
	if !foundDigit {
		t.Error("expected white digit pixels inside the disc")
	}
}

func TestLineDrawsNoHead(t *testing.T) {
	base := blank(100, 100)
	shapes := []Shape{Line{From: image.Pt(10, 50), To: image.Pt(90, 50), Color: color.RGBA{0, 0, 255, 255}, Stroke: 3}}
	out, _ := Render(base, nil, shapes)
	if !anyChangedInRect(out, image.Rect(45, 48, 55, 53)) {
		t.Error("expected line pixels at midpoint")
	}
	// No arrowhead: rows well above/below the shaft near the tip stay white.
	if anyChangedInRect(out, image.Rect(80, 60, 95, 75)) {
		t.Error("line should not draw an arrowhead")
	}
}

func TestFreehandDrawsPath(t *testing.T) {
	base := blank(100, 100)
	pts := []image.Point{{10, 10}, {30, 40}, {60, 20}, {80, 70}}
	shapes := []Shape{Freehand{Points: pts, Color: color.RGBA{0, 128, 0, 255}, Stroke: 2}}
	out, _ := Render(base, nil, shapes)
	for i, p := range pts {
		if !anyChangedInRect(out, image.Rect(p.X-2, p.Y-2, p.X+3, p.Y+3)) {
			t.Errorf("expected freehand pixels near vertex %d %v", i, p)
		}
	}
	if !anyChangedInRect(out, image.Rect(18, 23, 23, 28)) {
		t.Error("expected freehand pixels along first segment")
	}
}

func TestFreehandSinglePointStamps(t *testing.T) {
	base := blank(20, 20)
	shapes := []Shape{Freehand{Points: []image.Point{{10, 10}}, Color: color.RGBA{0, 0, 0, 255}, Stroke: 3}}
	out, _ := Render(base, nil, shapes)
	if !anyChangedInRect(out, image.Rect(9, 9, 12, 12)) {
		t.Error("expected a stamp at the single point")
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

// Blur must be redaction-grade: fine detail (text-scale high frequency)
// inside the region must be destroyed, not softened. 1px alternating
// stripes are the worst case - after blurring, per-row contrast must
// collapse to near-uniform.
func TestBlurRedactsFineDetail(t *testing.T) {
	w, h := 240, 80
	base := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{0, 0, 0, 255}
			if x%2 == 0 {
				c = color.RGBA{255, 255, 255, 255}
			}
			base.Set(x, y, c)
		}
	}
	before := rowVariance(base, h/2, 20, w-20)
	shapes := []Shape{BlurRegion{Rect: image.Rect(0, 0, w, h), Radius: 3}}
	out, _ := Render(base, nil, shapes)
	after := rowVariance(out, h/2, 20, w-20)
	if after > before/100 {
		t.Fatalf("blur left legible detail: variance before=%.1f after=%.1f (want <1%%)", before, after)
	}
}
