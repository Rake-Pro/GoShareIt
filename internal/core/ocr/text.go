package ocr

import (
	"image"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Text joins Lines with "\n". A line without engine text is joined from its
// words.
func (r Result) Text() string {
	parts := make([]string, 0, len(r.Lines))
	for _, l := range r.Lines {
		t := l.Text
		if t == "" {
			t = JoinWords(l.Words)
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, "\n")
}

// WordCount returns the number of words in all lines.
func (r Result) WordCount() int {
	n := 0
	for _, l := range r.Lines {
		n += len(l.Words)
	}
	return n
}

// JoinWords joins word texts with single spaces, except between two words
// that meet in CJK script (Han, Hiragana, Katakana), which are written
// without spaces.
func JoinWords(words []Word) string {
	var b strings.Builder
	for i, w := range words {
		if i > 0 && NeedsSpace(words[i-1].Text, w.Text) {
			b.WriteByte(' ')
		}
		b.WriteString(w.Text)
	}
	return b.String()
}

// NeedsSpace reports whether a space separates word a from the following
// word b when they are joined into running text.
func NeedsSpace(a, b string) bool {
	last, _ := utf8.DecodeLastRuneInString(a)
	first, _ := utf8.DecodeRuneInString(b)
	return !(isCJK(last) && isCJK(first))
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}

// UnionRect returns the smallest rectangle covering every word, the line
// rectangle for engines that report only word boxes.
func UnionRect(words []Word) image.Rectangle {
	var r image.Rectangle
	for i, w := range words {
		if i == 0 {
			r = w.Rect
			continue
		}
		r = r.Union(w.Rect)
	}
	return r
}
