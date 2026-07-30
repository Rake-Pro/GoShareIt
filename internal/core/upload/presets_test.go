package upload

import (
	"net/url"
	"strings"
	"testing"
)

func TestCustomPresetsRenderValidConfigs(t *testing.T) {
	presets := CustomPresets()
	if len(presets) == 0 {
		t.Fatal("CustomPresets returned no presets")
	}

	for id, cfg := range presets {
		t.Run(id, func(t *testing.T) {
			u, err := url.Parse(substitute(cfg.URL, "shot.png", "image/png"))
			if err != nil || !u.IsAbs() {
				t.Fatalf("URL %q is not a valid absolute URL: %v", cfg.URL, err)
			}
			c := NewCustom(cfg, nil)
			if c.method() != "POST" {
				t.Errorf("method = %q, want POST", c.method())
			}
			if c.isRawBody() {
				t.Errorf("preset %q should use a multipart body", id)
			}
			if c.fileField() == "" {
				t.Errorf("preset %q has empty file field", id)
			}
			for k, v := range cfg.Headers {
				if strings.Contains(substitute(v, "shot.png", "image/png"), "{") {
					t.Errorf("preset %q header %q left an unrendered placeholder: %q", id, k, v)
				}
			}
			for k, v := range cfg.ExtraFields {
				if strings.Contains(substitute(v, "shot.png", "image/png"), "{") {
					t.Errorf("preset %q field %q left an unrendered placeholder: %q", id, k, v)
				}
			}
		})
	}

	if presets["imgur"].ResponseURLPath != "data.link" {
		t.Errorf("imgur ResponseURLPath = %q, want %q", presets["imgur"].ResponseURLPath, "data.link")
	}
	if presets["catbox"].ExtraFields["reqtype"] != "fileupload" {
		t.Errorf("catbox reqtype field = %q, want %q", presets["catbox"].ExtraFields["reqtype"], "fileupload")
	}
}
