package tesseract

import (
	"context"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// fakeTesseract puts an executable "tesseract" shell script in a temp dir at
// the front of PATH. versionOut and langsOut are what --version and
// --list-langs print; body runs for a recognition call.
func fakeTesseract(t *testing.T, versionOut, langsOut, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes need a POSIX shell")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n" +
		"--version) printf '%s' '" + versionOut + "'; exit 0;;\n" +
		"--list-langs) printf '%s' '" + langsOut + "'; exit 0;;\n" +
		"esac\n" + body
	if err := os.WriteFile(filepath.Join(dir, "tesseract"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

const (
	v534     = "tesseract 5.3.4\n leptonica-1.82.0\n"
	langsOK  = "List of available languages in \"/usr/share/tesseract-ocr/5/tessdata/\" (3):\neng\ndeu\nosd\n"
	langsOSD = "List of available languages in \"/usr/share/tesseract-ocr/5/tessdata/\" (1):\nosd\n"
)

func TestProbeMatrix(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		e := &Engine{Path: filepath.Join(t.TempDir(), "no-such-tesseract")}
		st := e.Probe(context.Background())
		if st.Available || st.Reason != "Tesseract is not installed." || !strings.Contains(st.Hint, "apt install tesseract-ocr") {
			t.Fatalf("status = %+v", st)
		}
	})
	t.Run("too old", func(t *testing.T) {
		fakeTesseract(t, "tesseract 3.05.01\n", langsOK, "")
		st := (&Engine{}).Probe(context.Background())
		if st.Available || !strings.Contains(st.Reason, "3.05.01 is too old") {
			t.Fatalf("status = %+v", st)
		}
	})
	t.Run("only osd", func(t *testing.T) {
		fakeTesseract(t, v534, langsOSD, "")
		st := (&Engine{}).Probe(context.Background())
		if st.Available || st.Reason != "Tesseract has no language data." {
			t.Fatalf("status = %+v", st)
		}
	})
	t.Run("ok", func(t *testing.T) {
		fakeTesseract(t, v534, langsOK, "")
		st := (&Engine{}).Probe(context.Background())
		want := ocr.Status{Available: true, Engine: ocr.EngineTesseract, Version: "5.3.4", Langs: []string{"en", "de"}}
		if !reflect.DeepEqual(st, want) {
			t.Fatalf("status = %+v, want %+v", st, want)
		}
	})
}

func TestProbeNoticesNewInstall(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{Path: filepath.Join(dir, "tesseract")}
	if e.Probe(context.Background()).Available {
		t.Fatal("available before install")
	}
	script := "#!/bin/sh\ncase \"$1\" in\n--version) echo 'tesseract 5.5.0';;\n--list-langs) printf 'List of available languages (1):\\neng\\n';;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "tesseract"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if st := e.Probe(context.Background()); !st.Available || st.Version != "5.5.0" {
		t.Fatalf("after install: %+v", st)
	}
}

func white(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	return img
}

func TestRecognizeParsesFixture(t *testing.T) {
	fixture, err := filepath.Abs("testdata/two_lines.tsv")
	if err != nil {
		t.Fatal(err)
	}
	dir := fakeTesseract(t, v534, langsOK, "echo \"$@\" > \"$(dirname \"$0\")/args\"\ncat > /dev/null\ncat '"+fixture+"'\n")
	e := &Engine{Langs: []string{"de-DE", "xx"}}
	res, err := e.Recognize(context.Background(), white(200, 60), ocr.Options{})
	if err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if got := strings.TrimSpace(string(args)); got != "stdin stdout -l deu --psm 3 --dpi 300 -c preserve_interword_spaces=1 tsv" {
		t.Fatalf("args = %q", got)
	}
	if res.Engine != ocr.EngineTesseract || !reflect.DeepEqual(res.Langs, []string{"de"}) {
		t.Fatalf("engine/langs = %q %v", res.Engine, res.Langs)
	}
	if got := res.Text(); got != "Hello world\nuser@example.com" {
		t.Fatalf("text = %q", got)
	}
	// 200x60 is upscaled 2x and padded 10 px; boxes map back through both.
	l0 := res.Lines[0]
	if l0.Words[0].Rect != image.Rect(10, 10, 60, 30) || l0.Words[1].Rect != image.Rect(70, 10, 130, 30) {
		t.Fatalf("line 0 boxes = %v %v", l0.Words[0].Rect, l0.Words[1].Rect)
	}
	if l0.Rect != image.Rect(10, 10, 130, 30) {
		t.Fatalf("line 0 rect = %v", l0.Rect)
	}
	if w := res.Lines[1].Words; len(w) != 1 || w[0].Rect != image.Rect(10, 35, 110, 50) || w[0].Conf != 0.88 {
		t.Fatalf("line 1 = %+v", w)
	}
}

func TestRecognizeDefaultsToEnglish(t *testing.T) {
	fixture, _ := filepath.Abs("testdata/two_lines.tsv")
	dir := fakeTesseract(t, v534, langsOK, "echo \"$@\" > \"$(dirname \"$0\")/args\"\ncat > /dev/null\ncat '"+fixture+"'\n")
	if _, err := (&Engine{}).Recognize(context.Background(), white(20, 20), ocr.Options{}); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if !strings.Contains(string(args), "-l eng ") {
		t.Fatalf("args = %q", args)
	}
}

func TestRecognizeUnavailable(t *testing.T) {
	e := &Engine{Path: filepath.Join(t.TempDir(), "missing")}
	if _, err := e.Recognize(context.Background(), white(4, 4), ocr.Options{}); !errors.Is(err, ocr.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestRecognizeCancelKillsProcess(t *testing.T) {
	fakeTesseract(t, v534, langsOK, "exec sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	e := &Engine{}
	e.Probe(context.Background()) // warm the cache so the deadline only covers recognition
	start := time.Now()
	_, err := e.Recognize(ctx, white(10, 10), ocr.Options{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("Recognize took %v after cancel", d)
	}
}

func TestRecognizeReportsStderr(t *testing.T) {
	fakeTesseract(t, v534, langsOK, "cat > /dev/null\necho 'Error opening data file' >&2\nexit 1\n")
	_, err := (&Engine{}).Recognize(context.Background(), white(10, 10), ocr.Options{})
	if err == nil || !strings.Contains(err.Error(), "Error opening data file") {
		t.Fatalf("err = %v", err)
	}
}

func TestPreprocess(t *testing.T) {
	// Dark-mode capture: mean luminance below 128 is inverted.
	dark := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for i := 0; i < len(dark.Pix); i += 4 {
		dark.Pix[i], dark.Pix[i+1], dark.Pix[i+2], dark.Pix[i+3] = 0x20, 0x20, 0x20, 0xff
	}
	g := toGray(dark)
	if meanLuma(g) >= 128 {
		t.Fatal("dark image not dark")
	}
	png, tf, err := preprocess(dark)
	if err != nil || len(png) == 0 {
		t.Fatalf("preprocess: %v", err)
	}
	if tf.scale != 2 || tf.pad != padPx {
		t.Fatalf("transform = %+v", tf)
	}
	if chooseScale(image.Rect(0, 0, 2400, 900)) != 1 || chooseScale(image.Rect(0, 0, 1999, 10)) != 2 {
		t.Fatal("scale rule")
	}
	p := pad(g, 10)
	if p.Bounds() != image.Rect(0, 0, 120, 60) || p.GrayAt(0, 0) != (color.Gray{Y: 255}) || p.GrayAt(10, 10).Y != 0x20 {
		t.Fatalf("pad: bounds %v corner %v inner %v", p.Bounds(), p.GrayAt(0, 0), p.GrayAt(10, 10))
	}
	invert(g)
	if g.GrayAt(5, 5).Y != 0xdf {
		t.Fatalf("invert = %v", g.GrayAt(5, 5))
	}
	// Inverse mapping round-trips a box through scale and pad.
	tf = transform{scale: 2, pad: 10}
	if r := tf.back(30, 30, 101, 41); r != image.Rect(10, 10, 61, 31) {
		t.Fatalf("back = %v", r)
	}
}

func TestLangMapping(t *testing.T) {
	cases := map[string]string{"en": "eng", "en-US": "eng", "de": "deu", "zh-Hant": "chi_tra", "zh-TW": "chi_tra", "zh": "chi_sim", "xyz": "xyz"}
	for tag, want := range cases {
		if got := toTesseract(tag); got != want {
			t.Errorf("toTesseract(%q) = %q, want %q", tag, got, want)
		}
	}
	if toBCP47("chi_sim") != "zh-Hans" || toBCP47("frk") != "frk" {
		t.Error("toBCP47")
	}
	if got := pickLangs(nil, []string{"deu", "fra"}); !reflect.DeepEqual(got, []string{"deu"}) {
		t.Errorf("pickLangs without eng = %v", got)
	}
}

func TestParseTSVRejectsGarbage(t *testing.T) {
	if _, err := parseTSV([]byte("not tsv\n"), transform{scale: 1}, image.Rect(0, 0, 10, 10)); !errors.Is(err, errTSV) {
		t.Fatalf("err = %v", err)
	}
}

func TestProbeTimeoutNotCached(t *testing.T) {
	dir := fakeTesseract(t, v534, langsOK, "")
	// A --version that hangs past the probe deadline.
	slow := "#!/bin/sh\ncase \"$1\" in\n--version) exec sleep 30;;\n--list-langs) printf 'List (1):\\neng\\n';;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "tesseract"), []byte(slow), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &Engine{}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if st := e.Probe(ctx); st.Available || st.Reason != "Tesseract did not answer in time." {
		t.Fatalf("slow probe = %+v", st)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("probe did not stop at its deadline")
	}
	// Now fast: the timeout was not remembered.
	fast := "#!/bin/sh\ncase \"$1\" in\n--version) echo 'tesseract 5.3.4';;\n--list-langs) printf 'List (1):\\neng\\n';;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "tesseract"), []byte(fast), 0o755); err != nil {
		t.Fatal(err)
	}
	if st := e.Probe(context.Background()); !st.Available {
		t.Fatalf("probe after timeout = %+v", st)
	}
}
