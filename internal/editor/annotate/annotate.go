// Package annotate is the pure-Go image annotation engine for the GoShareIt
// editor. It is deliberately free of any GUI toolkit so it builds and unit
// tests on every platform with CGO disabled. The Gio UI (build-tagged for
// darwin/windows) feeds it a base image, an optional crop rectangle and a list
// of shapes; Render rasterizes them onto a copy and returns the result.
//
// It depends only on the standard image/* packages and golang.org/x/image.
package annotate

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Shape is a single annotation drawn onto the working image. Coordinates are in
// the pixel space of the (already cropped) base image.
type Shape interface {
	// draw rasterizes the shape onto dst.
	draw(dst draw.Image)
}

// Arrow is a straight line from From to To with a triangular head at To.
type Arrow struct {
	From, To image.Point
	Color    color.Color
	Stroke   int
}

// Rectangle is an axis-aligned rectangle outline.
type Rectangle struct {
	Rect   image.Rectangle
	Color  color.Color
	Stroke int
}

// Ellipse is an axis-aligned ellipse outline inscribed in Rect.
type Ellipse struct {
	Rect   image.Rectangle
	Color  color.Color
	Stroke int
}

// Text is a single line of text whose baseline-left origin is At. Face is
// optional; when nil a built-in bitmap face is used (basicfont.Face7x13). The
// Stroke field scales the built-in face by integer nearest-neighbor so text is
// legible at higher stroke widths.
type Text struct {
	At     image.Point
	Text   string
	Color  color.Color
	Face   font.Face
	Stroke int
}

// Render applies crop (if non-nil) to base then draws shapes onto a mutable
// RGBA copy, returning the annotated image. base is never mutated.
func Render(base image.Image, crop *image.Rectangle, shapes []Shape) (image.Image, error) {
	src := base
	srcBounds := base.Bounds()
	if crop != nil {
		c := crop.Intersect(srcBounds)
		if c.Empty() {
			c = srcBounds
		}
		srcBounds = c
	}

	// Normalize to an RGBA image whose origin is (0,0) so shape coordinates are
	// expressed in cropped-image space.
	dst := image.NewRGBA(image.Rect(0, 0, srcBounds.Dx(), srcBounds.Dy()))
	draw.Draw(dst, dst.Bounds(), src, srcBounds.Min, draw.Src)

	for _, s := range shapes {
		if s == nil {
			continue
		}
		s.draw(dst)
	}
	return dst, nil
}

// Crop returns the sub-image of img bounded by rect (intersected with the image
// bounds), copied into a fresh RGBA with a (0,0) origin. The input is not
// mutated. An empty intersection yields an error-free 0x0 image's bounds; in
// practice callers pass valid rects.
func Crop(img image.Image, rect image.Rectangle) image.Image {
	c := rect.Intersect(img.Bounds())
	dst := image.NewRGBA(image.Rect(0, 0, c.Dx(), c.Dy()))
	if !c.Empty() {
		draw.Draw(dst, dst.Bounds(), img, c.Min, draw.Src)
	}
	return dst
}

// --- rasterization helpers ---

func setPixel(dst draw.Image, x, y int, c color.Color) {
	if image.Pt(x, y).In(dst.Bounds()) {
		dst.Set(x, y, c)
	}
}

// fillDisc stamps a filled square of side ~stroke centered at (x,y). A square
// stamp is adequate for the stroke widths used here and keeps the math simple.
func stamp(dst draw.Image, x, y, stroke int, c color.Color) {
	if stroke < 1 {
		stroke = 1
	}
	r := stroke / 2
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			setPixel(dst, x+dx, y+dy, c)
		}
	}
}

// drawLine draws a stroked line using Bresenham, stamping a square of width
// stroke at each step.
func drawLine(dst draw.Image, a, b image.Point, stroke int, c color.Color) {
	if stroke < 1 {
		stroke = 1
	}
	dx := abs(b.X - a.X)
	dy := -abs(b.Y - a.Y)
	sx := sign(b.X - a.X)
	sy := sign(b.Y - a.Y)
	err := dx + dy
	x, y := a.X, a.Y
	for {
		stamp(dst, x, y, stroke, c)
		if x == b.X && y == b.Y {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x += sx
		}
		if e2 <= dx {
			err += dx
			y += sy
		}
	}
}

func (a Arrow) draw(dst draw.Image) {
	stroke := a.Stroke
	if stroke < 1 {
		stroke = 1
	}
	drawLine(dst, a.From, a.To, stroke, a.Color)

	// Arrowhead: two short lines from the tip, angled back along the shaft.
	dx := float64(a.To.X - a.From.X)
	dy := float64(a.To.Y - a.From.Y)
	length := dx*dx + dy*dy
	if length == 0 {
		return
	}
	l := sqrt(length)
	ux, uy := dx/l, dy/l // unit vector toward tip

	// Head size scales with stroke and overall length.
	head := 6*stroke + int(l*0.12)
	if head < 8 {
		head = 8
	}
	const ang = 0.5 // radians off the shaft
	cosA, sinA := cos(ang), sin(ang)

	// Rotate the reversed unit vector by +/-ang to get the two barbs.
	rx1 := -(ux*cosA - uy*sinA)
	ry1 := -(ux*sinA + uy*cosA)
	rx2 := -(ux*cosA + uy*sinA)
	ry2 := -(-ux*sinA + uy*cosA)

	p1 := image.Pt(a.To.X+int(rx1*float64(head)), a.To.Y+int(ry1*float64(head)))
	p2 := image.Pt(a.To.X+int(rx2*float64(head)), a.To.Y+int(ry2*float64(head)))
	drawLine(dst, a.To, p1, stroke, a.Color)
	drawLine(dst, a.To, p2, stroke, a.Color)
}

func (r Rectangle) draw(dst draw.Image) {
	rect := r.Rect.Canon()
	tl := rect.Min
	tr := image.Pt(rect.Max.X, rect.Min.Y)
	bl := image.Pt(rect.Min.X, rect.Max.Y)
	br := rect.Max
	drawLine(dst, tl, tr, r.Stroke, r.Color)
	drawLine(dst, tr, br, r.Stroke, r.Color)
	drawLine(dst, br, bl, r.Stroke, r.Color)
	drawLine(dst, bl, tl, r.Stroke, r.Color)
}

func (e Ellipse) draw(dst draw.Image) {
	rect := e.Rect.Canon()
	if rect.Dx() == 0 || rect.Dy() == 0 {
		return
	}
	cx := float64(rect.Min.X+rect.Max.X) / 2
	cy := float64(rect.Min.Y+rect.Max.Y) / 2
	rx := float64(rect.Dx()) / 2
	ry := float64(rect.Dy()) / 2
	stroke := e.Stroke
	if stroke < 1 {
		stroke = 1
	}
	// Parametric sampling; step fine enough to avoid gaps at this radius.
	steps := int(4 * (rx + ry))
	if steps < 32 {
		steps = 32
	}
	var prev image.Point
	for i := 0; i <= steps; i++ {
		t := 2 * pi * float64(i) / float64(steps)
		x := int(cx + rx*cos(t))
		y := int(cy + ry*sin(t))
		p := image.Pt(x, y)
		if i > 0 {
			drawLine(dst, prev, p, stroke, e.Color)
		}
		prev = p
	}
}

func (t Text) draw(dst draw.Image) {
	face := t.Face
	scale := t.Stroke
	if scale < 1 {
		scale = 1
	}
	if face == nil {
		// Render into a temporary mask with the bitmap face, then scale up by an
		// integer factor so stroke width controls legibility.
		drawScaledText(dst, t.At, t.Text, t.Color, scale)
		return
	}
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(t.Color),
		Face: face,
		Dot:  fixed.P(t.At.X, t.At.Y),
	}
	d.DrawString(t.Text)
}

// drawScaledText renders text with basicfont.Face7x13 into a small mask and
// nearest-neighbor scales it by factor onto dst. At is the top-left of the text
// box (not the baseline) for predictable placement from a GUI click.
func drawScaledText(dst draw.Image, at image.Point, s string, c color.Color, factor int) {
	face := basicfont.Face7x13
	d := &font.Drawer{Face: face}
	w := d.MeasureString(s).Ceil()
	if w <= 0 {
		w = 1
	}
	h := face.Metrics().Height.Ceil()
	if h <= 0 {
		h = 13
	}
	asc := face.Metrics().Ascent.Ceil()

	mask := image.NewRGBA(image.Rect(0, 0, w, h))
	md := &font.Drawer{
		Dst:  mask,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(0, asc),
	}
	md.DrawString(s)

	if factor == 1 {
		draw.Draw(dst, image.Rect(at.X, at.Y, at.X+w, at.Y+h), mask, image.Point{}, draw.Over)
		return
	}
	scaled := image.NewRGBA(image.Rect(0, 0, w*factor, h*factor))
	xdraw.NearestNeighbor.Scale(scaled, scaled.Bounds(), mask, mask.Bounds(), xdraw.Over, nil)
	draw.Draw(dst, image.Rect(at.X, at.Y, at.X+w*factor, at.Y+h*factor), scaled, image.Point{}, draw.Over)
}

// --- tiny math helpers (avoid pulling math into hot pixel loops via aliases) ---

const pi = math.Pi

func cos(x float64) float64  { return math.Cos(x) }
func sin(x float64) float64  { return math.Sin(x) }
func sqrt(x float64) float64 { return math.Sqrt(x) }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}
