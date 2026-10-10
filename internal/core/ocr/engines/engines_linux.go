//go:build linux

package engines

import (
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/tesseract"
)

// New returns the tesseract CLI engine (pure Go, so the CGO-off build gets
// it too and reports a truthful status).
func New(c Config) ocr.Engine {
	return &tesseract.Engine{Path: c.TesseractPath, Langs: c.Langs}
}
