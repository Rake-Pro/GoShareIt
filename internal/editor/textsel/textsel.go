// Package textsel is the pure-Go word layout behind the editor's text
// selection in the Select tool: hit testing, reading-order ranges, the
// selected text and the rectangles a selection covers. It has no GUI
// dependency, so it is unit-tested in the core CI job.
package textsel

import (
	"image"
	"sort"
	"strings"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// nearPx is how far (image pixels) outside a line a press still picks a word
// on it, so a drag that starts in the gap between words or lines works.
const nearPx = 8

// Ref addresses one word: Lines[Line].Words[Word].
type Ref struct{ Line, Word int }

// Less orders refs in reading order.
func (r Ref) Less(o Ref) bool {
	if r.Line != o.Line {
		return r.Line < o.Line
	}
	return r.Word < o.Word
}

// Layout is the recognized words of one image.
type Layout struct{ Lines []ocr.Line }

// New builds a Layout from a recognition result.
func New(r ocr.Result) Layout { return Layout{Lines: r.Lines} }

// Empty reports whether the layout has no words.
func (l Layout) Empty() bool {
	for _, ln := range l.Lines {
		if len(ln.Words) > 0 {
			return false
		}
	}
	return true
}

// HitWord returns the word under p, or the nearest word on the nearest line
// within nearPx image pixels.
func (l Layout) HitWord(p image.Point) (Ref, bool) {
	for li, ln := range l.Lines {
		for wi, w := range ln.Words {
			if p.In(w.Rect) {
				return Ref{li, wi}, true
			}
		}
	}
	best, bestLine := -1, nearPx+1
	for li, ln := range l.Lines {
		if len(ln.Words) == 0 {
			continue
		}
		lr := lineRect(ln)
		if !p.In(lr.Inset(-nearPx)) {
			continue
		}
		if d := axisDist(p.Y, lr.Min.Y, lr.Max.Y); d < bestLine {
			best, bestLine = li, d
		}
	}
	if best < 0 {
		return Ref{}, false
	}
	bw, bd := 0, -1
	for wi, w := range l.Lines[best].Words {
		d := axisDist(p.X, w.Rect.Min.X, w.Rect.Max.X)
		if bd < 0 || d < bd {
			bw, bd = wi, d
		}
	}
	return Ref{best, bw}, true
}

// lineRect is the line rectangle, or the union of its words when the engine
// left it empty.
func lineRect(ln ocr.Line) image.Rectangle {
	if !ln.Rect.Empty() {
		return ln.Rect
	}
	return ocr.UnionRect(ln.Words)
}

// axisDist is the distance from v to the interval [lo, hi) (0 inside).
func axisDist(v, lo, hi int) int {
	switch {
	case v < lo:
		return lo - v
	case v >= hi:
		return v - hi + 1
	}
	return 0
}

// Range returns a..b inclusive in reading order (swapped when b precedes a).
func (l Layout) Range(a, b Ref) []Ref {
	if b.Less(a) {
		a, b = b, a
	}
	var out []Ref
	for li := a.Line; li <= b.Line && li < len(l.Lines); li++ {
		from, to := 0, len(l.Lines[li].Words)-1
		if li == a.Line {
			from = a.Word
		}
		if li == b.Line {
			to = b.Word
		}
		for wi := from; wi <= to; wi++ {
			out = append(out, Ref{li, wi})
		}
	}
	return out
}

// LineRefs returns every word of one line.
func (l Layout) LineRefs(line int) []Ref {
	if line < 0 || line >= len(l.Lines) {
		return nil
	}
	out := make([]Ref, len(l.Lines[line].Words))
	for wi := range out {
		out[wi] = Ref{line, wi}
	}
	return out
}

// All returns every word in reading order.
func (l Layout) All() []Ref {
	var out []Ref
	for li := range l.Lines {
		out = append(out, l.LineRefs(li)...)
	}
	return out
}

// Text joins the words: spaces inside a line (none between CJK words), "\n"
// between lines.
func (l Layout) Text(refs []Ref) string {
	var lines []string
	for _, grp := range l.group(refs) {
		lines = append(lines, ocr.JoinWords(grp))
	}
	return strings.Join(lines, "\n")
}

// Rects returns one rectangle per line segment covered by refs (the union of
// the selected words on that line), used for Redact selection and highlights.
func (l Layout) Rects(refs []Ref) []image.Rectangle {
	var out []image.Rectangle
	for _, grp := range l.group(refs) {
		out = append(out, ocr.UnionRect(grp))
	}
	return out
}

// group sorts refs and splits them into per-line word lists, dropping refs
// that do not exist.
func (l Layout) group(refs []Ref) [][]ocr.Word {
	rs := append([]Ref(nil), refs...)
	sort.Slice(rs, func(i, j int) bool { return rs[i].Less(rs[j]) })
	var out [][]ocr.Word
	cur := -1
	for _, r := range rs {
		if r.Line < 0 || r.Line >= len(l.Lines) || r.Word < 0 || r.Word >= len(l.Lines[r.Line].Words) {
			continue
		}
		if r.Line != cur {
			out = append(out, nil)
			cur = r.Line
		}
		out[len(out)-1] = append(out[len(out)-1], l.Lines[r.Line].Words[r.Word])
	}
	return out
}

// AllText is every word as running text: the engine's own line text where
// the line is whole, the joined words where Mask removed some.
func (l Layout) AllText() string {
	parts := make([]string, 0, len(l.Lines))
	for _, ln := range l.Lines {
		t := ln.Text
		if t == "" {
			t = ocr.JoinWords(ln.Words)
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, "\n")
}

// Mask returns the layout without the words that cover hides: a word goes
// when more than half of its box lies under the union of the cover
// rectangles (the editor's Redact boxes), so redacted text can be neither
// selected nor copied. A line that loses words drops its engine text and
// shrinks to the words left; a line that loses all of them is dropped. With
// no cover the layout is returned as is, so removing the boxes (undo)
// brings the words back.
func (l Layout) Mask(cover []image.Rectangle) Layout {
	if len(cover) == 0 {
		return l
	}
	var out Layout
	for _, ln := range l.Lines {
		var kept []ocr.Word
		for _, w := range ln.Words {
			if !Covered(w.Rect, cover) {
				kept = append(kept, w)
			}
		}
		switch {
		case len(kept) == len(ln.Words):
			out.Lines = append(out.Lines, ln)
		case len(kept) > 0:
			out.Lines = append(out.Lines, ocr.Line{Rect: ocr.UnionRect(kept), Words: kept})
		}
	}
	return out
}

// Covered reports whether more than half of r lies under the union of
// cover. Overlapping cover rectangles are counted once.
func Covered(r image.Rectangle, cover []image.Rectangle) bool {
	area := r.Dx() * r.Dy()
	if area <= 0 {
		return false
	}
	var clipped []image.Rectangle
	xs, ys := []int{r.Min.X, r.Max.X}, []int{r.Min.Y, r.Max.Y}
	for _, c := range cover {
		if c = c.Intersect(r); !c.Empty() {
			clipped = append(clipped, c)
			xs = append(xs, c.Min.X, c.Max.X)
			ys = append(ys, c.Min.Y, c.Max.Y)
		}
	}
	if len(clipped) == 0 {
		return false
	}
	// Union area by coordinate compression: each grid cell between
	// neighbouring edges is either fully inside some cover box or not.
	xs, ys = sortedUnique(xs), sortedUnique(ys)
	under := 0
	for i := 0; i+1 < len(xs); i++ {
		for j := 0; j+1 < len(ys); j++ {
			p := image.Pt(xs[i], ys[j])
			for _, c := range clipped {
				if p.In(c) {
					under += (xs[i+1] - xs[i]) * (ys[j+1] - ys[j])
					break
				}
			}
		}
	}
	// A quarter is enough: a Redact box drawn over the secret half of a
	// token must take the whole word out of selection and copying, and
	// Redact selection and Quick redact always cover whole words anyway.
	return 4*under >= area
}

func sortedUnique(v []int) []int {
	sort.Ints(v)
	out := v[:0]
	for i, x := range v {
		if i == 0 || x != v[i-1] {
			out = append(out, x)
		}
	}
	return out
}
