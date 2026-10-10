//go:build darwin && !cgo

package engines

import "github.com/Rake-Pro/GoShareIt/internal/core/ocr"

// New reports that this build has no Vision shim.
func New(Config) ocr.Engine {
	return ocr.Unavailable{Why: "This build has no text recognition (compiled without cgo)."}
}
