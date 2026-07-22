package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSemverGreater(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"1.0.0", "1.0.0", false},
		{"2.0.0", "1.9.9", true},
		{"1.10.0", "1.9.0", true},
		{"1.0.0", "0.0.0-dev", true},
		{"1.2.3", "1.2.3-dev", true},
		{"1.2.3-rc1", "1.2.3", false},
	}
	for _, c := range cases {
		got, err := semverGreater(c.a, c.b)
		if err != nil {
			t.Fatalf("semverGreater(%q,%q): %v", c.a, c.b, err)
		}
		if got != c.want {
			t.Errorf("semverGreater(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	if _, err := semverGreater("1.2", "1.0.0"); err == nil {
		t.Error("expected error for malformed version")
	}
}

func TestChecksumFor(t *testing.T) {
	sums := "abc\n" +
		"0123456789012345678901234567890123456789012345678901234567890123  GoShareIt_1.0.0_linux_amd64.tar.gz\n" +
		"ABCDEF6789012345678901234567890123456789012345678901234567890123  other.zip\n"
	got, err := checksumFor(sums, "GoShareIt_1.0.0_linux_amd64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0123456789012345678901234567890123456789012345678901234567890123" {
		t.Errorf("wrong checksum: %s", got)
	}
	if got, err := checksumFor(sums, "other.zip"); err != nil || got != "abcdef6789012345678901234567890123456789012345678901234567890123" {
		t.Errorf("expected lowercased checksum, got %q err %v", got, err)
	}
	if _, err := checksumFor(sums, "missing.zip"); err == nil {
		t.Error("expected error for missing entry")
	}
}

// serveRelease stands up a fake GitHub API with one release and its assets.
func serveRelease(t *testing.T, tag string, assets map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	names := make([]string, 0, len(assets))
	for n := range assets {
		names = append(names, n)
	}
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[`, tag)
		for i, n := range names {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"id":%d,"name":%q,"size":%d}`, i+1, n, len(assets[n]))
		}
		fmt.Fprint(w, `]}`)
	})
	mux.HandleFunc("/repos/o/r/releases/assets/", func(w http.ResponseWriter, r *http.Request) {
		var id int
		fmt.Sscanf(filepath.Base(r.URL.Path), "%d", &id)
		if id < 1 || id > len(names) {
			http.NotFound(w, r)
			return
		}
		w.Write(assets[names[id-1]])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckAndDownload(t *testing.T) {
	payload := []byte("new binary bytes")
	sum := sha256.Sum256(payload)
	name := AssetName("1.1.0")
	sums := hex.EncodeToString(sum[:]) + "  " + name + "\n"
	srv := serveRelease(t, "v1.1.0", map[string][]byte{
		name:            payload,
		"checksums.txt": []byte(sums),
	})

	u, err := New(Config{Repo: "o/r", Current: "1.0.0", APIBaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "1.1.0" {
		t.Fatalf("expected release 1.1.0, got %+v", rel)
	}

	path, err := u.Download(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("downloaded payload mismatch")
	}
}

func TestCheckUpToDate(t *testing.T) {
	srv := serveRelease(t, "v1.0.0", nil)
	u, err := New(Config{Repo: "o/r", Current: "1.0.0", APIBaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel != nil {
		t.Fatalf("expected up-to-date, got %+v", rel)
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	name := AssetName("1.1.0")
	srv := serveRelease(t, "v1.1.0", map[string][]byte{
		name: []byte("payload"),
		"checksums.txt": []byte(
			"0000000000000000000000000000000000000000000000000000000000000000  " + name + "\n"),
	})
	u, err := New(Config{Repo: "o/r", Current: "1.0.0", APIBaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Download(context.Background(), rel); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestExtractTarGzFlattens(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte("#!/bin/sh\necho hi\n")
	if err := tw.WriteHeader(&tar.Header{Name: "nested/dir/goshareit", Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	tw.Write(content)
	tw.Close()
	gz.Close()

	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := extract(archive, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "goshareit"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Error("extracted content mismatch")
	}
	fi, _ := os.Stat(filepath.Join(dir, "goshareit"))
	if fi.Mode().Perm()&0o100 == 0 {
		t.Error("extracted file is not executable")
	}
}
