package annotate

import (
	"image"
	"math"
)

// Bounds returns the image-space box a shape covers, stroke included. The
// editor draws its selection outline around it.
func Bounds(s Shape) image.Rectangle {
	switch s := s.(type) {
	case Arrow:
		hw := strokeHalf(s.Stroke)
		r := span(s.From, s.To)
		if from, tip := center(s.From), center(s.To); from != tip {
			l := math.Hypot(tip.x-from.x, tip.y-from.y)
			_, p1, p2 := arrowHead(tip, (tip.x-from.x)/l, (tip.y-from.y)/l, l, s.Stroke)
			r = r.Union(boxOf(p1, p2))
		}
		return inflate(r, hw)
	case Line:
		return inflate(span(s.From, s.To), strokeHalf(s.Stroke))
	case Freehand:
		if len(s.Points) == 0 {
			return image.Rectangle{}
		}
		r := span(s.Points[0], s.Points[0])
		for _, p := range s.Points[1:] {
			r = r.Union(span(p, p))
		}
		return inflate(r, strokeHalf(s.Stroke))
	case Rectangle:
		return inflate(s.Rect.Canon(), strokeHalf(s.Stroke))
	case Ellipse:
		return inflate(s.Rect.Canon(), strokeHalf(s.Stroke))
	case Text:
		return image.Rectangle{Min: s.At, Max: s.At.Add(TextSize(s.Text, s.Stroke))}
	case BlurRegion:
		return s.Rect.Canon()
	case Pixelate:
		return s.Rect.Canon()
	case Highlight:
		return s.Rect.Canon()
	case StepBadge:
		rad := s.Radius
		if rad < 1 {
			rad = 14
		}
		return image.Rectangle{Min: s.Center.Sub(image.Pt(rad, rad)), Max: s.Center.Add(image.Pt(rad+1, rad+1))}
	case Redact:
		var r image.Rectangle
		for _, rc := range s.Rects {
			r = r.Union(rc.Canon())
		}
		return r
	}
	return image.Rectangle{}
}

// Hit reports whether the image point p touches s, with tol extra image
// pixels of slack (the editor passes a few screen pixels converted to image
// pixels, so a 1 px line is as easy to pick as a thick one). Strokes
// (arrows, lines, freehand, rectangle and ellipse outlines) are hit near the
// stroke; filled shapes (text, blur, pixelate, highlight, step, redact)
// anywhere inside.
func Hit(s Shape, p image.Point, tol float64) bool {
	q := center(p)
	switch s := s.(type) {
	case Arrow:
		from, tip := center(s.From), center(s.To)
		if distSeg(q, from, tip) <= strokeHalf(s.Stroke)+tol {
			return true
		}
		if from == tip {
			return false
		}
		l := math.Hypot(tip.x-from.x, tip.y-from.y)
		_, p1, p2 := arrowHead(tip, (tip.x-from.x)/l, (tip.y-from.y)/l, l, s.Stroke)
		return inTriangle(q, tip, p1, p2) || distSeg(q, tip, p1) <= tol || distSeg(q, p1, p2) <= tol || distSeg(q, p2, tip) <= tol
	case Line:
		return distSeg(q, center(s.From), center(s.To)) <= strokeHalf(s.Stroke)+tol
	case Freehand:
		lim := strokeHalf(s.Stroke) + tol
		for i, pp := range s.Points {
			a := center(pp)
			b := a
			if i+1 < len(s.Points) {
				b = center(s.Points[i+1])
			}
			if distSeg(q, a, b) <= lim {
				return true
			}
		}
		return false
	case Rectangle:
		r := s.Rect.Canon()
		a, b := center(r.Min), center(r.Max)
		return distRectEdge(q, a, b) <= strokeHalf(s.Stroke)+tol
	case Ellipse:
		r := s.Rect.Canon()
		if r.Dx() == 0 || r.Dy() == 0 {
			return false
		}
		c := pt{float64(r.Min.X+r.Max.X)/2 + 0.5, float64(r.Min.Y+r.Max.Y)/2 + 0.5}
		ring := ellipsePoly(c, float64(r.Dx())/2, float64(r.Dy())/2)
		lim := strokeHalf(s.Stroke) + tol
		for i := range ring {
			if distSeg(q, ring[i], ring[(i+1)%len(ring)]) <= lim {
				return true
			}
		}
		return false
	case StepBadge:
		rad := float64(s.Radius)
		if rad < 1 {
			rad = 14
		}
		c := center(s.Center)
		return math.Hypot(q.x-c.x, q.y-c.y) <= rad+tol
	case Redact:
		for _, rc := range s.Rects {
			if inBox(q, rc.Canon(), tol) {
				return true
			}
		}
		return false
	case Text, BlurRegion, Pixelate, Highlight:
		return inBox(q, Bounds(s), tol)
	}
	return false
}

// HitTest returns the index of the topmost shape (the last one in shapes)
// under p, or -1. nil entries are skipped (the editor keeps its crop there).
// A shape hit on its stroke or fill wins; when there is none, the topmost
// Rectangle or Ellipse whose inside holds p is picked, so an empty frame
// can be grabbed from its middle without hiding smaller shapes inside it.
func HitTest(shapes []Shape, p image.Point, tol float64) int {
	for i := len(shapes) - 1; i >= 0; i-- {
		if shapes[i] != nil && Hit(shapes[i], p, tol) {
			return i
		}
	}
	q := center(p)
	for i := len(shapes) - 1; i >= 0; i-- {
		switch s := shapes[i].(type) {
		case Rectangle:
			if inBox(q, s.Rect.Canon(), 0) {
				return i
			}
		case Ellipse:
			r := s.Rect.Canon()
			if r.Dx() == 0 || r.Dy() == 0 {
				continue
			}
			rx, ry := float64(r.Dx())/2, float64(r.Dy())/2
			cx, cy := float64(r.Min.X+r.Max.X)/2+0.5, float64(r.Min.Y+r.Max.Y)/2+0.5
			if dx, dy := (q.x-cx)/rx, (q.y-cy)/ry; dx*dx+dy*dy <= 1 {
				return i
			}
		}
	}
	return -1
}

// span is the pixel box covering a and b (never empty).
func span(a, b image.Point) image.Rectangle {
	return image.Rect(min(a.X, b.X), min(a.Y, b.Y), max(a.X, b.X)+1, max(a.Y, b.Y)+1)
}

func strokeHalf(stroke int) float64 { return math.Max(float64(stroke), 1) / 2 }

func inflate(r image.Rectangle, hw float64) image.Rectangle {
	n := int(math.Ceil(hw))
	return r.Inset(-n)
}

func boxOf(a, b pt) image.Rectangle {
	return image.Rect(int(math.Floor(math.Min(a.x, b.x))), int(math.Floor(math.Min(a.y, b.y))),
		int(math.Ceil(math.Max(a.x, b.x))), int(math.Ceil(math.Max(a.y, b.y))))
}

// inBox reports whether q lies in the pixels of r, grown by tol.
func inBox(q pt, r image.Rectangle, tol float64) bool {
	return q.x >= float64(r.Min.X)-tol && q.x <= float64(r.Max.X)+tol &&
		q.y >= float64(r.Min.Y)-tol && q.y <= float64(r.Max.Y)+tol
}

// distSeg is the distance from q to the segment a-b.
func distSeg(q, a, b pt) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	l2 := dx*dx + dy*dy
	t := 0.0
	if l2 > 0 {
		t = math.Max(0, math.Min(1, ((q.x-a.x)*dx+(q.y-a.y)*dy)/l2))
	}
	return math.Hypot(q.x-(a.x+t*dx), q.y-(a.y+t*dy))
}

// distRectEdge is the distance from q to the outline of the box a-b (the
// stroke's centre line).
func distRectEdge(q, a, b pt) float64 {
	c1, c2 := pt{b.x, a.y}, pt{a.x, b.y}
	return math.Min(math.Min(distSeg(q, a, c1), distSeg(q, c1, b)), math.Min(distSeg(q, b, c2), distSeg(q, c2, a)))
}

func inTriangle(q, a, b, c pt) bool {
	d1 := (q.x-b.x)*(a.y-b.y) - (a.x-b.x)*(q.y-b.y)
	d2 := (q.x-c.x)*(b.y-c.y) - (b.x-c.x)*(q.y-c.y)
	d3 := (q.x-a.x)*(c.y-a.y) - (c.x-a.x)*(q.y-a.y)
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}
