package config

import (
	"errors"
	"reflect"
	"testing"
)

func TestOCRDefaults(t *testing.T) {
	cfg := NewDefault()
	if !cfg.OCREnabled() || !cfg.OCRAutoRun() {
		t.Fatal("ocr.enabled and ocr.auto_run default to true")
	}
	if !reflect.DeepEqual(cfg.OCR.QuickRedact, []string{"email", "phone"}) || cfg.OCR.TimeoutSeconds != 20 {
		t.Fatalf("defaults = %+v", cfg.OCR)
	}
	off := false
	cfg.OCR.Enabled, cfg.OCR.AutoRun = &off, &off
	if cfg.OCREnabled() || cfg.OCRAutoRun() {
		t.Fatal("explicit false must win")
	}
}

func TestOCRExplicitEmptyQuickRedactKept(t *testing.T) {
	path := writeFile(t, t.TempDir(), "config.yaml", "upload:\n  enabled: false\nocr:\n  quick_redact: []\n")
	cfg, err := LoadLocalOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OCR.QuickRedact == nil || len(cfg.OCR.QuickRedact) != 0 {
		t.Fatalf("quick_redact = %#v, want an explicit empty list", cfg.OCR.QuickRedact)
	}
}

func TestOCRValidation(t *testing.T) {
	cases := []struct {
		yaml  string
		field string
	}{
		{"ocr:\n  quick_redact: [email, ssn]\n", "ocr.quick_redact"},
		{"ocr:\n  languages: [en, \"de DE\"]\n", "ocr.languages"},
		{"ocr:\n  languages: [en, zh-Hant, pt-BR]\n  quick_redact: [token, url, ip]\n", ""},
	}
	for _, c := range cases {
		path := writeFile(t, t.TempDir(), "config.yaml", "upload:\n  enabled: false\n"+c.yaml)
		_, err := LoadLocalOnly(path)
		if c.field == "" {
			if err != nil {
				t.Errorf("%q: unexpected error %v", c.yaml, err)
			}
			continue
		}
		var fe *FieldError
		if !errors.As(err, &fe) || fe.Field != c.field {
			t.Errorf("%q: err = %v, want a field error on %s", c.yaml, err, c.field)
		}
	}
}

func TestEditorToolsNoMigration(t *testing.T) {
	load := func(t *testing.T, yaml string) *Config {
		t.Helper()
		cfg, err := LoadLocalOnly(writeFile(t, t.TempDir(), "config.yaml", "upload:\n  enabled: false\n"+yaml))
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	// Select and Redact are always in the editor toolbar now, so the old
	// starter list is no longer extended; it loads as written.
	cfg := load(t, "editor:\n  tools: [step, crop, arrow, rect, text, blur, highlight]\n")
	want := []string{"step", "crop", "arrow", "rect", "text", "blur", "highlight"}
	if !reflect.DeepEqual(cfg.Editor.Tools, want) || cfg.Editor.ToolsRevision != 0 {
		t.Fatalf("tools = %v (rev %d), want %v unchanged", cfg.Editor.Tools, cfg.Editor.ToolsRevision, want)
	}
	// Configs written by v0.3.x still load: tools_revision and the old
	// select_text and redact tokens are accepted as they are.
	cfg = load(t, "editor:\n  tools_revision: 1\n  tools: [crop, arrow, redact, select_text]\n")
	want = []string{"crop", "arrow", "redact", "select_text"}
	if !reflect.DeepEqual(cfg.Editor.Tools, want) || cfg.Editor.ToolsRevision != 1 {
		t.Fatalf("v0.3 tools = %v (rev %d)", cfg.Editor.Tools, cfg.Editor.ToolsRevision)
	}
	// No list: the editor shows every tool.
	if cfg := load(t, ""); cfg.Editor.Tools != nil {
		t.Fatalf("empty tools = %v", cfg.Editor.Tools)
	}
}
