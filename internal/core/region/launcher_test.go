package region

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"
)

// writeStub writes an executable shell-script helper to dir and returns its path.
func writeStub(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// argParse extracts --out into $out, ignoring the --region flag.
const argParse = `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    --out) out="$2"; shift 2;;
    --region) shift;;
    *) shift;;
  esac
done
`

func TestLauncherConfirm(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "confirm.sh", argParse+`printf '10,20,30,40' > "$out"
exit 0
`)
	l := Launcher{HelperPath: helper}
	rect, ok, err := l.Select(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	want := image.Rect(10, 20, 40, 60)
	if rect != want {
		t.Errorf("rect = %v, want %v", rect, want)
	}
}

func TestLauncherConfirmTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "confirm_nl.sh", argParse+`printf '0,0,1920,1080\n' > "$out"
exit 0
`)
	l := Launcher{HelperPath: helper}
	rect, ok, err := l.Select(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if want := image.Rect(0, 0, 1920, 1080); rect != want {
		t.Errorf("rect = %v, want %v", rect, want)
	}
}

func TestLauncherCancel(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "cancel.sh", argParse+"exit 64\n")
	l := Launcher{HelperPath: helper}
	rect, ok, err := l.Select(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if !rect.Empty() {
		t.Errorf("rect = %v, want empty", rect)
	}
}

func TestLauncherError(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "err.sh", argParse+"exit 1\n")
	l := Launcher{HelperPath: helper}
	_, ok, err := l.Select(context.Background())
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
}

func TestLauncherBadOutput(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "bad.sh", argParse+`printf 'not,a,rect' > "$out"
exit 0
`)
	l := Launcher{HelperPath: helper}
	_, ok, err := l.Select(context.Background())
	if err == nil {
		t.Fatalf("err = nil, want parse error")
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
}

func TestLauncherMissingHelper(t *testing.T) {
	l := Launcher{HelperPath: filepath.Join(t.TempDir(), "does-not-exist")}
	_, ok, err := l.Select(context.Background())
	if err == nil {
		t.Fatalf("err = nil, want error for missing helper")
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
}

func TestParseRect(t *testing.T) {
	cases := []struct {
		in      string
		want    image.Rectangle
		wantErr bool
	}{
		{"1,2,3,4", image.Rect(1, 2, 4, 6), false},
		{" 5, 6, 7, 8 ", image.Rect(5, 6, 12, 14), false},
		{"0,0,0,10", image.Rectangle{}, true},
		{"0,0,10,0", image.Rectangle{}, true},
		{"1,2,3", image.Rectangle{}, true},
		{"a,b,c,d", image.Rectangle{}, true},
	}
	for _, c := range cases {
		got, err := parseRect(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseRect(%q) err = nil, want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseRect(%q) err = %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseRect(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
