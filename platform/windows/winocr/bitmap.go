package winocr

import (
	"image"
	"math"

	xdraw "golang.org/x/image/draw"
)

// upscaleBelow is the long-side limit under which images are upscaled 2x:
// Windows OCR is tuned for photos and misses small UI text otherwise (the
// same default ShareX uses).
const upscaleBelow = 1200

// chooseScale returns the factor applied before recognition: 2x when the
// image is small and the doubled size still fits maxDim, a downscale to fit
// maxDim when it is too large, else 1. maxDim is OcrEngine.MaxImageDimension
// (0 = unknown, treated as no limit).
func chooseScale(b image.Rectangle, maxDim uint32) float64 {
	long := max(b.Dx(), b.Dy())
	if long == 0 {
		return 1
	}
	limit := int(maxDim)
	if maxDim == 0 {
		limit = math.MaxInt32
	}
	switch {
	case long < upscaleBelow && 2*long <= limit:
		return 2
	case long > limit:
		return float64(limit) / float64(long)
	}
	return 1
}

// toBGRA8Premultiplied returns img scaled by scale as tightly packed BGRA8
// premultiplied pixels (the layout SoftwareBitmap.CreateCopyFromBuffer
// expects for BitmapPixelFormatBgra8) with its width and height.
func toBGRA8Premultiplied(img image.Image, scale float64) (pix []byte, w, h int) {
	b := img.Bounds()
	w = max(1, int(math.Round(float64(b.Dx())*scale)))
	h = max(1, int(math.Round(float64(b.Dy())*scale)))
	rgba := image.NewRGBA(image.Rect(0, 0, w, h)) // RGBA is premultiplied
	if w == b.Dx() && h == b.Dy() {
		xdraw.Draw(rgba, rgba.Rect, img, b.Min, xdraw.Src)
	} else {
		xdraw.CatmullRom.Scale(rgba, rgba.Rect, img, b, xdraw.Src, nil)
	}
	pix = rgba.Pix
	for i := 0; i+3 < len(pix); i += 4 {
		pix[i], pix[i+2] = pix[i+2], pix[i]
	}
	return pix, w, h
}

// mapBack divides a box in scaled-image pixels by scale and rounds it
// outward into the source image space (origin off).
func mapBack(x, y, w, h float32, scale float64, off image.Point) image.Rectangle {
	if scale <= 0 {
		scale = 1
	}
	return image.Rect(
		int(math.Floor(float64(x)/scale)), int(math.Floor(float64(y)/scale)),
		int(math.Ceil(float64(x+w)/scale)), int(math.Ceil(float64(y+h)/scale)),
	).Add(off)
}
