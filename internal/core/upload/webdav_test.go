package upload

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebDAVUploadNoTemplate(t *testing.T) {
	var (
		putPath string
		putBody string
		putCT   string
		putUser string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		putPath = r.URL.Path
		putCT = r.Header.Get("Content-Type")
		u, _, _ := r.BasicAuth()
		putUser = u
		b, _ := io.ReadAll(r.Body)
		putBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	wd := NewWebDAV(WebDAVConfig{
		BaseURL:  srv.URL,
		Username: "uploader",
		Password: "pw",
	}, srv.Client())

	got, err := wd.Upload(context.Background(), "shot.png", strings.NewReader("PNGDATA"), 7, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if want := "/shot.png"; putPath != want {
		t.Errorf("PUT path = %q, want %q", putPath, want)
	}
	if putBody != "PNGDATA" {
		t.Errorf("PUT body = %q", putBody)
	}
	if putCT != "image/png" {
		t.Errorf("PUT content-type = %q", putCT)
	}
	if putUser != "uploader" {
		t.Errorf("PUT basic-auth user = %q", putUser)
	}
	want := srv.URL + "/shot.png"
	if got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
	if got.DirectURL != want {
		t.Errorf("DirectURL = %q, want %q", got.DirectURL, want)
	}
	if got.ShareToken != "" {
		t.Errorf("ShareToken = %q, want empty", got.ShareToken)
	}
}

func TestWebDAVUploadRemoteDirAndURLTemplate(t *testing.T) {
	var putPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		putPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	wd := NewWebDAV(WebDAVConfig{
		BaseURL:     srv.URL + "/", // trailing slash should be normalized
		Username:    "uploader",
		Password:    "pw",
		RemoteDir:   "/Screenshots/",
		URLTemplate: "https://cdn.example.com/{path}",
	}, srv.Client())

	got, err := wd.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if want := "/Screenshots/a.png"; putPath != want {
		t.Errorf("PUT path = %q, want %q", putPath, want)
	}
	if want := "https://cdn.example.com/Screenshots/a.png"; got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
	if got.DirectURL != got.PublicURL {
		t.Errorf("DirectURL = %q, want equal to PublicURL", got.DirectURL)
	}
}

func TestWebDAVUploadErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, "parent collection does not exist")
	}))
	defer srv.Close()

	wd := NewWebDAV(WebDAVConfig{BaseURL: srv.URL, Username: "u", Password: "pw"}, srv.Client())
	if _, err := wd.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png"); err == nil {
		t.Fatal("expected error on 409 response")
	}
}
