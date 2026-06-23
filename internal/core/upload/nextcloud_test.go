package upload

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNextcloudUpload(t *testing.T) {
	var (
		putPath      string
		putBody      string
		putCT        string
		putUser      string
		shareForm    string
		shareHeaders http.Header
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut:
			putPath = r.URL.Path
			putCT = r.Header.Get("Content-Type")
			u, _, _ := r.BasicAuth()
			putUser = u
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "files_sharing"):
			shareHeaders = r.Header.Clone()
			b, _ := io.ReadAll(r.Body)
			shareForm = string(b)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `{"ocs":{"meta":{"status":"ok","statuscode":200,"message":"OK"},"data":{"token":"abc123"}}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	nc := NewNextcloud(NextcloudConfig{
		BaseURL:   srv.URL,
		Username:  "imgshare@rake.pro",
		Password:  "app-pw",
		RemoteDir: "",
	}, srv.Client())

	got, err := nc.Upload(context.Background(), "shot.png", strings.NewReader("PNGDATA"), 7, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if want := "/remote.php/dav/files/imgshare/shot.png"; putPath != want {
		t.Errorf("PUT path = %q, want %q", putPath, want)
	}
	if putBody != "PNGDATA" {
		t.Errorf("PUT body = %q", putBody)
	}
	if putCT != "image/png" {
		t.Errorf("PUT content-type = %q", putCT)
	}
	if putUser != "imgshare@rake.pro" {
		t.Errorf("PUT basic-auth user = %q, want full username", putUser)
	}

	if shareHeaders.Get("OCS-APIRequest") != "true" {
		t.Errorf("OCS-APIRequest header = %q", shareHeaders.Get("OCS-APIRequest"))
	}
	if shareHeaders.Get("Accept") != "application/json" {
		t.Errorf("Accept header = %q", shareHeaders.Get("Accept"))
	}
	if !strings.Contains(shareForm, "path=%2Fshot.png") {
		t.Errorf("share form missing path: %q", shareForm)
	}
	if !strings.Contains(shareForm, "shareType=3") {
		t.Errorf("share form missing shareType: %q", shareForm)
	}
	if !strings.Contains(shareForm, "permissions=1") {
		t.Errorf("share form missing permissions: %q", shareForm)
	}

	if got.ShareToken != "abc123" {
		t.Errorf("token = %q", got.ShareToken)
	}
	if want := srv.URL + "/s/abc123/preview"; got.DirectURL != want {
		t.Errorf("DirectURL = %q, want %q", got.DirectURL, want)
	}
	if want := srv.URL + "/s/abc123"; got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
}

func TestNextcloudRemoteDirAndDavUser(t *testing.T) {
	var putPath, shareForm string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putPath = r.URL.Path
			w.WriteHeader(http.StatusCreated)
			return
		}
		b, _ := io.ReadAll(r.Body)
		shareForm = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ocs":{"meta":{"statuscode":200},"data":{"token":"t"}}}`)
	}))
	defer srv.Close()

	nc := NewNextcloud(NextcloudConfig{
		BaseURL:   srv.URL + "/", // trailing slash should be normalized
		Username:  "imgshare@rake.pro",
		Password:  "pw",
		RemoteDir: "/Screenshots/",
	}, srv.Client())

	if _, err := nc.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png"); err != nil {
		t.Fatal(err)
	}
	if want := "/remote.php/dav/files/imgshare/Screenshots/a.png"; putPath != want {
		t.Errorf("PUT path = %q, want %q", putPath, want)
	}
	if !strings.Contains(shareForm, "path=%2FScreenshots%2Fa.png") {
		t.Errorf("share path = %q", shareForm)
	}
}

func TestNextcloudExpireDate(t *testing.T) {
	orig := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = orig }()

	var shareForm string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusCreated)
			return
		}
		b, _ := io.ReadAll(r.Body)
		shareForm = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ocs":{"meta":{"statuscode":200},"data":{"token":"t"}}}`)
	}))
	defer srv.Close()

	nc := NewNextcloud(NextcloudConfig{
		BaseURL:         srv.URL,
		Username:        "imgshare@rake.pro",
		Password:        "pw",
		ShareExpireDays: 7,
		SharePassword:   "secret",
	}, srv.Client())

	if _, err := nc.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(shareForm, "expireDate=2026-01-08") {
		t.Errorf("expireDate missing/wrong: %q", shareForm)
	}
	if !strings.Contains(shareForm, "password=secret") {
		t.Errorf("share password missing: %q", shareForm)
	}
}

func TestNextcloudOCSError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ocs":{"meta":{"statuscode":404,"message":"not found"},"data":{}}}`)
	}))
	defer srv.Close()

	nc := NewNextcloud(NextcloudConfig{BaseURL: srv.URL, Username: "imgshare@rake.pro", Password: "pw"}, srv.Client())
	if _, err := nc.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png"); err == nil {
		t.Fatal("expected error on statuscode 404")
	}
}
