//go:build darwin && cgo

package engines

import (
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/platform/darwin/vision"
)

// New returns the Apple Vision engine.
func New(c Config) ocr.Engine { return vision.New(c.Langs) }
