package edit

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

func TestNoopEditorPassthrough(t *testing.T) {
	in := capture.Result{Bytes: []byte("hello"), Mime: "image/png", Kind: capture.KindImage}
	out, ok, err := NoopEditor{}.Edit(context.Background(), in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if !bytes.Equal(out.Bytes, in.Bytes) || out.Mime != in.Mime {
		t.Errorf("out = %+v, want input unchanged", out)
	}
}

// writeStub writes an executable shell-script helper to dir and returns its path.
func writeStub(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

const argParse = `#!/bin/sh
in=""
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    --in) in="$2"; shift 2;;
    --out) out="$2"; shift 2;;
    *) shift;;
  esac
done
`

func TestLauncherConfirm(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "confirm.sh", argParse+`printf 'EDITED' > "$out"
exit 0
`)
	l := Launcher{HelperPath: helper}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	out, ok, err := l.Edit(context.Background(), in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if string(out.Bytes) != "EDITED" {
		t.Errorf("out.Bytes = %q, want EDITED", out.Bytes)
	}
	if out.Mime != "image/png" || out.Kind != capture.KindImage || out.Path != "" {
		t.Errorf("out metadata = %+v", out)
	}
}

func TestLauncherPassesThemeAndConfirmLabel(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	body := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n" + argParse + `printf 'EDITED' > "$out"
exit 0
`
	helper := writeStub(t, dir, "flags.sh", body)
	l := Launcher{HelperPath: helper, Theme: "dark", ConfirmLabel: "Copy & Upload"}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	if _, _, err := l.Edit(context.Background(), in); err != nil {
		t.Fatalf("err = %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(got)
	if !strings.Contains(args, "--theme dark") {
		t.Errorf("args = %q, want --theme dark", args)
	}
	if !strings.Contains(args, "--confirm-label Copy & Upload") {
		t.Errorf("args = %q, want --confirm-label Copy & Upload", args)
	}
}

func TestLauncherOmitsThemeAndConfirmLabelWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	body := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n" + argParse + `printf 'EDITED' > "$out"
exit 0
`
	helper := writeStub(t, dir, "flags.sh", body)
	l := Launcher{HelperPath: helper}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	if _, _, err := l.Edit(context.Background(), in); err != nil {
		t.Fatalf("err = %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(got)
	if strings.Contains(args, "--theme") || strings.Contains(args, "--confirm-label") {
		t.Errorf("args = %q, want no --theme/--confirm-label", args)
	}
}

func TestLauncherCancel(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "cancel.sh", argParse+"exit 64\n")
	l := Launcher{HelperPath: helper}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	out, ok, err := l.Edit(context.Background(), in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if string(out.Bytes) != "ORIGINAL" {
		t.Errorf("out.Bytes = %q, want ORIGINAL", out.Bytes)
	}
}

func TestLauncherError(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "err.sh", argParse+"exit 1\n")
	l := Launcher{HelperPath: helper}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	out, ok, err := l.Edit(context.Background(), in)
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if string(out.Bytes) != "ORIGINAL" {
		t.Errorf("out.Bytes = %q, want ORIGINAL", out.Bytes)
	}
}

func TestLauncherMissingHelper(t *testing.T) {
	l := Launcher{HelperPath: filepath.Join(t.TempDir(), "does-not-exist")}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	out, ok, err := l.Edit(context.Background(), in)
	if err == nil {
		t.Fatalf("err = nil, want error for missing helper")
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if string(out.Bytes) != "ORIGINAL" {
		t.Errorf("out.Bytes = %q, want ORIGINAL", out.Bytes)
	}
}

func TestLauncherVideoUnchanged(t *testing.T) {
	// HelperPath points nowhere; video must short-circuit before exec.
	l := Launcher{HelperPath: filepath.Join(t.TempDir(), "nope")}
	in := capture.Result{Bytes: []byte("VID"), Mime: "video/mp4", Kind: capture.KindVideo}
	out, ok, err := l.Edit(context.Background(), in)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if string(out.Bytes) != "VID" {
		t.Errorf("out.Bytes = %q, want VID", out.Bytes)
	}
}
