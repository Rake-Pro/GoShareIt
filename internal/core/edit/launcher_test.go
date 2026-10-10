package edit

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

func TestNoopEditorPassthrough(t *testing.T) {
	in := capture.Result{Bytes: []byte("hello"), Mime: "image/png", Kind: capture.KindImage}
	out, action, ok, err := NoopEditor{}.Edit(context.Background(), in, Opts{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if action != ActionDefault {
		t.Errorf("action = %v, want ActionDefault", action)
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
	out, action, ok, err := l.Edit(context.Background(), in, Opts{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if action != ActionDefault {
		t.Errorf("action = %v, want ActionDefault", action)
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
	if _, _, _, err := l.Edit(context.Background(), in, Opts{}); err != nil {
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
	if _, _, _, err := l.Edit(context.Background(), in, Opts{}); err != nil {
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

// TestLauncherPassesActionsAndUploadEnabled asserts --actions is always
// present and --upload-enabled reflects Opts.CanUpload.
func TestLauncherPassesActionsAndUploadEnabled(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	body := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n" + argParse + `printf 'EDITED' > "$out"
exit 0
`
	helper := writeStub(t, dir, "flags.sh", body)
	l := Launcher{HelperPath: helper}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	if _, _, _, err := l.Edit(context.Background(), in, Opts{CanUpload: false}); err != nil {
		t.Fatalf("err = %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(got)
	if !strings.Contains(args, "--actions") {
		t.Errorf("args = %q, want --actions", args)
	}
	if !strings.Contains(args, "--upload-enabled=false") {
		t.Errorf("args = %q, want --upload-enabled=false", args)
	}
}

func TestLauncherCancel(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "cancel.sh", argParse+"exit 64\n")
	l := Launcher{HelperPath: helper}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	out, action, ok, err := l.Edit(context.Background(), in, Opts{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if action != ActionDefault {
		t.Errorf("action = %v, want ActionDefault", action)
	}
	if string(out.Bytes) != "ORIGINAL" {
		t.Errorf("out.Bytes = %q, want ORIGINAL", out.Bytes)
	}
}

func TestLauncherActionButtons(t *testing.T) {
	cases := []struct {
		name string
		code string
		want Action
	}{
		{"copy", "65", ActionCopy},
		{"save", "66", ActionSave},
		{"upload", "67", ActionUpload},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			helper := writeStub(t, dir, c.name+".sh", argParse+`printf 'EDITED' > "$out"
exit `+c.code+`
`)
			l := Launcher{HelperPath: helper}
			in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
			out, action, ok, err := l.Edit(context.Background(), in, Opts{CanUpload: true})
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !ok {
				t.Fatalf("ok = false, want true")
			}
			if action != c.want {
				t.Errorf("action = %v, want %v", action, c.want)
			}
			if string(out.Bytes) != "EDITED" {
				t.Errorf("out.Bytes = %q, want EDITED", out.Bytes)
			}
		})
	}
}

func TestLauncherError(t *testing.T) {
	dir := t.TempDir()
	helper := writeStub(t, dir, "err.sh", argParse+"exit 1\n")
	l := Launcher{HelperPath: helper}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	out, action, ok, err := l.Edit(context.Background(), in, Opts{})
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if action != ActionDefault {
		t.Errorf("action = %v, want ActionDefault", action)
	}
	if string(out.Bytes) != "ORIGINAL" {
		t.Errorf("out.Bytes = %q, want ORIGINAL", out.Bytes)
	}
}

func TestLauncherMissingHelper(t *testing.T) {
	l := Launcher{HelperPath: filepath.Join(t.TempDir(), "does-not-exist")}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	out, action, ok, err := l.Edit(context.Background(), in, Opts{})
	if err == nil {
		t.Fatalf("err = nil, want error for missing helper")
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if action != ActionDefault {
		t.Errorf("action = %v, want ActionDefault", action)
	}
	if string(out.Bytes) != "ORIGINAL" {
		t.Errorf("out.Bytes = %q, want ORIGINAL", out.Bytes)
	}
}

func TestLauncherVideoUnchanged(t *testing.T) {
	// HelperPath points nowhere; video must short-circuit before exec.
	l := Launcher{HelperPath: filepath.Join(t.TempDir(), "nope")}
	in := capture.Result{Bytes: []byte("VID"), Mime: "video/mp4", Kind: capture.KindVideo}
	out, action, ok, err := l.Edit(context.Background(), in, Opts{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false")
	}
	if action != ActionDefault {
		t.Errorf("action = %v, want ActionDefault", action)
	}
	if string(out.Bytes) != "VID" {
		t.Errorf("out.Bytes = %q, want VID", out.Bytes)
	}
}

// ConfirmLabelFor is evaluated per launch with the live upload state, so a
// runtime upload toggle shows on the confirm button.
func TestLauncherConfirmLabelForUsesLiveUploadState(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	body := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n" + argParse + `printf 'EDITED' > "$out"
exit 0
`
	helper := writeStub(t, dir, "label.sh", body)
	l := Launcher{HelperPath: helper, ConfirmLabel: "stale", ConfirmLabelFor: func(up bool) string {
		if up {
			return "Upload"
		}
		return "Copy"
	}}
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	for up, want := range map[bool]string{true: "--confirm-label Upload", false: "--confirm-label Copy"} {
		if _, _, _, err := l.Edit(context.Background(), in, Opts{CanUpload: up}); err != nil {
			t.Fatalf("err = %v", err)
		}
		got, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatalf("read args: %v", err)
		}
		if !strings.Contains(string(got), want) {
			t.Errorf("CanUpload=%v: args = %q, want %q", up, got, want)
		}
	}
}

// launchArgs runs l against a stub helper that records its arguments and
// returns them.
func launchArgs(t *testing.T, l Launcher, opts Opts) string {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	body := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n" + argParse + `printf 'EDITED' > "$out"
exit 0
`
	l.HelperPath = writeStub(t, dir, "flags.sh", body)
	in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
	if _, _, _, err := l.Edit(context.Background(), in, opts); err != nil {
		t.Fatalf("err = %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	return string(got)
}

func TestLauncherOCRFlags(t *testing.T) {
	avail := Opts{OCR: ocr.Status{Available: true, Engine: ocr.EngineTesseract}}
	args := launchArgs(t, Launcher{}, avail)
	if !strings.Contains(args, "--ocr=hidden") || strings.Contains(args, "--ocr-auto") {
		t.Errorf("disabled: args = %q, want --ocr=hidden only", args)
	}
	if !strings.Contains(args, "--text-out ") {
		t.Errorf("args = %q, want --text-out", args)
	}

	l := Launcher{OCREnabled: true, OCRAutoRun: true, OCRLangs: []string{"en", "de"}, QuickRedact: []string{"email", "ip"}, OCRTimeout: 20 * time.Second, TesseractPath: "/opt/tess"}
	args = launchArgs(t, l, avail)
	for _, want := range []string{"--ocr=on", "--ocr-auto=true", "--ocr-langs en,de", "--ocr-quick-redact email,ip", "--ocr-timeout 20", "--ocr-tesseract /opt/tess"} {
		if !strings.Contains(args, want) {
			t.Errorf("enabled: args = %q, want %q", args, want)
		}
	}

	off := Opts{OCR: ocr.Status{Reason: "Tesseract is not installed.", Hint: "Install it."}}
	args = launchArgs(t, Launcher{OCREnabled: true, OCRAutoRun: true}, off)
	if !strings.Contains(args, "--ocr=off --ocr-reason Tesseract is not installed. Install it.") || strings.Contains(args, "--ocr-auto") {
		t.Errorf("unavailable: args = %q", args)
	}
}

func TestLauncherHandsOffCopiedText(t *testing.T) {
	cases := []struct {
		name  string
		write string // shell line run by the helper; "" writes nothing
		code  int
		want  []string
	}{
		{"text copied, confirm", `printf 'hello world' > "$text"`, 0, []string{"hello world"}},
		{"text copied, cancel", `printf 'kept' > "$text"`, 64, []string{"kept"}},
		{"empty file", `: > "$text"`, 0, nil},
		{"no file", "", 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			body := "#!/bin/sh\ntext=\"\"\nout=\"\"\nwhile [ $# -gt 0 ]; do case \"$1\" in --text-out) text=\"$2\"; shift 2;; --out) out=\"$2\"; shift 2;; *) shift;; esac; done\n" +
				c.write + "\nprintf 'EDITED' > \"$out\"\nexit " + strconv.Itoa(c.code) + "\n"
			var got []string
			l := Launcher{HelperPath: writeStub(t, dir, "text.sh", body), OnText: func(s string) { got = append(got, s) }}
			in := capture.Result{Bytes: []byte("ORIGINAL"), Mime: "image/png", Kind: capture.KindImage}
			if _, _, _, err := l.Edit(context.Background(), in, Opts{}); err != nil {
				t.Fatalf("err = %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("OnText calls = %q, want %q", got, c.want)
			}
		})
	}
}
