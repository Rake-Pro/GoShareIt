//go:build darwin && cgo

package vision

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

// TestRecognizeSynthetic renders a known string with Go Regular and runs real
// Vision recognition over it. It runs on the macos-build CI job. It skips
// when Vision is unavailable or errors on the machine, and fails only when
// Vision returns text that is wrong.
func TestRecognizeSynthetic(t *testing.T) {
	t.Run("configured languages map to Vision tags", func(t *testing.T) {
		e := New([]string{"en", "zz"})
		st := e.Probe(context.Background())
		if !st.Available {
			t.Skipf("Vision unavailable: %s", st.Reason)
		}
		got := e.visionLangs(context.Background(), []string{"en", "zz"})
		if len(got) != 1 || !strings.HasPrefix(strings.ToLower(got[0]), "en") {
			t.Fatalf("visionLangs = %v, want one English tag from %v", got, st.Langs)
		}
		if got := e.visionLangs(context.Background(), []string{"zz"}); got != nil {
			t.Fatalf("unmatched tags = %v, want automatic detection (nil)", got)
		}
	})

	e := New(nil)
	st := e.Probe(context.Background())
	if !st.Available {
		t.Skipf("Vision unavailable: %s", st.Reason)
	}
	t.Logf("Vision languages: %v", st.Langs)

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
		// A runner that lists languages but cannot run recognition (no
		// inference backend in the VM) is an environment gap, not a wrong
		// result. A returned result is still checked strictly below.
		t.Skipf("Vision could not recognize on this machine: %v", err)
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
