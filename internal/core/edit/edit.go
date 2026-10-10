// Package edit defines the portable annotation-editor seam. The core depends
// only on the Editor interface; the concrete GUI lives out-of-process and is
// invoked by a host-side implementation (see Launcher).
package edit

import (
	"context"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// Action identifies how the user confirmed out of the editor, overriding the
// default config-driven pipeline for this capture only. ActionDefault (the
// zero value) means "run the pipeline as configured" - it is also what a
// cancelled or errored Edit reports.
type Action int

const (
	ActionDefault Action = iota
	ActionCopy
	ActionSave
	ActionUpload
)

// Opts carries per-call context the editor needs to render itself correctly.
type Opts struct {
	// CanUpload reports whether uploads are currently enabled, so the editor
	// can grey out its Upload action button instead of offering an action
	// that would silently fall back.
	CanUpload bool
	// OCR is the host's cached probe result. The editor greys out its text
	// tools when Available is false and shows Reason on hover.
	OCR ocr.Status
}

// Editor presents the captured image for annotation and returns the result.
//
// Contract:
//   - On confirm, returns the edited capture.Result (Bytes re-encoded,
//     Mime/Kind updated, Path cleared since the bytes no longer match any
//     on-disk file) and ok=true. action reports which explicit action button
//     (if any) the user confirmed with; ActionDefault means the plain confirm
//     button was used and the caller should run its normal config-driven
//     pipeline.
//   - If the user CANCELS (Cancel, Esc, window close), returns the input
//     Result unchanged, ok=false and action=ActionDefault. Callers MUST treat
//     ok=false as "discard this capture": the user opened the editor to change
//     the image, so the unedited original must not be copied, saved or
//     uploaded.
//   - Returns an error only for genuine failures (helper crash, timeout,
//     decode error). Callers fail closed on it the same way: the capture is
//     discarded and the error reported. action is ActionDefault on error.
//   - Only KindImage is editable. KindVideo is returned unchanged, ok=false,
//     action=ActionDefault.
type Editor interface {
	Edit(ctx context.Context, in capture.Result, opts Opts) (out capture.Result, action Action, ok bool, err error)
}

// NoopEditor is a stand-in that never edits. It returns the input unchanged
// with ok=false, which a caller following the contract above treats as a
// cancel; core skips the edit step entirely when no Editor is wired.
type NoopEditor struct{}

// Edit returns the input unchanged, action=ActionDefault and ok=false.
func (NoopEditor) Edit(_ context.Context, in capture.Result, _ Opts) (capture.Result, Action, bool, error) {
	return in, ActionDefault, false, nil
}
