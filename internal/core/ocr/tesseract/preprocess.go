package tesseract

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	xdraw "golang.org/x/image/draw"
)

// Preprocessing constants from the tesseract ImproveQuality guidance.
const (
	padPx        = 10   // white border around the text
	upscaleBelow = 2000 // long side under this is upscaled 2x
)

// transform records how the preprocessed image relates to the source, so
// word boxes can be mapped back.
type transform struct {
	scale float64
	pad   int
}

// preprocess converts img to the image tesseract reads best: grayscale, dark
// text on a light background (dark-mode captures are inverted), upscaled 2x
// when small, with a white border. It returns the PNG bytes and the
// transform for mapping boxes back.
func preprocess(img image.Image) ([]byte, transform, error) {
	gray := toGray(img)
	if meanLuma(gray) < 128 {
		invert(gray)
	}
	scale := chooseScale(gray.Bounds())
	src := gray
	if scale != 1 {
		b := gray.Bounds()
		up := image.NewGray(image.Rect(0, 0, int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)))
		xdraw.CatmullRom.Scale(up, up.Bounds(), gray, b, xdraw.Src, nil)
		src = up
	}
	padded := pad(src, padPx)
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, padded); err != nil {
		return nil, transform{}, err
	}
	return buf.Bytes(), transform{scale: scale, pad: padPx}, nil
}

// toGray draws img into a zero-origin *image.Gray; the Gray color model does
// the luminance conversion.
func toGray(img image.Image) *image.Gray {
	b := img.Bounds()
	g := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(g, g.Bounds(), img, b.Min, draw.Src)
	return g
}

func meanLuma(g *image.Gray) float64 {
	b := g.Bounds()
	if b.Empty() {
		return 255
	}
	var sum uint64
	for y := 0; y < b.Dy(); y++ {
		row := g.Pix[y*g.Stride : y*g.Stride+b.Dx()]
		for _, v := range row {
			sum += uint64(v)
		}
	}
	return float64(sum) / float64(b.Dx()*b.Dy())
}

func invert(g *image.Gray) {
	for i, v := range g.Pix {
		g.Pix[i] = 255 - v
	}
}

// chooseScale upscales 2x when the long side is under upscaleBelow, so the
// result stays under 4000 px; larger captures are used as they are.
func chooseScale(b image.Rectangle) float64 {
	if max(b.Dx(), b.Dy()) < upscaleBelow {
		return 2
	}
	return 1
}

// pad returns g centred on a white canvas with n px on every side.
func pad(g *image.Gray, n int) *image.Gray {
	b := g.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx()+2*n, b.Dy()+2*n))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.Gray{Y: 255}), image.Point{}, draw.Src)
	draw.Draw(out, image.Rect(n, n, n+b.Dx(), n+b.Dy()), g, b.Min, draw.Src)
	return out
}
