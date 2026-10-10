package ocr_test

import (
	"context"
	"errors"
	"image"
	"reflect"
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/ocrtest"
)

func TestResultTextJoinsLines(t *testing.T) {
	r := ocrtest.Lines("Hello@0,0,40,10 world@45,0,40,10", "second@0,20,50,10")
	if got := r.Text(); got != "Hello world\nsecond" {
		t.Fatalf("Text = %q", got)
	}
	// A line without engine text is joined from its words.
	r.Lines[0].Text = ""
	if got := r.Text(); got != "Hello world\nsecond" {
		t.Fatalf("Text (words) = %q", got)
	}
	if r.WordCount() != 3 {
		t.Fatalf("WordCount = %d", r.WordCount())
	}
}

func TestJoinWordsCJK(t *testing.T) {
	words := []ocr.Word{{Text: "\u65e5\u672c"}, {Text: "\u8a9e"}, {Text: "OK"}, {Text: "\u30c6\u30b9\u30c8"}}
	if got := ocr.JoinWords(words); got != "\u65e5\u672c\u8a9e OK \u30c6\u30b9\u30c8" {
		t.Fatalf("JoinWords = %q", got)
	}
	ko := []ocr.Word{{Text: "\uc548\ub155"}, {Text: "\ud558\uc138\uc694"}}
	if got := ocr.JoinWords(ko); got != "\uc548\ub155 \ud558\uc138\uc694" {
		t.Fatalf("Hangul keeps spaces, got %q", got)
	}
}

func TestUnavailable(t *testing.T) {
	u := ocr.Unavailable{Why: "No engine.", Hint: "Install one."}
	st := u.Probe(context.Background())
	if st.Available || st.Explain() != "No engine. Install one." {
		t.Fatalf("status = %+v", st)
	}
	if _, err := u.Recognize(context.Background(), image.NewRGBA(image.Rect(0, 0, 1, 1)), ocr.Options{}); !errors.Is(err, ocr.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestFindSensitive(t *testing.T) {
	type tc struct {
		name  string
		line  string
		kinds []ocr.Kind
		want  []string // matched texts in order
	}
	cases := []tc{
		{"email", "Mail@0,0,30,10 john.doe+x@example.co.uk@35,0,100,10 now@140,0,20,10", []ocr.Kind{ocr.KindEmail}, []string{"john.doe+x@example.co.uk"}},
		{"us phone across words", "Call@0,0,20,10 (555)@25,0,30,10 123-4567@60,0,40,10", []ocr.Kind{ocr.KindPhone}, []string{"(555) 123-4567"}},
		{"e164", "+49@0,0,20,10 30@25,0,10,10 1234567@40,0,50,10", []ocr.Kind{ocr.KindPhone}, []string{"+49 30 1234567"}},
		{"dotted phone", "555.123.4567@0,0,60,10", []ocr.Kind{ocr.KindPhone}, []string{"555.123.4567"}},
		{"iso date is not a phone", "on@0,0,10,10 2026-10-10@15,0,60,10", []ocr.Kind{ocr.KindPhone}, nil},
		{"dotted date is not a phone", "10.10.2026@0,0,60,10", []ocr.Kind{ocr.KindPhone}, nil},
		{"version is not a phone", "Windows@0,0,40,10 10.0.19045.1234@45,0,80,10", []ocr.Kind{ocr.KindPhone}, nil},
		{"ip is not a phone", "192.168.100.200@0,0,80,10", []ocr.Kind{ocr.KindPhone}, nil},
		{"short number", "port@0,0,20,10 8080@25,0,20,10", []ocr.Kind{ocr.KindPhone}, nil},
		{"adjacent phones", "555-123-4567@0,0,60,10 555-987-6543@65,0,60,10", []ocr.Kind{ocr.KindPhone}, []string{"555-123-4567", "555-987-6543"}},
		{"adjacent e164", "+1@0,0,10,10 555@15,0,20,10 123@40,0,20,10 4567@65,0,25,10 +44@95,0,20,10 20@120,0,10,10 79460000@135,0,50,10", []ocr.Kind{ocr.KindPhone}, []string{"+1 555 123 4567", "+44 20 79460000"}},
		{"ip", "host@0,0,20,10 10.20.30.40@25,0,60,10", []ocr.Kind{ocr.KindIP}, []string{"10.20.30.40"}},
		{"bad ip", "999.1.1.1@0,0,40,10", []ocr.Kind{ocr.KindIP}, nil},
		{"url", "see@0,0,20,10 https://example.com/a?b=c@25,0,90,10", []ocr.Kind{ocr.KindURL}, []string{"https://example.com/a?b=c"}},
		{"token", "key@0,0,20,10 api_key_Q7w2Zx9pLm4Vt8Rk1Ns6@25,0,150,10", []ocr.Kind{ocr.KindToken}, []string{"api_key_Q7w2Zx9pLm4Vt8Rk1Ns6"}},
		{"long word is not a token", "internationalizations@0,0,100,10", []ocr.Kind{ocr.KindToken}, nil},
		{"kind not requested", "a@b.co@0,0,40,10", []ocr.Kind{ocr.KindPhone}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hits := ocr.FindSensitive(ocrtest.Lines(c.line), c.kinds)
			var got []string
			for _, h := range hits {
				got = append(got, h.Text)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestFindSensitiveMapsWords(t *testing.T) {
	r := ocrtest.Lines("Call@0,0,20,10 (555)@25,0,30,10 123-4567@60,0,40,10 today@105,0,30,10")
	hits := ocr.FindSensitive(r, []ocr.Kind{ocr.KindPhone})
	if len(hits) != 1 {
		t.Fatalf("hits = %d", len(hits))
	}
	want := []image.Rectangle{image.Rect(25, 0, 55, 10), image.Rect(60, 0, 100, 10)}
	if !reflect.DeepEqual(hits[0].Rects, want) {
		t.Fatalf("rects = %v, want %v", hits[0].Rects, want)
	}
	// A match inside a longer word covers the whole word.
	r = ocrtest.Lines("mailto:me@x.io@0,0,80,10")
	hits = ocr.FindSensitive(r, ocr.DefaultKinds)
	if len(hits) != 1 || hits[0].Rects[0] != image.Rect(0, 0, 80, 10) {
		t.Fatalf("partial word hit = %+v", hits)
	}
}

func TestParseKind(t *testing.T) {
	if k, ok := ocr.ParseKind(" Email "); !ok || k != ocr.KindEmail {
		t.Fatalf("ParseKind = %q %v", k, ok)
	}
	if _, ok := ocr.ParseKind("ssn"); ok {
		t.Fatal("unknown kind accepted")
	}
}
