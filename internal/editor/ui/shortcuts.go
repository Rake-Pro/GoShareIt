//go:build darwin || windows || (linux && cgo)

package ui

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
)

// toolKeys maps a single key (no modifier) to its tool. Only read while the
// annotation text field does not have focus.
var toolKeys = map[key.Name]Tool{
	"C": ToolCrop, "A": ToolArrow, "R": ToolRect, "E": ToolEllip, "T": ToolText,
	"B": ToolBlur, "P": ToolPixelate, "H": ToolHighlight, "N": ToolStep,
	"L": ToolLine, "F": ToolFreehand, "X": ToolRedact, "V": ToolSelect,
}

// nudgeKeys are the arrow keys that move the selected shape.
var nudgeKeys = map[key.Name]image.Point{
	key.NameLeftArrow: {-1, 0}, key.NameRightArrow: {1, 0},
	key.NameUpArrow: {0, -1}, key.NameDownArrow: {0, 1},
}

// toolKeyName returns the shortcut key shown for tool t ("" when none).
func toolKeyName(t Tool) string {
	for k, v := range toolKeys {
		if v == t {
			return string(k)
		}
	}
	return ""
}

// toolForKey returns the tool bound to name.
func toolForKey(name key.Name) (Tool, bool) {
	t, ok := toolKeys[name]
	return t, ok
}

// singleKeyFilters are the unmodified keys: tools, stroke [ ], swatches 1-7,
// Delete/Backspace, and the arrows (Shift allowed, for 10 px nudges). A
// filter without Optional matches only when no modifier is held, so
// Ctrl/Cmd+A stays Select all.
func singleKeyFilters() []event.Filter {
	fs := make([]event.Filter, 0, len(toolKeys)+15)
	fs = append(fs, key.Filter{Name: key.NameDeleteBackward}, key.Filter{Name: key.NameDeleteForward})
	for k := range nudgeKeys {
		fs = append(fs, key.Filter{Name: k, Optional: key.ModShift})
	}
	for k := range toolKeys {
		fs = append(fs, key.Filter{Name: k})
	}
	fs = append(fs, key.Filter{Name: "["}, key.Filter{Name: "]"})
	for _, d := range []key.Name{"1", "2", "3", "4", "5", "6", "7"} {
		fs = append(fs, key.Filter{Name: d})
	}
	return fs
}

// shortcutFilters are the modifier shortcuts (Ctrl on Windows/Linux, Cmd on
// macOS).
func shortcutFilters() []event.Filter {
	return []event.Filter{
		key.Filter{Name: "Z", Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Name: "Y", Required: key.ModShortcut},
		key.Filter{Name: "C", Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Name: "S", Required: key.ModShortcut},
		key.Filter{Name: "A", Required: key.ModShortcut},
		key.Filter{Name: "R", Required: key.ModShortcut | key.ModShift},
		key.Filter{Name: "0", Required: key.ModShortcut},
		key.Filter{Name: "1", Required: key.ModShortcut},
		key.Filter{Name: "=", Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Name: "+", Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Name: "-", Required: key.ModShortcut},
		key.Filter{Name: key.NameReturn},
		key.Filter{Name: key.NameEnter},
		key.Filter{Name: key.NameSpace},
	}
}

// handleSingleKey applies an unmodified key press (Shift only for the
// arrows). Stroke and colour keys also restyle a selected shape.
func (e *editor) handleSingleKey(gtx layout.Context, ke key.Event) {
	name := ke.Name
	if d, ok := nudgeKeys[name]; ok {
		if ke.Modifiers.Contain(key.ModShift) {
			d = d.Mul(10)
		}
		e.nudge(d)
		return
	}
	switch name {
	case key.NameDeleteBackward, key.NameDeleteForward:
		e.deleteSelected()
		return
	case "[":
		e.setStroke(e.stroke - 1)
		return
	case "]":
		e.setStroke(e.stroke + 1)
		return
	case "1", "2", "3", "4", "5", "6", "7":
		if i := int(name[0] - '1'); i < len(e.palette) {
			e.setColor(e.palette[i])
		}
		return
	}
	t, ok := toolForKey(name)
	if !ok {
		return
	}
	if !e.hasTool(t) {
		e.setHint(gtx, toolLabel(t)+" is not in this editor's tool list (Settings > Editor > Tools).")
		return
	}
	e.selectTool(gtx, t)
}

// hasTool reports whether t is in the toolbar.
func (e *editor) hasTool(t Tool) bool {
	for _, x := range e.tools {
		if x == t {
			return true
		}
	}
	return false
}

// selectTool switches to t the way a toolbar click does. Select starts a
// deferred recognition (auto-run off) for its text selection, and leaving
// it clears the text selection; Text focuses the annotation field.
func (e *editor) selectTool(gtx layout.Context, t Tool) {
	e.discardArmed = false
	e.cancelGesture()
	if t != ToolSelect {
		e.clearSelection()
		e.hoverText = false
	}
	if t != ToolSelect {
		e.deselect()
	}
	e.tool = t
	switch t {
	case ToolText:
		gtx.Execute(key.FocusCmd{Tag: &e.textIn})
	default:
		// The field is only shown with the Text tool; leaving it focused
		// would swallow every shortcut.
		if gtx.Focused(&e.textIn) {
			gtx.Execute(key.FocusCmd{})
		}
		if t == ToolSelect {
			e.startOCR()
		}
	}
}
