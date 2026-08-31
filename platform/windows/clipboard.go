//go:build windows

package windows

import (
	"context"
	"fmt"
	"sync"

	"golang.design/x/clipboard"
)

// Clipboard implements the core clipboard seam via golang.design/x/clipboard.
// On Windows that package is pure syscall (no cgo): it talks to the Win32
// clipboard API directly.
type Clipboard struct{}

var clipboardInitOnce struct {
	sync.Once
	err error
}

// NewClipboard returns a Windows clipboard backend.
func NewClipboard() *Clipboard { return &Clipboard{} }

// clipboardInit runs clipboard.Init exactly once; subsequent calls reuse the
// result. It is shared with the interactive capture path in capture.go.
func clipboardInit() error {
	clipboardInitOnce.Do(func() {
		clipboardInitOnce.err = clipboard.Init()
	})
	return clipboardInitOnce.err
}

// readImage returns the clipboard image as PNG bytes (the x/clipboard package
// normalizes FmtImage to canonical PNG on Windows), or (nil,false) if empty.
// Shared with capture.go's interactive snip poller.
func readImage() ([]byte, bool) {
	if err := clipboardInit(); err != nil {
		return nil, false
	}
	b, err := clipboard.Read(context.Background(), clipboard.FmtImage)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// WriteText copies plain text to the clipboard.
func (c *Clipboard) WriteText(s string) error {
	if err := clipboardInit(); err != nil {
		return fmt.Errorf("windows clipboard: init: %w", err)
	}
	if _, err := clipboard.Write(context.Background(), clipboard.FmtText, []byte(s)); err != nil {
		return fmt.Errorf("windows clipboard: write text: %w", err)
	}
	return nil
}

// WriteImage copies a PNG-encoded image to the clipboard.
func (c *Clipboard) WriteImage(png []byte) error {
	if err := clipboardInit(); err != nil {
		return fmt.Errorf("windows clipboard: init: %w", err)
	}
	if _, err := clipboard.Write(context.Background(), clipboard.FmtImage, png); err != nil {
		return fmt.Errorf("windows clipboard: write image: %w", err)
	}
	return nil
}

// ReadImage returns the clipboard image as PNG bytes, or (nil,false) if empty.
func (c *Clipboard) ReadImage() ([]byte, bool) { return readImage() }

// ReadText returns the clipboard text, or ("",false) if empty.
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
