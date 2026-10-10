package annotate

import (
	"image"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

var (
	goRegularOnce sync.Once
	goRegular     *opentype.Font
	faceCache     sync.Map // pixel size (int) -> font.Face
)

// DefaultFace returns the editor's text face for a stroke width: Go Regular
// (golang.org/x/image/font/gofont/goregular) at 11*stroke px, 72 dpi, no
// hinting, cached per size. 11*stroke keeps the line height close to the old
// 13*stroke bitmap face so captures and the toolbar preview keep their
// proportions. Faces are not safe for concurrent drawing; the editor draws
// from one goroutine. It falls back to basicfont.Face7x13 if the font cannot
// be loaded.
func DefaultFace(stroke int) font.Face {
	return faceForSize(11 * max(stroke, 1))
}

func faceForSize(size int) font.Face {
	if f, ok := faceCache.Load(size); ok {
		return f.(font.Face)
	}
	goRegularOnce.Do(func() { goRegular, _ = opentype.Parse(goregular.TTF) })
	if goRegular == nil {
		return basicfont.Face7x13
	}
	f, err := opentype.NewFace(goRegular, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return basicfont.Face7x13
	}
	actual, _ := faceCache.LoadOrStore(size, f)
	return actual.(font.Face)
}

// TextSize returns the size of s drawn by Text with the default face at the
// given stroke, so a GUI can preview the exact extent.
func TextSize(s string, stroke int) image.Point {
	face := DefaultFace(stroke)
	m := face.Metrics()
	w := (&font.Drawer{Face: face}).MeasureString(s).Ceil()
	return image.Pt(w, (m.Ascent + m.Descent).Ceil())
}
