package upload

import (
	"net/url"
	"strings"
	"testing"
)

// TestCustomPresetsRenderValidConfigs checks every preset is a well-formed
// request template with documentation: an absolute URL, a known method and
// body kind, a file field for multipart bodies, notes for the UI, and a secret
// hint whenever {secret} appears anywhere in it.
func TestCustomPresetsRenderValidConfigs(t *testing.T) {
	presets := CustomPresets()
	if len(presets) < 10 {
		t.Fatalf("CustomPresets returned %d presets, want the full catalog", len(presets))
	}

	for id, p := range presets {
		t.Run(id, func(t *testing.T) {
			cfg := p.CustomConfig
			u, err := url.Parse(substitute(cfg.URL, "shot.png", "image/png"))
			if err != nil || !u.IsAbs() || u.Scheme != "https" {
				t.Fatalf("URL %q is not a valid absolute https URL: %v", cfg.URL, err)
			}
			c := NewCustom(cfg, nil)
			if m := c.method(); m != "POST" && m != "PUT" {
				t.Errorf("method = %q, want POST or PUT", m)
			}
			if !c.isRawBody() && c.fileField() == "" {
				t.Errorf("multipart preset has empty file field")
			}
			if c.isRawBody() && len(cfg.ExtraFields) > 0 {
				t.Errorf("raw-body preset carries extra fields that would be ignored")
			}
			if p.Label == "" || p.Kinds == "" || p.Help == "" {
				t.Errorf("preset is missing Label/Kinds/Help notes")
			}

			usesSecret := strings.Contains(cfg.URL, "{secret")
			for _, v := range cfg.Headers {
				usesSecret = usesSecret || strings.Contains(v, "{secret")
			}
			for _, v := range cfg.ExtraFields {
				usesSecret = usesSecret || strings.Contains(v, "{secret")
			}
			if usesSecret && p.Secret == "" {
				t.Errorf("preset uses {secret} but does not say what the secret is")
			}
			if usesSecret && p.SecretOptional {
				t.Errorf("preset uses {secret} but claims the secret is optional; an empty secret would be sent")
			}

			// Templates must reference only the response and known placeholders.
			for _, tmpl := range []string{cfg.ResponseURLTemplate, cfg.ResponseDirectURLTemplate, cfg.ResponseDeleteURLTemplate} {
				if tmpl == "" {
					continue
				}
				rest := responsePlaceholder.ReplaceAllString(tmpl, "")
				if strings.ContainsAny(rest, "{}") {
					t.Errorf("template %q has an unknown placeholder", tmpl)
				}
				if strings.Contains(tmpl, "{regex") && cfg.ResponseURLRegex == "" {
					t.Errorf("template %q uses {regex} without a response URL regex", tmpl)
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
