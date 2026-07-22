// Package clipboard defines the clipboard seam.
package clipboard

// Clipboard is the OS clipboard abstraction. Image payloads are PNG-encoded.
type Clipboard interface {
	WriteText(s string) error
	WriteImage(png []byte) error
	ReadImage() ([]byte, bool)
	ReadText() (string, bool)
}
