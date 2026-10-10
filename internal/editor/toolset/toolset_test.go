package toolset

import (
	"reflect"
	"testing"
)

func TestToolbar(t *testing.T) {
	all := append(append([]string{"select"}, Drawing...), "redact")
	for _, c := range []struct {
		listed  []string
		initial string
		tools   []string
		tool    string
	}{
		// The owner's customised v0.3.2 list: Redact is added, last.
		{[]string{"crop", "arrow", "rect", "text", "blur", "pixelate", "highlight", "step"}, "arrow",
			[]string{"select", "crop", "arrow", "rect", "text", "blur", "pixelate", "highlight", "step", "redact"}, "arrow"},
		// Old tokens are accepted and ignored for placement.
		{[]string{"redact", "select_text", "arrow", "select", "nope"}, "redact",
			[]string{"select", "arrow", "redact"}, "redact"},
		{nil, "", all, "arrow"},
		{[]string{"select_text"}, "select_text", all, "select"},
		// A default tool outside the toolbar falls back to the first drawing tool.
		{[]string{"rect", "text"}, "blur", []string{"select", "rect", "text", "redact"}, "rect"},
		{[]string{"rect"}, "select", []string{"select", "rect", "redact"}, "select"},
	} {
		tools, tool := Toolbar(c.listed, c.initial)
		if !reflect.DeepEqual(tools, c.tools) || tool != c.tool {
			t.Errorf("Toolbar(%v, %q) = %v, %q; want %v, %q", c.listed, c.initial, tools, tool, c.tools, c.tool)
		}
	}
}

func TestStartsRecognition(t *testing.T) {
	for _, c := range []struct {
		auto    bool
		initial string
		want    bool
	}{
		{true, "arrow", true},
		{false, "arrow", false},
		// auto_run off + default_tool select (or the old select_text).
		{false, "select", true},
		{false, "select_text", true},
		{false, "redact", false},
	} {
		_, tool := Toolbar(nil, c.initial)
		if got := StartsRecognition(c.auto, tool); got != c.want {
			t.Errorf("StartsRecognition(%v, %q -> %q) = %v, want %v", c.auto, c.initial, tool, got, c.want)
		}
	}
}
