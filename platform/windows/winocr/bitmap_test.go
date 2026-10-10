package winocr

import (
	"image"
	"image/color"
	"testing"
)

func TestChooseScale(t *testing.T) {
	cases := []struct {
		b      image.Rectangle
		maxDim uint32
		want   float64
	}{
		{image.Rect(0, 0, 800, 600), 10000, 2},
		{image.Rect(0, 0, 800, 600), 1000, 1},   // doubled would not fit
		{image.Rect(0, 0, 1600, 900), 10000, 1}, // big enough already
		{image.Rect(0, 0, 4000, 1000), 2600, 0.65},
		{image.Rect(0, 0, 100, 50), 0, 2}, // unknown limit
	}
	for _, c := range cases {
		if got := chooseScale(c.b, c.maxDim); got != c.want {
			t.Errorf("chooseScale(%v, %d) = %v, want %v", c.b, c.maxDim, got, c.want)
		}
	}
}

func TestToBGRA8Premultiplied(t *testing.T) {
	img := image.NewNRGBA(image.Rect(5, 5, 7, 6))
	img.SetNRGBA(5, 5, color.NRGBA{R: 200, G: 100, B: 50, A: 255})
	img.SetNRGBA(6, 5, color.NRGBA{R: 200, G: 100, B: 0, A: 128}) // half transparent
	pix, w, h := toBGRA8Premultiplied(img, 1)
	if w != 2 || h != 1 || len(pix) != 8 {
		t.Fatalf("size = %dx%d, %d bytes", w, h, len(pix))
	}
	if got := pix[:4]; got[0] != 50 || got[1] != 100 || got[2] != 200 || got[3] != 255 {
		t.Fatalf("opaque pixel BGRA = %v", got)
	}
	// Premultiplied: 200*128/255 ~ 100, 100*128/255 ~ 50.
	if got := pix[4:]; got[0] != 0 || got[1] < 49 || got[1] > 51 || got[2] < 99 || got[2] > 101 || got[3] != 128 {
		t.Fatalf("translucent pixel BGRA = %v", got)
	}
	_, w, h = toBGRA8Premultiplied(image.NewRGBA(image.Rect(0, 0, 30, 20)), 2)
	if w != 60 || h != 40 {
		t.Fatalf("scaled size = %dx%d", w, h)
	}
}

func TestMapBack(t *testing.T) {
	if r := mapBack(21, 11, 40, 20.5, 2, image.Pt(100, 0)); r != image.Rect(110, 5, 131, 16) {
		t.Fatalf("mapBack = %v", r)
	}
}
