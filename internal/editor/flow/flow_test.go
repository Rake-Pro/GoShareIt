package flow

import (
	"image"
	"testing"
)

func sizes(h int, ws ...int) []image.Point {
	out := make([]image.Point, len(ws))
	for i, w := range ws {
		out[i] = image.Pt(w, h)
	}
	return out
}

// checkInside fails when an item leaves [0, maxW] or overlaps another item.
func checkInside(t *testing.T, name string, pos, sz []image.Point, maxW int) {
	t.Helper()
	for i := range pos {
		r := image.Rectangle{Min: pos[i], Max: pos[i].Add(sz[i])}
		if r.Min.X < 0 || r.Max.X > maxW {
			t.Errorf("%s: item %d at %v is outside 0..%d", name, i, r, maxW)
		}
		for j := range i {
			o := image.Rectangle{Min: pos[j], Max: pos[j].Add(sz[j])}
			if r.Overlaps(o) {
				t.Errorf("%s: items %d %v and %d %v overlap", name, j, o, i, r)
			}
		}
	}
}

func rowsOf(pos []image.Point) int {
	seen := map[int]bool{}
	for _, p := range pos {
		seen[p.Y] = true
	}
	return len(seen)
}

func TestWrap(t *testing.T) {
	sz := sizes(10, 40, 40, 40, 40)
	pos, total := Wrap(sz, 100, 5, 2, false)
	want := []image.Point{{0, 0}, {45, 0}, {0, 12}, {45, 12}}
	for i := range want {
		if pos[i] != want[i] {
			t.Fatalf("pos = %v, want %v", pos, want)
		}
	}
	if total != image.Pt(85, 22) {
		t.Fatalf("total = %v", total)
	}
	// Right-aligned rows end at maxW; a short last row too.
	pos, _ = Wrap(sizes(10, 40, 40, 40), 100, 5, 0, true)
	if pos[1].X+40 != 100 || pos[2].X+40 != 100 {
		t.Fatalf("right pos = %v", pos)
	}
	// An item wider than the row gets its own row, starting at 0.
	pos, _ = Wrap(sizes(10, 30, 150, 30), 100, 0, 0, false)
	if pos[1] != image.Pt(0, 10) || pos[2] != image.Pt(0, 20) {
		t.Fatalf("wide pos = %v", pos)
	}
	// Shorter items are centred in their row.
	pos, total = Wrap([]image.Point{{20, 40}, {20, 20}}, 100, 0, 0, false)
	if pos[1].Y != 10 || total.Y != 40 {
		t.Fatalf("centred pos = %v total %v", pos, total)
	}
	if _, total := Wrap(nil, 100, 0, 0, true); total != (image.Point{}) {
		t.Fatalf("empty total = %v", total)
	}
}

func TestBar(t *testing.T) {
	left, right := sizes(10, 50, 50), sizes(10, 30, 60)
	pos, total := Bar(left, right, 300, 0, 12, 0)
	if pos[0].X != 0 || pos[2].X != 210 || pos[3].X+60 != 300 || total != image.Pt(300, 10) {
		t.Fatalf("one row: pos %v total %v", pos, total)
	}
	// Too narrow for both: left rows, then right rows right-aligned.
	pos, total = Bar(left, right, 150, 0, 12, 0)
	if pos[2].Y != 10 || pos[3].X+60 != 150 || total.Y != 20 {
		t.Fatalf("wrapped: pos %v total %v", pos, total)
	}
	checkInside(t, "bar", pos, append(append([]image.Point{}, left...), right...), 150)
}

func TestStretch(t *testing.T) {
	if w := Stretch(800, 1000, 140, 320, 240); w != 200 {
		t.Fatalf("spare 200 -> %d", w)
	}
	if w := Stretch(300, 1000, 140, 320, 240); w != 320 {
		t.Fatalf("capped -> %d", w)
	}
	if w := Stretch(900, 1000, 140, 320, 240); w != 240 {
		t.Fatalf("fallback -> %d", w)
	}
	if w := Stretch(900, 200, 140, 320, 240); w != 200 {
		t.Fatalf("fallback capped -> %d", w)
	}
}

// The owner's v0.3.2 toolbar, in dp, measured from the macOS screenshot at
// 1000 dp (button widths include the 4 dp inset on each side; the key
// suffix " (V)" adds about 22 dp, so the 10 tools take 979 dp with keys and
// 759 dp without). The editor measures the real labels at run time; these
// numbers pin down the wrapping rules at the widths that matter: the 640 dp
// minimum, 900, the 1000 dp default and 1400.
var (
	ownerTools    = []int{97, 88, 96, 121, 83, 84, 107, 116, 87, 100} // Select .. Step, Redact, with keys
	ownerSuffix   = 22
	ownerSwatches = 7 * 30
	ownerControls = []int{129, 64, 65, 64, 88} // stroke -/px/+, zoom, Undo, Redo, Copy text
	ownerActions  = []int{64, 64, 77, 205}     // Copy, Save, Upload, Cancel+Confirm
	ownerQuick    = 110                        // Quick redact (Redact tool)
	ownerRedSel   = 125                        // Redact selection (Select tool)
)

const (
	toolbarInset = 16 // 8 dp each side
	groupGap     = 12
	buttonH      = 44
	swatchH      = 30
)

// ownerVariants builds the controls of each tool the way layoutActionBar
// does: the Text field and Quick redact go after stroke and zoom (index
// 2), Redact selection at the end.
func ownerVariants(maxW int) map[string][]int {
	with := func(at, w int) []int {
		return append(append(append([]int(nil), ownerControls[:at]...), w), ownerControls[at:]...)
	}
	used := Width(ownerControls, 0) + groupGap + Width(ownerActions, 0)
	return map[string][]int{
		"arrow":  ownerControls,
		"text":   with(2, Stretch(used, maxW, 140, 320, 240)),
		"redact": with(2, ownerQuick),
		"select": with(len(ownerControls), ownerRedSel),
	}
}

func TestOwnerToolbar(t *testing.T) {
	for _, c := range []struct {
		window    int
		keys      bool
		toolRows  int            // rows the tool buttons take
		swatchRow int            // row index of the swatch group in the tool flow
		barRows   map[string]int // toolbar rows of the controls + actions per tool
	}{
		{640, false, 2, 1, map[string]int{"arrow": 2, "text": 3, "redact": 2, "select": 2}},
		{900, false, 1, 1, map[string]int{"arrow": 1, "text": 2, "redact": 2, "select": 2}},
		{1000, true, 1, 1, map[string]int{"arrow": 1, "text": 1, "redact": 1, "select": 1}},
		{1400, true, 1, 0, map[string]int{"arrow": 1, "text": 1, "redact": 1, "select": 1}},
	} {
		maxW := c.window - toolbarInset
		keys := Fits(ownerTools, maxW, 0)
		if keys != c.keys {
			t.Errorf("%d dp: keys shown = %v, want %v", c.window, keys, c.keys)
		}
		tools := make([]int, len(ownerTools))
		for i, w := range ownerTools {
			tools[i] = w
			if !keys {
				tools[i] -= ownerSuffix
			}
		}
		row1 := append(sizes(buttonH, tools...), image.Pt(ownerSwatches, swatchH))
		pos, _ := Wrap(row1, maxW, 0, 0, false)
		checkInside(t, "tools", pos, row1, maxW)
		if n := rowsOf(pos[:len(tools)]); n != c.toolRows {
			t.Errorf("%d dp: tool rows = %d, want %d", c.window, n, c.toolRows)
		}
		if row := (pos[len(tools)].Y + swatchH/2) / buttonH; row != c.swatchRow {
			t.Errorf("%d dp: swatches on row %d, want %d", c.window, row, c.swatchRow)
		}

		right := sizes(buttonH, ownerActions...)
		var variants [][]image.Point
		most := 0
		for tool, controls := range ownerVariants(maxW) {
			left := sizes(buttonH, controls...)
			variants = append(variants, left)
			pos, total := Bar(left, right, maxW, 0, groupGap, 0)
			all := append(append([]image.Point{}, left...), right...)
			checkInside(t, tool, pos, all, maxW)
			if n := rowsOf(pos); n != c.barRows[tool] || total.Y != n*buttonH {
				t.Errorf("%d dp %s: row 2 takes %d rows (%d dp), want %d", c.window, tool, n, total.Y, c.barRows[tool])
			}
			most = max(most, c.barRows[tool])
			// Cancel and Confirm (the last item) always end at the right edge.
			last := len(pos) - 1
			if pos[last].X+all[last].X != maxW {
				t.Errorf("%d dp %s: Confirm ends at %d, want %d", c.window, tool, pos[last].X+all[last].X, maxW)
			}
		}
		// The reserved height is the tallest tool's, so a tool switch never
		// changes the toolbar height.
		if h := MaxHeight(variants, right, maxW, 0, groupGap, 0); h != most*buttonH {
			t.Errorf("%d dp: reserved height %d, want %d", c.window, h, most*buttonH)
		}
	}
}

func TestBarEmptyGroup(t *testing.T) {
	wide := sizes(10, 60, 60, 60)
	// Only a left group, wider than the row: it wraps instead of overflowing.
	pos, total := Bar(wide, nil, 130, 0, 12, 0)
	checkInside(t, "left only", pos, wide, 130)
	if total.Y != 20 {
		t.Fatalf("left only: pos %v total %v", pos, total)
	}
	// Only a right group: it wraps and stays right-aligned.
	pos, total = Bar(nil, wide, 130, 0, 12, 0)
	checkInside(t, "right only", pos, wide, 130)
	if total.Y != 20 || pos[2].X+60 != 130 || pos[1].X+60 != 130 {
		t.Fatalf("right only: pos %v total %v", pos, total)
	}
}
