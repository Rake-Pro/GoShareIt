package ocr

import "testing"

// A Chinese region implies the script, so region-only tags never cross the
// Hans/Hant line in either direction.
func TestMatchTagChineseRegionImpliesScript(t *testing.T) {
	cases := []struct {
		want string
		tags []string
		idx  int
	}{
		{"zh-TW", []string{"zh-Hans", "zh-Hant"}, 1},
		{"zh-HK", []string{"zh-Hans", "zh-Hant"}, 1},
		{"zh-CN", []string{"zh-Hant", "zh-Hans"}, 1},
		{"zh-SG", []string{"zh-Hant", "zh-Hans"}, 1},
		{"zh-Hans", []string{"zh-TW", "zh-CN"}, 1},
		{"zh-Hant", []string{"zh-CN", "zh-TW"}, 1},
		{"zh-Hant", []string{"zh-Hans-CN"}, -1},
		{"zh", []string{"zh-Hans-CN", "zh-Hant-TW"}, 0},
	}
	for _, c := range cases {
		if got := MatchTag(c.want, c.tags); got != c.idx {
			t.Errorf("MatchTag(%q, %v) = %d, want %d", c.want, c.tags, got, c.idx)
		}
	}
}
