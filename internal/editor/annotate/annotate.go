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
	"strconv"

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

// Line is a straight stroked line from From to To with no arrowhead.
type Line struct {
	From, To image.Point
	Color    color.Color
	Stroke   int
}

// Freehand is a connected polyline through Points, drawn as one stroke.
type Freehand struct {
	Points []image.Point
	Color  color.Color
	Stroke int
}

// BlurRegion box-blurs the pixels under Rect in place (P4b obscure tool). Radius
// is the box-kernel radius; three separable passes approximate a Gaussian. The
// effect reads whatever has been drawn into dst beneath it, so add it after the
// base is composited (Render does this) and before shapes that should sit on top.
type BlurRegion struct {
	Rect   image.Rectangle
	Radius int
}

// Pixelate replaces Rect with a mosaic of Block-sized cells, each the average
// color of the pixels it covers (P4b obscure tool).
type Pixelate struct {
	Rect  image.Rectangle
	Block int
}

// Highlight composites a translucent Color over Rect (P4b callout tool). Alpha
// is the overlay opacity (0 -> a sensible default); underlying detail remains
// partly visible.
type Highlight struct {
	Rect  image.Rectangle
	Color color.Color
	Alpha uint8
}

// StepBadge is an auto-incrementing numbered callout: a filled disc of Color
// centered at Center with Number drawn in a contrasting color at its middle
// (P4b callout tool). Radius is the disc radius (0 -> default).
type StepBadge struct {
	Center image.Point
	Number int
	Color  color.Color
	Radius int
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

func (l Line) draw(dst draw.Image) {
	stroke := l.Stroke
	if stroke < 1 {
		stroke = 1
	}
	drawLine(dst, l.From, l.To, stroke, l.Color)
}

func (f Freehand) draw(dst draw.Image) {
	stroke := f.Stroke
	if stroke < 1 {
		stroke = 1
	}
	if len(f.Points) == 0 {
		return
	}
	if len(f.Points) == 1 {
		stamp(dst, f.Points[0].X, f.Points[0].Y, stroke, f.Color)
		return
	}
	for i := 1; i < len(f.Points); i++ {
		drawLine(dst, f.Points[i-1], f.Points[i], stroke, f.Color)
	}
}

func (b BlurRegion) draw(dst draw.Image) {
	rect := b.Rect.Canon().Intersect(dst.Bounds())
	if rect.Empty() {
		return
	}
	// Blur is a redaction tool: the output must destroy legible text, not
	// soften it. Radius acts as a strength multiplier (x4), with a floor
	// scaled to the region size so screenshot-scale text (Retina 2x glyphs)
	// is unreadable even at the smallest stroke setting.
	radius := b.Radius
	if radius < 1 {
		radius = 4
	}
	radius *= 4
	minR := rect.Dx()
	if rect.Dy() < minR {
		minR = rect.Dy()
	}
	minR /= 4
	if minR > 48 {
		minR = 48
	}
	if minR < 12 {
		minR = 12
	}
	if radius < minR {
		radius = minR
	}
	region := subImageRGBA(dst, rect)
	for pass := 0; pass < 3; pass++ {
		boxBlurH(region, radius)
		boxBlurV(region, radius)
	}
	draw.Draw(dst, rect, region, image.Point{}, draw.Src)
}

func (p Pixelate) draw(dst draw.Image) {
	rect := p.Rect.Canon().Intersect(dst.Bounds())
	if rect.Empty() {
		return
	}
	block := p.Block
	if block < 2 {
		block = 8
	}
	for by := rect.Min.Y; by < rect.Max.Y; by += block {
		for bx := rect.Min.X; bx < rect.Max.X; bx += block {
			x1 := min(bx+block, rect.Max.X)
			y1 := min(by+block, rect.Max.Y)
			var rs, gs, bs, as, n uint64
			for y := by; y < y1; y++ {
				for x := bx; x < x1; x++ {
					cr, cg, cb, ca := dst.At(x, y).RGBA()
					rs += uint64(cr)
					gs += uint64(cg)
					bs += uint64(cb)
					as += uint64(ca)
					n++
				}
			}
			if n == 0 {
				continue
			}
			avg := color.RGBA64{
				R: uint16(rs / n), G: uint16(gs / n), B: uint16(bs / n), A: uint16(as / n),
			}
			for y := by; y < y1; y++ {
				for x := bx; x < x1; x++ {
					dst.Set(x, y, avg)
				}
			}
		}
	}
}

func (h Highlight) draw(dst draw.Image) {
	rect := h.Rect.Canon().Intersect(dst.Bounds())
	if rect.Empty() {
		return
	}
	a := h.Alpha
	if a == 0 {
		a = 0x60
	}
	r, g, b, _ := h.Color.RGBA()
	overlay := image.NewUniform(color.NRGBA{
		R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: a,
	})
	draw.Draw(dst, rect, overlay, image.Point{}, draw.Over)
}

func (s StepBadge) draw(dst draw.Image) {
	radius := s.Radius
	if radius < 1 {
		radius = 14
	}
	cx, cy := s.Center.X, s.Center.Y
	rr := radius * radius
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= rr {
				setPixel(dst, cx+dx, cy+dy, s.Color)
			}
		}
	}
	label := strconv.Itoa(s.Number)
	face := basicfont.Face7x13
	d := &font.Drawer{Face: face}
	w := d.MeasureString(label).Ceil()
	hgt := face.Metrics().Height.Ceil()
	scale := radius / 7
	if scale < 1 {
		scale = 1
	}
	at := image.Pt(cx-(w*scale)/2, cy-(hgt*scale)/2)
	drawScaledText(dst, at, label, badgeTextColor(s.Color), scale)
}

// badgeTextColor returns black or white, whichever contrasts the disc color.
func badgeTextColor(c color.Color) color.Color {
	r, g, b, _ := c.RGBA()
	lum := (299*uint64(r) + 587*uint64(g) + 114*uint64(b)) / 1000
	if lum > 0x7fff {
		return color.RGBA{0, 0, 0, 0xff}
	}
	return color.RGBA{0xff, 0xff, 0xff, 0xff}
}

// subImageRGBA copies the rect-bounded region of dst into a fresh (0,0)-origin
// RGBA so the blur passes can read/write it without aliasing dst.
func subImageRGBA(dst draw.Image, rect image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(out, out.Bounds(), dst, rect.Min, draw.Src)
	return out
}

// boxBlurH replaces each pixel with the average of its horizontal neighbors
// within radius, clamping at the edges.
func boxBlurH(img *image.RGBA, radius int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	win := 2*radius + 1
	src := make([]uint8, len(img.Pix))
	copy(src, img.Pix)
	for y := 0; y < h; y++ {
		row := y * img.Stride
		for x := 0; x < w; x++ {
			var rs, gs, bs, as int
			for k := -radius; k <= radius; k++ {
				xx := clampi(x+k, 0, w-1)
				o := row + xx*4
				rs += int(src[o])
				gs += int(src[o+1])
				bs += int(src[o+2])
				as += int(src[o+3])
			}
			o := row + x*4
			img.Pix[o] = uint8(rs / win)
			img.Pix[o+1] = uint8(gs / win)
			img.Pix[o+2] = uint8(bs / win)
			img.Pix[o+3] = uint8(as / win)
		}
	}
}

// boxBlurV is the vertical counterpart of boxBlurH.
func boxBlurV(img *image.RGBA, radius int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	win := 2*radius + 1
	src := make([]uint8, len(img.Pix))
	copy(src, img.Pix)
	for x := 0; x < w; x++ {
		col := x * 4
		for y := 0; y < h; y++ {
			var rs, gs, bs, as int
			for k := -radius; k <= radius; k++ {
				yy := clampi(y+k, 0, h-1)
				o := yy*img.Stride + col
				rs += int(src[o])
				gs += int(src[o+1])
				bs += int(src[o+2])
				as += int(src[o+3])
			}
			o := y*img.Stride + col
			img.Pix[o] = uint8(rs / win)
			img.Pix[o+1] = uint8(gs / win)
			img.Pix[o+2] = uint8(bs / win)
			img.Pix[o+3] = uint8(as / win)
		}
	}
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
