package annotate

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// pt is a point in continuous image space. Pixel (x, y) covers
// [x, x+1) x [y, y+1), so an integer image point maps to its pixel centre.
type pt struct{ x, y float64 }

func center(p image.Point) pt { return pt{float64(p.X) + 0.5, float64(p.Y) + 0.5} }

// raster collects the polygons of one shape and composites them in a single
// anti-aliased pass, so overlapping parts (segment joints, caps) are covered
// once instead of blended twice. Polygons added with positive=true are
// filled; positive=false ones cut holes (rings).
type raster struct {
	polys [][]pt
	signs []bool
}

func (r *raster) add(p []pt, positive bool) {
	if len(p) >= 3 {
		r.polys = append(r.polys, p)
		r.signs = append(r.signs, positive)
	}
}

// draw rasterizes the collected polygons with golang.org/x/image/vector and
// composites col over dst with anti-aliased coverage.
func (r *raster) draw(dst draw.Image, col color.Color) {
	if len(r.polys) == 0 {
		return
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range r.polys {
		for _, q := range p {
			minX, minY = math.Min(minX, q.x), math.Min(minY, q.y)
			maxX, maxY = math.Max(maxX, q.x), math.Max(maxY, q.y)
		}
	}
	b := image.Rect(int(math.Floor(minX))-1, int(math.Floor(minY))-1, int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1).
		Intersect(dst.Bounds())
	if b.Empty() {
		return
	}
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	ox, oy := float64(b.Min.X), float64(b.Min.Y)
	for i, p := range r.polys {
		// The rasterizer accumulates signed coverage: give every filled
		// polygon the same orientation and every hole the opposite one.
		if (signedArea(p) > 0) != r.signs[i] {
			p = reversed(p)
		}
		z.MoveTo(float32(p[0].x-ox), float32(p[0].y-oy))
		for _, q := range p[1:] {
			z.LineTo(float32(q.x-ox), float32(q.y-oy))
		}
		z.ClosePath()
	}
	z.Draw(dst, b, image.NewUniform(col), image.Point{})
}

func signedArea(p []pt) float64 {
	a := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i].x*p[j].y - p[j].x*p[i].y
	}
	return a / 2
}

func reversed(p []pt) []pt {
	out := make([]pt, len(p))
	for i, q := range p {
		out[len(p)-1-i] = q
	}
	return out
}

// ellipsePoly approximates an ellipse with enough segments that the chord
// error stays well under a pixel.
func ellipsePoly(c pt, rx, ry float64) []pt {
	n := int(math.Min(1024, math.Max(64, (rx+ry)*1.5)))
	out := make([]pt, n)
	for i := range out {
		t := 2 * math.Pi * float64(i) / float64(n)
		out[i] = pt{c.x + rx*math.Cos(t), c.y + ry*math.Sin(t)}
	}
	return out
}

func discPoly(c pt, rad float64) []pt {
	n := int(math.Min(64, math.Max(8, rad*4)))
	out := make([]pt, n)
	for i := range out {
		t := 2 * math.Pi * float64(i) / float64(n)
		out[i] = pt{c.x + rad*math.Cos(t), c.y + rad*math.Sin(t)}
	}
	return out
}

// segmentPoly is the rectangle of half-width hw around a-b (nil when a == b).
func segmentPoly(a, b pt, hw float64) []pt {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return nil
	}
	nx, ny := -dy/l*hw, dx/l*hw
	return []pt{{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny}, {b.x - nx, b.y - ny}, {a.x - nx, a.y - ny}}
}

// addPolyline adds a round-capped, round-joined stroke through pts.
func (r *raster) addPolyline(pts []pt, w float64) {
	hw := math.Max(w, 1) / 2
	for i, p := range pts {
		r.add(discPoly(p, hw), true)
		if i > 0 {
			r.add(segmentPoly(pts[i-1], p, hw), true)
		}
	}
}

// strokeSegment draws a round-capped line of width w from a to b.
func strokeSegment(dst draw.Image, a, b image.Point, w float64, col color.Color) {
	strokePolyline(dst, []image.Point{a, b}, w, col)
}

// strokePolyline draws a connected stroke of width w through pts with round
// joints, so Freehand has no gaps.
func strokePolyline(dst draw.Image, pts []image.Point, w float64, col color.Color) {
	cs := make([]pt, len(pts))
	for i, p := range pts {
		cs[i] = center(p)
	}
	var r raster
	r.addPolyline(cs, w)
	r.draw(dst, col)
}

// strokeRect outlines rect with a stroke of width w centred on its edges and
// mitred corners: the outer ring minus the inner one.
func strokeRect(dst draw.Image, rect image.Rectangle, w float64, col color.Color) {
	hw := math.Max(w, 1) / 2
	a, b := center(rect.Min), center(rect.Max)
	var r raster
	r.add([]pt{{a.x - hw, a.y - hw}, {b.x + hw, a.y - hw}, {b.x + hw, b.y + hw}, {a.x - hw, b.y + hw}}, true)
	if b.x-a.x > 2*hw && b.y-a.y > 2*hw {
		r.add([]pt{{a.x + hw, a.y + hw}, {b.x - hw, a.y + hw}, {b.x - hw, b.y - hw}, {a.x + hw, b.y - hw}}, false)
	}
	r.draw(dst, col)
}

// strokeEllipse outlines the ellipse inscribed in rect with a stroke of
// width w: the outer ring minus the inner one.
func strokeEllipse(dst draw.Image, rect image.Rectangle, w float64, col color.Color) {
	hw := math.Max(w, 1) / 2
	c := pt{float64(rect.Min.X+rect.Max.X)/2 + 0.5, float64(rect.Min.Y+rect.Max.Y)/2 + 0.5}
	rx, ry := float64(rect.Dx())/2, float64(rect.Dy())/2
	var r raster
	r.add(ellipsePoly(c, rx+hw, ry+hw), true)
	if rx > hw && ry > hw {
		r.add(ellipsePoly(c, rx-hw, ry-hw), false)
	}
	r.draw(dst, col)
}

// fillTriangle fills the triangle a, b, c.
func fillTriangle(dst draw.Image, a, b, c pt, col color.Color) {
	var r raster
	r.add([]pt{a, b, c}, true)
	r.draw(dst, col)
}

// fillDisc fills a disc of radius rad centred on pixel c.
func fillDisc(dst draw.Image, c image.Point, rad float64, col color.Color) {
	var r raster
	r.add(discPoly(center(c), rad), true)
	r.draw(dst, col)
}
