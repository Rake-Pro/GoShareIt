//go:build linux

package linux

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// fakeFFmpeg puts an "ffmpeg" on PATH that runs script with the output path
// (ffmpeg's last argument) in $out, and points the temp dir at a fresh one.
func fakeFFmpeg(t *testing.T, script string) string {
	t.Helper()
	bin := t.TempDir()
	body := "#!/bin/sh\nfor a; do out=$a; done\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(bin, "ffmpeg"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	return tmp
}

// The recording is written into a private directory, never straight into the
// shared temp dir, and that directory is gone after Stop.
func TestRecorderUsesPrivateTempDir(t *testing.T) {
	tmp := fakeFFmpeg(t, `printf MP4DATA > "$out"; read line; exit 0`)
	r := &Recorder{}
	if err := r.Start(context.Background(), capture.VideoFull); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(r.outPath)
	if filepath.Dir(dir) != tmp {
		t.Errorf("recording dir %s is not inside the temp dir %s", dir, tmp)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("recording dir mode = %o, want 700", fi.Mode().Perm())
	}
	res, err := r.Stop(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Bytes) != "MP4DATA" {
		t.Errorf("bytes = %q", res.Bytes)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("recording dir left behind: %v", err)
	}
}

func TestRecorderCleansUpOnFailure(t *testing.T) {
	tmp := fakeFFmpeg(t, `printf PARTIAL > "$out"; exit 3`)
	r := &Recorder{}
	if err := r.Start(context.Background(), capture.VideoFull); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Stop(context.Background()); err == nil {
		t.Fatal("expected an error from a failed ffmpeg")
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("temp dir not cleaned up: %v", entries)
	}
}
