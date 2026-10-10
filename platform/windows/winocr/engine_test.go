//go:build windows

package winocr

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// TestRecognizeSynthetic runs real Windows OCR over a rendered string. It
// skips with the probe reason when no OCR language pack is installed; run it
// on a Windows machine with go test ./platform/windows/winocr/.
func TestRecognizeSynthetic(t *testing.T) {
	e := New(nil)
	st := e.Probe(context.Background())
	if !st.Available {
		t.Skipf("Windows OCR unavailable: %s", st.Explain())
	}
	t.Logf("recognizer languages: %v", st.Langs)

	img := image.NewRGBA(image.Rect(0, 0, 600, 120))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 48, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		t.Fatal(err)
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.Black), Face: face, Dot: fixed.P(20, 80)}
	d.DrawString("GoShareIt 12345")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := e.Recognize(ctx, img, ocr.Options{})
	if err != nil {
		t.Fatal(err)
	}
	txt := res.Text()
	t.Logf("recognized %q", txt)
	if !strings.Contains(txt, "GoShareIt") || !strings.Contains(txt, "12345") {
		t.Fatalf("text = %q, want GoShareIt and 12345", txt)
	}
	for _, l := range res.Lines {
		for _, w := range l.Words {
			if !w.Rect.In(img.Bounds()) || w.Rect.Empty() {
				t.Errorf("word %q rect %v outside the image", w.Text, w.Rect)
			}
		}
	}
}
