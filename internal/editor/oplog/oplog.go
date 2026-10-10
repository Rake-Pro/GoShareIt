// Package oplog is the editor's undo history: a log of operations on an
// ordered list (add, remove, modify at an index) instead of snapshots of the
// whole list. Each Op holds both sides, so undo and redo are exact and a
// move or a recolour of one shape costs one entry. It is generic and free of
// any GUI toolkit so it unit-tests with CGO off.
package oplog

import "slices"

// Kind is what an Op does to the list.
type Kind int

const (
	// Add inserts After at Index.
	Add Kind = iota
	// Remove deletes the item at Index (Before holds it for undo).
	Remove
	// Modify replaces the item at Index: Before with After.
	Modify
)

// Op is one undoable change.
type Op[T any] struct {
	Kind   Kind
	Index  int
	Before T // Remove, Modify
	After  T // Add, Modify
}

// MaxOps caps the undo history; the oldest operations are dropped beyond
// it, so a long session does not keep every shape version alive.
const MaxOps = 500

// Log is the undo/redo history. The zero value is ready to use.
type Log[T any] struct {
	done, undone []Op[T]
	// mergeKey is the key of the last DoMerge; a following DoMerge with the
	// same key on the same index folds into that entry.
	mergeKey string
}

// Do applies op to items, records it and clears the redo side. It returns
// the new list (items may be modified in place).
func (l *Log[T]) Do(items []T, op Op[T]) []T {
	l.mergeKey = ""
	return l.record(items, op)
}

// DoMerge is Do for a burst of small changes to one item (keyboard nudges,
// stroke steps): a Modify with the same key on the same index as the
// previous DoMerge updates that entry's After instead of adding a new one,
// so the whole burst is one undo step. Any other call ends the burst.
func (l *Log[T]) DoMerge(items []T, op Op[T], key string) []T {
	if n := len(l.done); key != "" && op.Kind == Modify && n > 0 && l.mergeKey == key &&
		len(l.undone) == 0 && l.done[n-1].Kind == Modify && l.done[n-1].Index == op.Index {
		l.done[n-1].After = op.After
		return apply(items, op)
	}
	items = l.record(items, op)
	l.mergeKey = key
	return items
}

// Seal ends a DoMerge burst, so the next DoMerge starts a new undo step
// (used when the selection changes).
func (l *Log[T]) Seal() { l.mergeKey = "" }

func (l *Log[T]) record(items []T, op Op[T]) []T {
	items = apply(items, op)
	l.done = append(l.done, op)
	if n := len(l.done); n > MaxOps {
		l.done = slices.Delete(l.done, 0, n-MaxOps) // zeroes the freed tail
	}
	// Release the discarded redo entries, not just the length.
	clear(l.undone)
	l.undone = l.undone[:0]
	return items
}

// Undo reverts the last operation. ok is false when there is nothing to
// undo; op is the operation that was reverted.
func (l *Log[T]) Undo(items []T) (out []T, op Op[T], ok bool) {
	l.mergeKey = ""
	if len(l.done) == 0 {
		return items, op, false
	}
	op = l.done[len(l.done)-1]
	l.done[len(l.done)-1] = Op[T]{}
	l.done = l.done[:len(l.done)-1]
	l.undone = append(l.undone, op)
	return revert(items, op), op, true
}

// Redo re-applies the last undone operation. ok is false when there is
// nothing to redo.
func (l *Log[T]) Redo(items []T) (out []T, op Op[T], ok bool) {
	l.mergeKey = ""
	if len(l.undone) == 0 {
		return items, op, false
	}
	op = l.undone[len(l.undone)-1]
	l.undone[len(l.undone)-1] = Op[T]{}
	l.undone = l.undone[:len(l.undone)-1]
	l.done = append(l.done, op)
	return apply(items, op), op, true
}

// CanUndo reports whether Undo has something to revert.
func (l *Log[T]) CanUndo() bool { return len(l.done) > 0 }

// CanRedo reports whether Redo has something to re-apply.
func (l *Log[T]) CanRedo() bool { return len(l.undone) > 0 }

func apply[T any](items []T, op Op[T]) []T {
	switch op.Kind {
	case Add:
		return slices.Insert(items, min(max(op.Index, 0), len(items)), op.After)
	case Remove:
		if op.Index >= 0 && op.Index < len(items) {
			return slices.Delete(items, op.Index, op.Index+1)
		}
	case Modify:
		if op.Index >= 0 && op.Index < len(items) {
			items[op.Index] = op.After
		}
	}
	return items
}

func revert[T any](items []T, op Op[T]) []T {
	switch op.Kind {
	case Add:
		return apply(items, Op[T]{Kind: Remove, Index: op.Index})
	case Remove:
		return apply(items, Op[T]{Kind: Add, Index: op.Index, After: op.Before})
	case Modify:
		return apply(items, Op[T]{Kind: Modify, Index: op.Index, After: op.Before})
	}
	return items
}
