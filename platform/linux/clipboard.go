//go:build linux

package linux

import (
	"context"
	"fmt"
	"sync"

	"golang.design/x/clipboard"
)

// Clipboard implements the core clipboard seam via golang.design/x/clipboard.
// On Linux that package is pure Go: it speaks the Wayland data-control
// protocol where the compositor offers it and falls back to the X11
// selection protocol (over XWayland on compositors that do not).
type Clipboard struct{}

var clipboardInitOnce struct {
	sync.Once
	err error
}

// NewClipboard returns a Linux clipboard backend.
func NewClipboard() *Clipboard { return &Clipboard{} }

// clipboardInit runs clipboard.Init exactly once; subsequent calls reuse the
// result. Init fails when neither a Wayland nor an X11 display is reachable.
func clipboardInit() error {
	clipboardInitOnce.Do(func() {
		clipboardInitOnce.err = clipboard.Init()
	})
	return clipboardInitOnce.err
}

// WriteText copies plain text to the clipboard.
func (c *Clipboard) WriteText(s string) error {
	if err := clipboardInit(); err != nil {
		return fmt.Errorf("linux clipboard: init: %w", err)
	}
	if _, err := clipboard.Write(context.Background(), clipboard.FmtText, []byte(s)); err != nil {
		return fmt.Errorf("linux clipboard: write text: %w", err)
	}
	return nil
}

// WriteImage copies a PNG-encoded image to the clipboard.
func (c *Clipboard) WriteImage(png []byte) error {
	if err := clipboardInit(); err != nil {
		return fmt.Errorf("linux clipboard: init: %w", err)
	}
	if _, err := clipboard.Write(context.Background(), clipboard.FmtImage, png); err != nil {
		return fmt.Errorf("linux clipboard: write image: %w", err)
	}
	return nil
}

// ReadImage returns the clipboard image as PNG bytes, or (nil,false) if empty.
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
