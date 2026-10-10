package ocr

import (
	"image"
	"regexp"
	"strings"
)

// Kind names one class of sensitive text the Quick redact button can hide.
type Kind string

const (
	KindEmail Kind = "email"
	KindPhone Kind = "phone"
	KindToken Kind = "token" // 20+ char runs of [A-Za-z0-9_-] with letters and digits, e.g. API keys
	KindURL   Kind = "url"
	KindIP    Kind = "ip"
)

// AllKinds lists every Kind in display order.
var AllKinds = []Kind{KindEmail, KindPhone, KindToken, KindURL, KindIP}

// DefaultKinds is what Quick redact hides when nothing is configured
// (Snipping Tool parity).
var DefaultKinds = []Kind{KindEmail, KindPhone}

// ParseKind returns the Kind named s and whether it is known.
func ParseKind(s string) (Kind, bool) {
	k := Kind(strings.ToLower(strings.TrimSpace(s)))
	for _, v := range AllKinds {
		if v == k {
			return k, true
		}
	}
	return "", false
}

// Hit is one match: the covering rectangle(s) in image pixels (one per word
// the match spans) plus the matched text, for the Quick redact button.
type Hit struct {
	Kind  Kind
	Text  string
	Rects []image.Rectangle
}

var (
	reEmail = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)
	rePhone = regexp.MustCompile(`\+?\(?\d[\d ().-]{5,}\d`)
	reToken = regexp.MustCompile(`[A-Za-z0-9_-]{20,}`)
	reURL   = regexp.MustCompile(`https?://\S+`)
	reIP    = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b`)

	// Phone false positives: ISO and dotted dates, and IPs / version numbers.
	reDate    = regexp.MustCompile(`^(?:\d{4}[-./]\d{1,2}[-./]\d{1,2}|\d{1,2}[-./]\d{1,2}[-./]\d{2,4})$`)
	reDotted1 = regexp.MustCompile(`(?:^|\.)\d(?:\.|$)`) // a one-digit dot group: 10.0.19045
)

// FindSensitive scans each line's joined text with one regexp per kind and
// maps character offsets back to the rectangles of the words the match
// overlaps (a partly matched word is covered whole). Unknown kinds are
// ignored. The matchers are deliberately simple; callers must not claim
// completeness.
func FindSensitive(r Result, kinds []Kind) []Hit {
	var hits []Hit
	for _, l := range r.Lines {
		text, spans := joinWithSpans(l.Words)
		for _, k := range kinds {
			for _, m := range matches(k, text) {
				h := Hit{Kind: k, Text: text[m[0]:m[1]]}
				for i, sp := range spans {
					if sp[0] < m[1] && m[0] < sp[1] {
						h.Rects = append(h.Rects, l.Words[i].Rect)
					}
				}
				if len(h.Rects) > 0 {
					hits = append(hits, h)
				}
			}
		}
	}
	return hits
}

// joinWithSpans joins words like JoinWords and returns each word's byte
// range in the joined text.
func joinWithSpans(words []Word) (string, [][2]int) {
	var b strings.Builder
	spans := make([][2]int, len(words))
	for i, w := range words {
		if i > 0 && NeedsSpace(words[i-1].Text, w.Text) {
			b.WriteByte(' ')
		}
		start := b.Len()
		b.WriteString(w.Text)
		spans[i] = [2]int{start, b.Len()}
	}
	return b.String(), spans
}

func matches(k Kind, text string) [][]int {
	switch k {
	case KindEmail:
		return reEmail.FindAllStringIndex(text, -1)
	case KindURL:
		return reURL.FindAllStringIndex(text, -1)
	case KindIP:
		return reIP.FindAllStringIndex(text, -1)
	case KindToken:
		var out [][]int
		for _, m := range reToken.FindAllStringIndex(text, -1) {
			s := text[m[0]:m[1]]
			if strings.ContainsAny(s, "0123456789") && strings.IndexFunc(s, isASCIILetter) >= 0 {
				out = append(out, m)
			}
		}
		return out
	case KindPhone:
		var out [][]int
		for _, m := range rePhone.FindAllStringIndex(text, -1) {
			out = append(out, phoneMatches(text, m[0], m[1])...)
		}
		return out
	}
	return nil
}

// phoneMatches checks one rePhone candidate text[start:end]. The greedy
// pattern can join numbers separated only by spaces ("555-123-4567
// 555-987-6543"); a candidate with too many digits is split at its spaces
// and the runs of parts are tried again, longest first from the left, so
// each number is still found.
func phoneMatches(text string, start, end int) [][]int {
	cand := text[start:end]
	if isPhone(cand) {
		return [][]int{{start, end}}
	}
	if digitCount(cand) <= 15 || !strings.Contains(cand, " ") {
		return nil
	}
	// Byte offsets of the space-separated parts.
	var parts [][2]int
	for i := 0; i < len(cand); {
		for i < len(cand) && cand[i] == ' ' {
			i++
		}
		j := i
		for j < len(cand) && cand[j] != ' ' {
			j++
		}
		if j > i {
			parts = append(parts, [2]int{i, j})
		}
		i = j
	}
	var out [][]int
	for i := 0; i < len(parts); {
		found := false
		for j := len(parts) - 1; j >= i; j-- {
			s, e := parts[i][0], parts[j][1]
			if isPhone(cand[s:e]) {
				out = append(out, []int{start + s, start + e})
				i, found = j+1, true
				break
			}
		}
		if !found {
			i++
		}
	}
	return out
}

func digitCount(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n++
		}
	}
	return n
}

func isASCIILetter(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }

// isPhone filters rePhone candidates: 7 to 15 digits, and not a date, an IP
// address or a dotted version number.
func isPhone(s string) bool {
	if digits := digitCount(s); digits < 7 || digits > 15 {
		return false
	}
	t := strings.TrimSpace(s)
	if reDate.MatchString(t) {
		return false
	}
	// Only digits and dots: an IP address or a version number unless every
	// group has at least two digits and there are at most three groups
	// (555.123.4567).
	if strings.Trim(t, "0123456789.") == "" && (strings.Count(t, ".") > 2 || reDotted1.MatchString(t)) {
		return false
	}
	return true
}
