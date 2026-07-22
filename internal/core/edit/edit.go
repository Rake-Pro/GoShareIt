// Package edit defines the portable annotation-editor seam. The core depends
// only on the Editor interface; the concrete GUI lives out-of-process and is
// invoked by a host-side implementation (see Launcher).
package edit

import (
	"context"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// Editor presents the captured image for annotation and returns the result.
//
// Contract:
//   - On confirm, returns the edited capture.Result (Bytes re-encoded,
//     Mime/Kind updated, Path cleared since the bytes no longer match any
//     on-disk file) and ok=true.
//   - If the user SKIPS or CANCELS, returns the input Result unchanged and
//     ok=false. Callers MUST treat ok=false as "proceed with the original".
//   - Returns an error only for genuine failures (helper crash, decode error).
//     A failure is non-fatal to the pipeline: the caller logs and continues
//     with the original image (fail-open), matching the app's "never fatal on
//     a non-core failure" posture.
//   - Only KindImage is editable. KindVideo is returned unchanged, ok=false.
type Editor interface {
	Edit(ctx context.Context, in capture.Result) (out capture.Result, ok bool, err error)
}

// NoopEditor is the default when editing is disabled. It always passes through
// the input unchanged with ok=false.
type NoopEditor struct{}

// Edit returns the input unchanged and ok=false.
func (NoopEditor) Edit(_ context.Context, in capture.Result) (capture.Result, bool, error) {
	return in, false, nil
}
