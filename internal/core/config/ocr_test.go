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

func TestEditorToolsMigration(t *testing.T) {
	load := func(t *testing.T, yaml string) *Config {
		t.Helper()
		cfg, err := LoadLocalOnly(writeFile(t, t.TempDir(), "config.yaml", "upload:\n  enabled: false\n"+yaml))
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	// The old starter list, in any order, gets the two new tools.
	cfg := load(t, "editor:\n  tools: [step, crop, arrow, rect, text, blur, highlight]\n")
	want := []string{"step", "crop", "arrow", "rect", "text", "blur", "highlight", "redact", "select_text"}
	if !reflect.DeepEqual(cfg.Editor.Tools, want) || !cfg.MigratedEditorTools() || cfg.Editor.ToolsRevision != 1 {
		t.Fatalf("migrated tools = %v (migrated %v, rev %d)", cfg.Editor.Tools, cfg.MigratedEditorTools(), cfg.Editor.ToolsRevision)
	}
	// Customised lists are left alone.
	for _, y := range []string{
		"editor:\n  tools: [crop, arrow, rect, text, blur, highlight]\n",
		"editor:\n  tools: [crop, arrow, rect, text, blur, highlight, step, line]\n",
		"editor:\n  tools: [crop, arrow, rect, text, blur, highlight, highlight]\n",
	} {
		if cfg := load(t, y); cfg.MigratedEditorTools() || len(cfg.Editor.Tools) > 8 {
			t.Errorf("%q migrated to %v", y, cfg.Editor.Tools)
		}
	}
	// Once recorded, a list trimmed back to the old set stays as the user left it.
	cfg = load(t, "editor:\n  tools_revision: 1\n  tools: [crop, arrow, rect, text, blur, highlight, step]\n")
	if cfg.MigratedEditorTools() || len(cfg.Editor.Tools) != 7 {
		t.Fatalf("revision 1 list changed to %v", cfg.Editor.Tools)
	}
	// No list: the editor shows every tool; nothing to migrate.
	if cfg := load(t, ""); cfg.MigratedEditorTools() || cfg.Editor.Tools != nil {
		t.Fatalf("empty tools = %v", cfg.Editor.Tools)
	}
}
