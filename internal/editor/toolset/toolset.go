// Package toolset decides which tools the editor toolbar shows, in which
// order, and which one starts active. It has no GUI dependency, so the
// rules are unit-tested on every platform.
package toolset

import "slices"

// Drawing is every drawing tool, in the default toolbar order.
var Drawing = []string{
	"crop", "arrow", "rect", "ellipse", "text",
	"blur", "pixelate", "highlight", "step", "line", "freehand",
}

// Select and Redact are always in the toolbar, first and last.
const (
	Select = "select"
	Redact = "redact"
	// SelectText is the old Select text tool, now part of Select. It is
	// still accepted in editor.tools (ignored) and as the initial tool.
	SelectText = "select_text"
)

// StartsRecognition reports whether text recognition starts as soon as
// the editor opens: always with auto-run, and with auto-run off when the
// initial tool is Select (text selection lives there, and opening in it
// counts as switching to it). tool is the resolved initial tool (Toolbar).
func StartsRecognition(autoRun bool, tool string) bool {
	return autoRun || tool == Select
}

// Toolbar returns the toolbar tools in order and the initial tool. listed
// (editor.tools) picks and orders the drawing tools; unknown names and
// "select", "redact" and "select_text" are ignored there, and an empty
// result means all of Drawing. Select comes first and Redact last, whatever
// is listed. initial "" means arrow, "select_text" means Select, and a tool
// that is not in the toolbar falls back to the first drawing tool.
func Toolbar(listed []string, initial string) ([]string, string) {
	var tools []string
	for _, t := range listed {
		if slices.Contains(Drawing, t) {
			tools = append(tools, t)
		}
	}
	if len(tools) == 0 {
		tools = slices.Clone(Drawing)
	}
	tools = append(append([]string{Select}, tools...), Redact)
	switch initial {
	case "":
		initial = "arrow"
	case SelectText:
		initial = Select
	}
	if !slices.Contains(tools, initial) {
		initial = tools[1]
	}
	return tools, initial
}
