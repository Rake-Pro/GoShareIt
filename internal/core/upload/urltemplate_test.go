package upload

import "testing"

func TestRenderURLTemplate(t *testing.T) {
	cases := []struct {
		name string
		tmpl string
		vals map[string]string
		want string
	}{
		{
			name: "simple substitution",
			tmpl: "https://{bucket}.example.com/{key}",
			vals: map[string]string{"bucket": "my-bucket", "key": "shot.png"},
			want: "https://my-bucket.example.com/shot.png",
		},
		{
			name: "key with directory prefix keeps slashes",
			tmpl: "https://cdn.example.com/{key}",
			vals: map[string]string{"key": "screenshots/2026/shot one.png"},
			want: "https://cdn.example.com/screenshots/2026/shot%20one.png",
		},
		{
			name: "name with special characters is escaped",
			tmpl: "https://cdn.example.com/{name}",
			vals: map[string]string{"name": "a b#c.png"},
			want: "https://cdn.example.com/a%20b%23c.png",
		},
		{
			name: "unmatched placeholder left untouched",
			tmpl: "https://cdn.example.com/{name}?x={unknown}",
			vals: map[string]string{"name": "a.png"},
			want: "https://cdn.example.com/a.png?x={unknown}",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderURLTemplate(tc.tmpl, tc.vals)
			if got != tc.want {
				t.Errorf("renderURLTemplate(%q, %v) = %q, want %q", tc.tmpl, tc.vals, got, tc.want)
			}
		})
	}
}
