package ocr

import (
	"reflect"
	"testing"
)

func TestMatchTag(t *testing.T) {
	vision := []string{"en-US", "fr-FR", "de-DE", "zh-Hans", "zh-Hant", "pt-BR"}
	windows := []string{"zh-Hant-TW", "zh-Hans-CN", "en-GB"}
	cases := []struct {
		want string
		tags []string
		idx  int
	}{
		{"en", vision, 0},
		{"EN-us", vision, 0},
		{"de", vision, 2},
		{"zh-Hant", vision, 4},
		{"zh-Hans", windows, 1}, // script first, not the first zh-* tag
		{"zh-hant-hk", windows, 0},
		{"zh", windows, 0},
		{"ja", vision, -1},
		{"", vision, -1},
		{"sr-Latn", []string{"sr"}, 0}, // same language without a script
		{"sr-Cyrl", []string{"sr-Latn"}, -1},
	}
	for _, c := range cases {
		if got := MatchTag(c.want, c.tags); got != c.idx {
			t.Errorf("MatchTag(%q, %v) = %d, want %d", c.want, c.tags, got, c.idx)
		}
	}
	if got := MatchTags([]string{"en", "xx", "en-US", "de"}, vision); !reflect.DeepEqual(got, []string{"en-US", "de-DE"}) {
		t.Errorf("MatchTags = %v", got)
	}
}
