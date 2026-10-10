//go:build !darwin && !windows && !linux

package engines

import "github.com/Rake-Pro/GoShareIt/internal/core/ocr"

// New reports that this platform has no OCR engine.
func New(Config) ocr.Engine {
	return ocr.Unavailable{Why: "Text recognition is not available on this platform."}
}
