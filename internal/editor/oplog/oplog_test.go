package oplog

import (
	"slices"
	"testing"
)

func check(t *testing.T, step string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s: items = %v, want %v", step, got, want)
	}
}

func TestAddRemoveModifyUndoRedoOrder(t *testing.T) {
	var l Log[string]
	var items []string
	items = l.Do(items, Op[string]{Kind: Add, Index: 0, After: "a"})
	items = l.Do(items, Op[string]{Kind: Add, Index: 1, After: "b"})
	items = l.Do(items, Op[string]{Kind: Add, Index: 2, After: "c"})
	check(t, "adds", items, []string{"a", "b", "c"})

	// Move b (a modify), then delete a (a remove in the middle of history).
	items = l.Do(items, Op[string]{Kind: Modify, Index: 1, Before: "b", After: "b'"})
	items = l.Do(items, Op[string]{Kind: Remove, Index: 0, Before: "a"})
	check(t, "move+delete", items, []string{"b'", "c"})

	var op Op[string]
	var ok bool
	items, op, ok = l.Undo(items)
	if !ok || op.Kind != Remove {
		t.Fatalf("first undo = %+v %v, want the remove", op, ok)
	}
	check(t, "undo delete", items, []string{"a", "b'", "c"})
	items, op, _ = l.Undo(items)
	if op.Kind != Modify || op.Index != 1 {
		t.Fatalf("second undo = %+v, want the modify at 1", op)
	}
	check(t, "undo move", items, []string{"a", "b", "c"})

	items, _, _ = l.Redo(items)
	check(t, "redo move", items, []string{"a", "b'", "c"})
	items, _, _ = l.Redo(items)
	check(t, "redo delete", items, []string{"b'", "c"})
	if _, _, ok := l.Redo(items); ok {
		t.Fatal("redo past the end succeeded")
	}

	// A new operation after an undo drops the redo side.
	items, _, _ = l.Undo(items)
	items = l.Do(items, Op[string]{Kind: Add, Index: 3, After: "d"})
	if l.CanRedo() {
		t.Fatal("redo survived a new operation")
	}
	check(t, "new op", items, []string{"a", "b'", "c", "d"})

	// Undo everything back to empty, in reverse order.
	for l.CanUndo() {
		items, _, _ = l.Undo(items)
	}
	check(t, "undo all", items, nil)
	if _, _, ok := l.Undo(items); ok {
		t.Fatal("undo on empty history succeeded")
	}
}

func TestRemoveRestoresPosition(t *testing.T) {
	var l Log[int]
	items := []int{1, 2, 3, 4}
	items = l.Do(items, Op[int]{Kind: Remove, Index: 2, Before: 3})
	check2 := func(want ...int) {
		t.Helper()
		if !slices.Equal(items, want) {
			t.Fatalf("items = %v, want %v", items, want)
		}
	}
	check2(1, 2, 4)
	items, _, _ = l.Undo(items)
	check2(1, 2, 3, 4)
	items, _, _ = l.Redo(items)
	check2(1, 2, 4)
}

func TestDoMergeFoldsABurst(t *testing.T) {
	var l Log[int]
	items := []int{0, 10}
	for i := 1; i <= 5; i++ {
		items = l.DoMerge(items, Op[int]{Kind: Modify, Index: 1, Before: items[1], After: items[1] + 1}, "nudge")
	}
	if items[1] != 15 {
		t.Fatalf("after nudges = %v", items)
	}
	items, op, _ := l.Undo(items)
	if items[1] != 10 || op.Before != 10 || op.After != 15 {
		t.Fatalf("one undo should revert the whole burst: items %v op %+v", items, op)
	}
	if l.CanUndo() {
		t.Fatal("burst left more than one entry")
	}

	// Seal (selection change) and a different index both start a new step.
	items, _, _ = l.Redo(items)
	items = l.DoMerge(items, Op[int]{Kind: Modify, Index: 1, Before: 15, After: 16}, "nudge")
	l.Seal()
	items = l.DoMerge(items, Op[int]{Kind: Modify, Index: 1, Before: 16, After: 17}, "nudge")
	items = l.DoMerge(items, Op[int]{Kind: Modify, Index: 0, Before: 0, After: 1}, "nudge")
	items, _, _ = l.Undo(items)
	items, _, _ = l.Undo(items)
	if items[1] != 16 || items[0] != 0 {
		t.Fatalf("separate steps not kept apart: %v", items)
	}
	// A plain Do between two merges ends the burst too.
	items = l.DoMerge(items, Op[int]{Kind: Modify, Index: 1, Before: 16, After: 20}, "stroke")
	items = l.Do(items, Op[int]{Kind: Add, Index: 2, After: 99})
	items = l.DoMerge(items, Op[int]{Kind: Modify, Index: 1, Before: 20, After: 21}, "stroke")
	items, _, _ = l.Undo(items)
	if items[1] != 20 || len(items) != 3 {
		t.Fatalf("merge crossed a Do: %v", items)
	}
}

func TestHistoryIsCapped(t *testing.T) {
	var l Log[int]
	var items []int
	for i := 0; i < MaxOps+50; i++ {
		items = l.Do(items, Op[int]{Kind: Add, Index: len(items), After: i})
	}
	if len(l.done) != MaxOps {
		t.Fatalf("history = %d ops, want %d", len(l.done), MaxOps)
	}
	n := 0
	for l.CanUndo() {
		items, _, _ = l.Undo(items)
		n++
	}
	// The oldest 50 adds can no longer be undone; their items stay.
	if n != MaxOps || len(items) != 50 || items[49] != 49 {
		t.Fatalf("undid %d, items left %v", n, items)
	}
	// A new op releases the whole redo side.
	l.Do(items, Op[int]{Kind: Add, Index: len(items), After: -1})
	if l.CanRedo() || len(l.undone) != 0 {
		t.Fatal("redo entries kept after a new op")
	}
	if full := l.undone[:cap(l.undone)]; len(full) > 0 && full[0] != (Op[int]{}) {
		t.Fatal("discarded redo entries still referenced")
	}
}
