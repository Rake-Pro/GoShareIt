// Package winocr is the Windows OCR engine: Windows.Media.Ocr called with
// CGO off through generated WinRT bindings over go-ole. The pure-Go pixel
// helpers (bitmap.go) build and are tested on every OS; the engine itself is
// windows-only.
package winocr

// The WinRT bindings under internal/winrt are winrt-go-gen output committed
// in-repo (MIT, see LICENSE.winrt-go); the generator is not a module
// dependency. Regenerate only on purpose and review the diff.
//
//go:generate sh ../../../scripts/winrt-gen.sh
