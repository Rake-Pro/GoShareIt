package name

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDefault(t *testing.T) {
	defer restore()
	nowFunc = func() time.Time { return time.Date(2026, 6, 23, 14, 5, 9, 0, time.UTC) }
	randFunc = func() string { return "deadbe" }

	got := Render("", "png")
	want := "goshareit_2026-06-23_14-05-09_deadbe.png"
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRenderTokens(t *testing.T) {
	defer restore()
	nowFunc = func() time.Time { return time.Date(2026, 6, 23, 14, 5, 9, 0, time.UTC) }
	randFunc = func() string { return "abc123" }

	cases := map[string]string{
		"{date}":     "2026-06-23",
		"{time}":     "14-05-09",
		"{datetime}": "2026-06-23_14-05-09",
		"{rand}":     "abc123",
		"x.{ext}":    "x.jpg",
	}
	for tmpl, want := range cases {
		if got := Render(tmpl, "jpg"); got != want {
			t.Errorf("Render(%q) = %q, want %q", tmpl, got, want)
		}
	}
}

func TestRenderExtStripsDot(t *testing.T) {
	defer restore()
	if got := Render("f.{ext}", ".png"); got != "f.png" {
		t.Errorf("Render = %q, want f.png", got)
	}
}

func TestRandomTokenLength(t *testing.T) {
	tok := randomToken()
	if len(tok) != 6 {
		t.Errorf("token len = %d, want 6", len(tok))
	}
	if strings.ContainsAny(tok, "ghijklmnopqrstuvwxyz") {
		t.Errorf("token not hex: %q", tok)
	}
}

func restore() {
	nowFunc = time.Now
	randFunc = randomToken
}
