package upload

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCustomUploadMultipartDefault(t *testing.T) {
	var (
		method      string
		reqtype     string
		fileName    string
		fileContent string
		filePartCT  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("ParseMediaType: %v", err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, perr := mr.NextPart()
			if perr == io.EOF {
				break
			}
			if perr != nil {
				t.Fatalf("NextPart: %v", perr)
			}
			b, _ := io.ReadAll(part)
			switch part.FormName() {
			case "reqtype":
				reqtype = string(b)
			case "file":
				fileName = part.FileName()
				fileContent = string(b)
				filePartCT = part.Header.Get("Content-Type")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"link":"https://cdn.example.com/shot.png"}}`)
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{
		URL:             srv.URL,
		ExtraFields:     map[string]string{"reqtype": "fileupload"},
		ResponseURLPath: "data.link",
	}, srv.Client())

	got, err := c.Upload(context.Background(), "shot.png", strings.NewReader("PNGDATA"), 7, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if reqtype != "fileupload" {
		t.Errorf("reqtype field = %q", reqtype)
	}
	if fileName != "shot.png" {
		t.Errorf("file name = %q", fileName)
	}
	if fileContent != "PNGDATA" {
		t.Errorf("file content = %q", fileContent)
	}
	if filePartCT != "image/png" {
		t.Errorf("file part content-type = %q", filePartCT)
	}
	if want := "https://cdn.example.com/shot.png"; got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
	if got.DirectURL != got.PublicURL {
		t.Errorf("DirectURL = %q, want equal to PublicURL", got.DirectURL)
	}
}

func TestCustomUploadRawBody(t *testing.T) {
	var (
		method string
		path   string
		auth   string
		ct     string
		body   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		ct = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, "https://0x0.example.com/abc.png\n")
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{
		Method:  "PUT",
		URL:     srv.URL + "/upload/{name}",
		Body:    "raw",
		Headers: map[string]string{"Authorization": "Bearer token-for-{mime}"},
	}, srv.Client())

	got, err := c.Upload(context.Background(), "abc.png", strings.NewReader("RAWDATA"), 7, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if method != http.MethodPut {
		t.Errorf("method = %q, want PUT", method)
	}
	if path != "/upload/abc.png" {
		t.Errorf("path = %q, want /upload/abc.png", path)
	}
	if auth != "Bearer token-for-image/png" {
		t.Errorf("Authorization = %q", auth)
	}
	if ct != "image/png" {
		t.Errorf("Content-Type = %q", ct)
	}
	if body != "RAWDATA" {
		t.Errorf("body = %q", body)
	}
	if want := "https://0x0.example.com/abc.png"; got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
}

func TestCustomUploadResponseURLRegex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `upload ok: url=https://cdn.example.com/x.png done`)
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{
		URL:              srv.URL,
		Body:             "raw",
		ResponseURLRegex: `url=(\S+) done`,
	}, srv.Client())

	got, err := c.Upload(context.Background(), "x.png", strings.NewReader("x"), 1, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if want := "https://cdn.example.com/x.png"; got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
}

func TestCustomUploadResponseArrayIndexAndDirectURLPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"files":[{"url":"https://cdn.example.com/view.png","direct":"https://cdn.example.com/raw.png"}]}`)
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{
		URL:                   srv.URL,
		Body:                  "raw",
		ResponseURLPath:       "files.0.url",
		ResponseDirectURLPath: "files.0.direct",
	}, srv.Client())

	got, err := c.Upload(context.Background(), "x.png", strings.NewReader("x"), 1, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if want := "https://cdn.example.com/view.png"; got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
	if want := "https://cdn.example.com/raw.png"; got.DirectURL != want {
		t.Errorf("DirectURL = %q, want %q", got.DirectURL, want)
	}
}

func TestCustomUploadWholeBodyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "  https://catbox.example.com/x.png  \n")
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{URL: srv.URL, Body: "raw"}, srv.Client())
	got, err := c.Upload(context.Background(), "x.png", strings.NewReader("x"), 1, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if want := "https://catbox.example.com/x.png"; got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
}

func TestCustomUploadErrorStatusIncludesSnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "invalid api key")
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{URL: srv.URL, Body: "raw"}, srv.Client())
	_, err := c.Upload(context.Background(), "x.png", strings.NewReader("x"), 1, "image/png")
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error missing body snippet: %v", err)
	}
}

func TestCustomUploadResponsePathMissingKeyErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{URL: srv.URL, Body: "raw", ResponseURLPath: "data.link"}, srv.Client())
	if _, err := c.Upload(context.Background(), "x.png", strings.NewReader("x"), 1, "image/png"); err == nil {
		t.Fatal("expected error for missing json key")
	}
}

// TestCustomUploadResponseTemplates covers a host that returns an id rather
// than a URL: every template placeholder kind is exercised once, and the
// deletion URL is captured alongside the public and direct links.
func TestCustomUploadResponseTemplates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Token", "tok-9")
		_, _ = io.WriteString(w, `{"data":{"id":"abc123","deletehash":"del456"}}`)
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{
		URL:                       srv.URL,
		ResponseURLRegex:          `"id":"([a-z0-9]+)"`,
		ResponseURLTemplate:       "https://host.example/p/{json:data.id}",
		ResponseDirectURLTemplate: "https://host.example/i/{regex}/{name}",
		ResponseDeleteURLTemplate: "https://host.example/delete/{json:data.deletehash}?t={header:X-Token}",
	}, nil)
	res, err := c.Upload(context.Background(), "shot.png", strings.NewReader("x"), 1, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.PublicURL != "https://host.example/p/abc123" {
		t.Errorf("PublicURL = %q", res.PublicURL)
	}
	if res.DirectURL != "https://host.example/i/abc123/shot.png" {
		t.Errorf("DirectURL = %q", res.DirectURL)
	}
	if res.DeleteURL != "https://host.example/delete/del456?t=tok-9" {
		t.Errorf("DeleteURL = %q", res.DeleteURL)
	}
}

// TestCustomUploadDeleteURLPathMissingIsNotFatal verifies an absent deletion
// field never fails an otherwise successful upload.
func TestCustomUploadDeleteURLPathMissingIsNotFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"url":"https://host.example/x"}}`)
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{URL: srv.URL, ResponseURLPath: "data.url", ResponseDeleteURLPath: "data.delete_url"}, nil)
	res, err := c.Upload(context.Background(), "shot.png", strings.NewReader("x"), 1, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.PublicURL != "https://host.example/x" || res.DeleteURL != "" {
		t.Errorf("result = %+v", res)
	}
}

// TestCustomUploadTemplateErrors verifies a template whose placeholder cannot
// be resolved fails loudly instead of yielding a half-rendered URL.
func TestCustomUploadTemplateErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{}}`)
	}))
	defer srv.Close()

	for _, tmpl := range []string{
		"https://h/{json:data.id}",
		"https://h/{regex}",
		"https://h/{header:X-Missing}",
	} {
		c := NewCustom(CustomConfig{URL: srv.URL, ResponseURLTemplate: tmpl}, nil)
		if _, err := c.Upload(context.Background(), "s.png", strings.NewReader("x"), 1, "image/png"); err == nil {
			t.Errorf("template %q: expected an error", tmpl)
		}
	}
}
