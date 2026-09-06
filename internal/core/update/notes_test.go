package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMinorBump(t *testing.T) {
	cases := []struct {
		cur, next string
		want      bool
	}{
		{"1.2.9", "1.2.10", false},
		{"1.2.9", "1.3.0", true},
		{"1.9.0", "2.0.0", true},
		{"0.1.17", "0.1.18", false},
		{"0.1.17", "0.2.0", true},
		{"junk", "1.0.0", false},
	}
	for _, c := range cases {
		if got := MinorBump(c.cur, c.next); got != c.want {
			t.Errorf("MinorBump(%s, %s) = %v", c.cur, c.next, got)
		}
	}
}

func TestNotesSpanAndRender(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
		 {"tag_name":"v1.3.0","published_at":"2026-09-06T00:00:00Z","body":"## What's Changed\n* Big **thing** by [me](https://x)\n\n- second"},
		 {"tag_name":"v1.2.10","published_at":"2026-09-05T00:00:00Z","body":"patch"},
		 {"tag_name":"v1.2.9","body":"old"},
		 {"tag_name":"v1.4.0","body":"future"},
		 {"tag_name":"v1.2.11","draft":true,"body":"draft"}
		]`))
	}))
	defer srv.Close()
	u, _ := New(Config{Repo: "o/r", Current: "1.2.9", APIBaseURL: srv.URL})
	notes, err := u.Notes(context.Background(), "1.2.9", "1.3.0")
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if len(notes) != 2 || notes[0].Version != "1.2.10" || notes[1].Version != "1.3.0" {
		t.Fatalf("notes = %+v", notes)
	}
	lines := RenderNotes(notes)
	var texts []string
	for _, l := range lines {
		texts = append(texts, l.Text)
	}
	joined := ""
	for _, s := range texts {
		joined += s + "\n"
	}
	for _, want := range []string{"v1.2.10  (2026-09-05)", "v1.3.0  (2026-09-06)", "What's Changed", "• Big thing by me", "• second"} {
		found := false
		for _, s := range texts {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing line %q in:\n%s", want, joined)
		}
	}
}
