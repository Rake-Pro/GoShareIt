package ocr

import "strings"

// MatchTag returns the index in tags (an engine's supported or installed
// BCP-47 tags) of the best match for the configured tag want, or -1:
//
//  1. an exact match, ignoring case ("en-US" == "en-us");
//  2. the same language and, when want names a script ("zh-Hans"), the same
//     script, so a configured "zh-Hans" never picks "zh-Hant-TW";
//  3. with a script in want, the same language with no script in the tag;
//     without one, the first tag of the same language ("en" -> "en-US").
func MatchTag(want string, tags []string) int {
	want = strings.TrimSpace(want)
	for i, t := range tags {
		if strings.EqualFold(t, want) {
			return i
		}
	}
	wl, ws := langScript(want)
	if wl == "" {
		return -1
	}
	if ws != "" {
		for i, t := range tags {
			if l, s := langScript(t); l == wl && s == ws {
				return i
			}
		}
		for i, t := range tags {
			if l, s := langScript(t); l == wl && s == "" {
				return i
			}
		}
		return -1
	}
	for i, t := range tags {
		if l, _ := langScript(t); l == wl {
			return i
		}
	}
	return -1
}

// MatchTags maps every wanted tag through MatchTag and returns the matched
// supported tags, without duplicates, in the wanted order.
func MatchTags(want, tags []string) []string {
	var out []string
	seen := map[int]bool{}
	for _, w := range want {
		if i := MatchTag(w, tags); i >= 0 && !seen[i] {
			seen[i] = true
			out = append(out, tags[i])
		}
	}
	return out
}

// langScript returns the lower-case language subtag and the title-case
// script subtag (4 letters, "" when absent) of a BCP-47 tag. For Chinese a
// region implies the script when none is spelled out (zh-CN and zh-SG are
// Hans, zh-TW, zh-HK and zh-MO are Hant), so a configured "zh-TW" never
// matches an engine's "zh-Hans" and vice versa.
func langScript(tag string) (lang, script string) {
	parts := strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		return "", ""
	}
	lang = strings.ToLower(parts[0])
	if len(parts) > 1 && len(parts[1]) == 4 && isAlpha(parts[1]) {
		script = strings.ToUpper(parts[1][:1]) + strings.ToLower(parts[1][1:])
	}
	if script == "" && lang == "zh" {
		for _, p := range parts[1:] {
			switch strings.ToUpper(p) {
			case "CN", "SG":
				return lang, "Hans"
			case "TW", "HK", "MO":
				return lang, "Hant"
			}
		}
	}
	return lang, script
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}
