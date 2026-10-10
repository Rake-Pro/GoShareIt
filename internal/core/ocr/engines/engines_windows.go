//go:build windows

package engines

import (
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/platform/windows/winocr"
)

// New returns the Windows.Media.Ocr engine.
func New(c Config) ocr.Engine { return winocr.New(c.Langs) }
