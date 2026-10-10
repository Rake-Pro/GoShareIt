// Package annotate is the pure-Go image annotation engine for the GoShareIt
// editor. It is deliberately free of any GUI toolkit so it builds and unit
// tests on every platform with CGO disabled. The Gio UI (build-tagged for
// darwin/windows) feeds it a base image, an optional crop rectangle and a list
// of shapes; Render rasterizes them onto a copy and returns the result.
//
// It depends only on the standard image/* packages and golang.org/x/image
// (vector for anti-aliased strokes, opentype and the Go fonts for text).
package annotate

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"

	"golang.org/x/image/font"
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

// Text is a single line of text. With a nil Face it is drawn anti-aliased in
// DefaultFace(Stroke) (Go Regular at 11*Stroke px) and At is the top-left of
// the text box; with a Face, At is the baseline-left origin.
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

// Redact fills each Rect with an opaque Color. It is the safe way to hide
// text; unlike BlurRegion and Pixelate nothing of the source survives.
type Redact struct {
	Rects []image.Rectangle
	Color color.Color
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

func (a Arrow) draw(dst draw.Image) {
	stroke := max(a.Stroke, 1)
	from, tip := center(a.From), center(a.To)
	dx, dy := tip.x-from.x, tip.y-from.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		fillDisc(dst, a.To, float64(stroke)/2, a.Color)
		return
	}
	ux, uy := dx/l, dy/l // unit vector toward the tip
	base, p1, p2 := arrowHead(tip, ux, uy, l, stroke)

	// Shaft and filled head in one anti-aliased pass; the shaft stops at the
	// head's base so it never pokes through the tip.
	var r raster
	if math.Hypot(base.x-from.x, base.y-from.y) > 0 && (base.x-from.x)*ux+(base.y-from.y)*uy > 0 {
		r.addPolyline([]pt{from, base}, float64(stroke))
	}
	r.add([]pt{tip, p1, p2}, true)
	r.draw(dst, a.Color)
}

// ArrowHeadAngle is the angle of each barb off the shaft, in radians.
const ArrowHeadAngle = 0.5

// ArrowHeadLength is the length of the barbs for a stroke and an arrow of
// length l (both in the same units); the editor preview uses it too.
func ArrowHeadLength(stroke, l float64) float64 {
	return math.Max(8, 6*stroke+l*0.12)
}

// arrowHead returns the base midpoint and the two barb tips of the filled
// head at tip, for the unit direction (ux, uy) of an arrow of length l.
func arrowHead(tip pt, ux, uy, l float64, stroke int) (base, p1, p2 pt) {
	head := ArrowHeadLength(float64(stroke), l)
	cosA, sinA := math.Cos(ArrowHeadAngle), math.Sin(ArrowHeadAngle)
	// Rotate the reversed unit vector by +/-ang to get the two barbs.
	rx1, ry1 := -(ux*cosA - uy*sinA), -(ux*sinA + uy*cosA)
	rx2, ry2 := -(ux*cosA + uy*sinA), -(-ux*sinA + uy*cosA)
	p1 = pt{tip.x + rx1*head, tip.y + ry1*head}
	p2 = pt{tip.x + rx2*head, tip.y + ry2*head}
	base = pt{tip.x - ux*head*cosA, tip.y - uy*head*cosA}
	return base, p1, p2
}

func (r Rectangle) draw(dst draw.Image) {
	strokeRect(dst, r.Rect.Canon(), float64(max(r.Stroke, 1)), r.Color)
}

func (e Ellipse) draw(dst draw.Image) {
	rect := e.Rect.Canon()
	if rect.Dx() == 0 || rect.Dy() == 0 {
		return
	}
	strokeEllipse(dst, rect, float64(max(e.Stroke, 1)), e.Color)
}

func (t Text) draw(dst draw.Image) {
	if t.Face != nil {
		d := &font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(t.Color),
			Face: t.Face,
			Dot:  fixed.P(t.At.X, t.At.Y),
		}
		d.DrawString(t.Text)
		return
	}
	// Default face: anti-aliased Go Regular sized by stroke. At is the
	// top-left of the text box (not the baseline) for predictable placement
	// from a GUI click.
	face := DefaultFace(t.Stroke)
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(t.Color),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(t.At.X), Y: fixed.I(t.At.Y) + face.Metrics().Ascent},
	}
	d.DrawString(t.Text)
}

func (l Line) draw(dst draw.Image) {
	strokeSegment(dst, l.From, l.To, float64(max(l.Stroke, 1)), l.Color)
}

func (f Freehand) draw(dst draw.Image) {
	stroke := f.Stroke
	if stroke < 1 {
		stroke = 1
	}
	if len(f.Points) == 0 {
		return
	}
	strokePolyline(dst, f.Points, float64(stroke), f.Color)
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

func (r Redact) draw(dst draw.Image) {
	if r.Color == nil {
		return
	}
	// Un-premultiply, then force full alpha: a redaction is never see-through.
	c := color.NRGBAModel.Convert(r.Color).(color.NRGBA)
	c.A = 0xff
	solid := image.NewUniform(c)
	for _, rc := range r.Rects {
		draw.Draw(dst, rc.Canon().Intersect(dst.Bounds()), solid, image.Point{}, draw.Src)
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
	fillDisc(dst, s.Center, float64(radius), s.Color)
	label := strconv.Itoa(s.Number)
	// The number is about 1.2x the radius tall, centred on the disc.
	face := faceForSize(max(8, radius*6/5))
	m := face.Metrics()
	w := (&font.Drawer{Face: face}).MeasureString(label)
	c := center(s.Center)
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(badgeTextColor(s.Color)),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.Int26_6(c.x*64) - w/2,
			Y: fixed.Int26_6(c.y*64) + (m.Ascent-m.Descent)/2,
		},
	}
	d.DrawString(label)
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
