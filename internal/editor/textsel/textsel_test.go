package textsel

import (
	"image"
	"reflect"
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/ocrtest"
)

func sample() Layout {
	return New(ocrtest.Lines(
		"The@10,10,30,12 quick@50,10,50,12 fox@110,10,30,12",
		"jumps@10,40,50,12 over@70,40,40,12",
	))
}

func TestHitWordExactAndNear(t *testing.T) {
	l := sample()
	if r, ok := l.HitWord(image.Pt(60, 15)); !ok || r != (Ref{0, 1}) {
		t.Fatalf("exact hit = %v %v", r, ok)
	}
	// In the gap between "quick" and "fox": nearest word on the line.
	if r, ok := l.HitWord(image.Pt(104, 15)); !ok || r != (Ref{0, 1}) {
		t.Fatalf("gap hit = %v %v", r, ok)
	}
	// Just above the second line, within the near band.
	if r, ok := l.HitWord(image.Pt(75, 35)); !ok || r != (Ref{1, 1}) {
		t.Fatalf("near-line hit = %v %v", r, ok)
	}
	if _, ok := l.HitWord(image.Pt(300, 300)); ok {
		t.Fatal("far point should miss")
	}
}

func TestRangeForwardAndBackward(t *testing.T) {
	l := sample()
	want := []Ref{{0, 2}, {1, 0}}
	if got := l.Range(Ref{0, 2}, Ref{1, 0}); !reflect.DeepEqual(got, want) {
		t.Fatalf("forward = %v", got)
	}
	if got := l.Range(Ref{1, 0}, Ref{0, 2}); !reflect.DeepEqual(got, want) {
		t.Fatalf("backward = %v", got)
	}
	if got := len(l.All()); got != 5 {
		t.Fatalf("All = %d words", got)
	}
}

func TestTextAndRects(t *testing.T) {
	l := sample()
	refs := l.Range(Ref{0, 1}, Ref{1, 0})
	if got := l.Text(refs); got != "quick fox\njumps" {
		t.Fatalf("Text = %q", got)
	}
	rects := l.Rects(refs)
	want := []image.Rectangle{image.Rect(50, 10, 140, 22), image.Rect(10, 40, 60, 52)}
	if !reflect.DeepEqual(rects, want) {
		t.Fatalf("Rects = %v, want %v", rects, want)
	}
	if got := l.Text(l.LineRefs(1)); got != "jumps over" {
		t.Fatalf("line text = %q", got)
	}
	if l.Text([]Ref{{5, 5}}) != "" {
		t.Fatal("invalid ref should be dropped")
	}
	if !New(ocrtest.Lines()).Empty() || l.Empty() {
		t.Fatal("Empty")
	}
}

func TestCovered(t *testing.T) {
	word := image.Rect(0, 0, 10, 10)
	for _, c := range []struct {
		cover []image.Rectangle
		want  bool
	}{
		{nil, false},
		{[]image.Rectangle{image.Rect(0, 0, 10, 5)}, true},                          // half: dropped
		{[]image.Rectangle{image.Rect(0, 0, 10, 2)}, false},                         // a fifth: kept
		{[]image.Rectangle{image.Rect(0, 0, 5, 5)}, true},                           // exactly a quarter: dropped
		{[]image.Rectangle{image.Rect(0, 0, 10, 6)}, true},                          // 60%
		{[]image.Rectangle{image.Rect(-5, -5, 20, 20)}, true},                       // fully
		{[]image.Rectangle{image.Rect(0, 0, 6, 10), image.Rect(4, 0, 10, 5)}, true}, // union 60+20
		// Overlapping boxes are counted once: 20% + the same 20% is still 20%
		// (double counting would reach the quarter and drop the word).
		{[]image.Rectangle{image.Rect(0, 0, 2, 10), image.Rect(0, 0, 2, 10)}, false},
		{[]image.Rectangle{image.Rect(20, 20, 30, 30)}, false},
	} {
		if got := Covered(word, c.cover); got != c.want {
			t.Errorf("Covered(%v) = %v, want %v", c.cover, got, c.want)
		}
	}
}

func TestMaskDropsRedactedWords(t *testing.T) {
	l := sample()
	full := l.AllText()
	if full != "The quick fox\njumps over" {
		t.Fatalf("AllText = %q", full)
	}
	// A Redact box over "quick" (and a sliver of "fox") hides "quick" only.
	redact := []image.Rectangle{image.Rect(48, 8, 112, 24)}
	m := l.Mask(redact)
	if got := m.AllText(); got != "The fox\njumps over" {
		t.Fatalf("masked AllText = %q", got)
	}
	if got := m.Text(m.All()); got != "The fox\njumps over" {
		t.Fatalf("masked selection text = %q", got)
	}
	// The covered word can no longer be hit or selected.
	if r, ok := m.HitWord(image.Pt(60, 15)); ok && m.Text([]Ref{r}) == "quick" {
		t.Fatalf("hit the redacted word: %v", r)
	}
	// A line whose words are all covered disappears.
	if got := l.Mask([]image.Rectangle{image.Rect(0, 38, 200, 60)}).AllText(); got != "The quick fox" {
		t.Fatalf("line masked AllText = %q", got)
	}
	// Undo removes the box: the text comes back unchanged.
	if got := l.Mask(nil).AllText(); got != full {
		t.Fatalf("after undo = %q", got)
	}
}
