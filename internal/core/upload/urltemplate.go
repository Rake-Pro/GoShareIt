package upload

import (
	"net/url"
	"strings"
)

// renderURLTemplate substitutes "{token}" placeholders in tmpl with values
// from vals. Each value is escaped for safe inclusion in a URL path; any "/"
// separators within a value (e.g. an object key with a directory prefix) are
// preserved by escaping path segments individually rather than the whole
// value. Unmatched placeholders are left untouched.
func renderURLTemplate(tmpl string, vals map[string]string) string {
	out := tmpl
	for k, v := range vals {
		out = strings.ReplaceAll(out, "{"+k+"}", escapePathSegments(v))
	}
	return out
}

func escapePathSegments(s string) string {
	parts := strings.Split(s, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
