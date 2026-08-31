//go:build darwin

package darwin

import (
	"context"
	"fmt"
	"sync"

	"golang.design/x/clipboard"
)

// Clipboard implements the core clipboard seam via golang.design/x/clipboard.
// On darwin that package requires cgo (it links the Cocoa pasteboard).
type Clipboard struct{}

var clipboardInitOnce struct {
	sync.Once
	err error
}

// NewClipboard returns a macOS clipboard backend.
func NewClipboard() *Clipboard { return &Clipboard{} }

// clipboardInit runs clipboard.Init exactly once; subsequent calls reuse the
// result. Init can fail if the process has no access to the pasteboard.
func clipboardInit() error {
	clipboardInitOnce.Do(func() {
		clipboardInitOnce.err = clipboard.Init()
	})
	return clipboardInitOnce.err
}

// WriteText copies plain text to the pasteboard.
func (c *Clipboard) WriteText(s string) error {
	if err := clipboardInit(); err != nil {
		return fmt.Errorf("darwin clipboard: init: %w", err)
	}
	if _, err := clipboard.Write(context.Background(), clipboard.FmtText, []byte(s)); err != nil {
		return fmt.Errorf("darwin clipboard: write text: %w", err)
	}
	return nil
}

// WriteImage copies a PNG-encoded image to the pasteboard.
func (c *Clipboard) WriteImage(png []byte) error {
	if err := clipboardInit(); err != nil {
		return fmt.Errorf("darwin clipboard: init: %w", err)
	}
	if _, err := clipboard.Write(context.Background(), clipboard.FmtImage, png); err != nil {
		return fmt.Errorf("darwin clipboard: write image: %w", err)
	}
	return nil
}

// ReadImage returns the pasteboard image as PNG bytes, or (nil,false) if empty.
func (c *Clipboard) ReadImage() ([]byte, bool) {
	if err := clipboardInit(); err != nil {
		return nil, false
	}
	b, err := clipboard.Read(context.Background(), clipboard.FmtImage)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// ReadText returns the pasteboard text, or ("",false) if empty.
func (c *Clipboard) ReadText() (string, bool) {
	if err := clipboardInit(); err != nil {
		return "", false
	}
	b, err := clipboard.Read(context.Background(), clipboard.FmtText)
	if err != nil || len(b) == 0 {
		return "", false
	}
	return string(b), true
}
