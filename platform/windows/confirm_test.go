//go:build windows

package windows

import "testing"

func TestPSEscapeDoublesAllSingleQuotes(t *testing.T) {
	cases := map[string]string{
		"it's":            "it''s",
		"a\u2018b\u2019c": "a\u2018\u2018b\u2019\u2019c",
		"\u201a\u201b":    "\u201a\u201a\u201b\u201b",
		"plain":           "plain",
	}
	for in, want := range cases {
		if got := psEscape(in); got != want {
			t.Errorf("psEscape(%q) = %q, want %q", in, got, want)
		}
	}
}
