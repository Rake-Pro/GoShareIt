package annotate

import (
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestVisualSample is a dev harness, skipped unless GOSHAREIT_BLUR_SAMPLE is
// set: it blurs a rectangle of a real screenshot with the production code so
// blur strength can be eyeballed. GOSHAREIT_BLUR_RECT is "x0,y0,x1,y1"
// (default 0,400,1150,460), output goes to GOSHAREIT_BLUR_OUT.
//
//	GOSHAREIT_BLUR_SAMPLE=in.png GOSHAREIT_BLUR_OUT=out.png \
//	  go test ./internal/editor/annotate/ -run TestVisualSample
func TestVisualSample(t *testing.T) {
	src := os.Getenv("GOSHAREIT_BLUR_SAMPLE")
	if src == "" {
		t.Skip("GOSHAREIT_BLUR_SAMPLE not set")
	}
	rect := image.Rect(0, 400, 1150, 460)
	if r := os.Getenv("GOSHAREIT_BLUR_RECT"); r != "" {
		parts := strings.Split(r, ",")
		if len(parts) == 4 {
			var v [4]int
			for i, p := range parts {
				v[i], _ = strconv.Atoi(strings.TrimSpace(p))
			}
			rect = image.Rect(v[0], v[1], v[2], v[3])
		}
	}
	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(img, nil, []Shape{BlurRegion{Rect: rect, Radius: 3}})
	if err != nil {
		t.Fatal(err)
	}
	o, err := os.Create(os.Getenv("GOSHAREIT_BLUR_OUT"))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := png.Encode(o, out); err != nil {
		t.Fatal(err)
	}
}
