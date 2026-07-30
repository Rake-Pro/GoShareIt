package upload

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestS3UploadWithURLTemplate(t *testing.T) {
	var (
		putPath string
		putBody string
		putCT   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			putPath = r.URL.Path
			putCT = r.Header.Get("Content-Type")
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			w.Header().Set("ETag", `"abc"`)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	s3, err := NewS3(S3Config{
		Endpoint:     srv.URL,
		Region:       "us-east-1",
		Bucket:       "my-bucket",
		AccessKey:    "AKIA",
		SecretKey:    "secret",
		Prefix:       "shots",
		URLTemplate:  "https://cdn.example.com/{bucket}/{key}",
		UsePathStyle: true,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	got, err := s3.Upload(context.Background(), "shot.png", strings.NewReader("PNGDATA"), 7, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if want := "/my-bucket/shots/shot.png"; putPath != want {
		t.Errorf("PUT path = %q, want %q", putPath, want)
	}
	// minio-go signs the body as an aws-chunked STREAMING-AWS4-HMAC-SHA256
	// payload (chunk-signature framing around the raw bytes); a real S3
	// server decodes that, our test handler just checks the payload made it.
	if !strings.Contains(putBody, "PNGDATA") {
		t.Errorf("PUT body = %q, want it to contain %q", putBody, "PNGDATA")
	}
	if putCT != "image/png" {
		t.Errorf("PUT content-type = %q", putCT)
	}
	if want := "https://cdn.example.com/my-bucket/shots/shot.png"; got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
	if got.DirectURL != got.PublicURL {
		t.Errorf("DirectURL = %q, want equal to PublicURL", got.DirectURL)
	}
	if got.ShareToken != "" {
		t.Errorf("ShareToken = %q, want empty", got.ShareToken)
	}
}

func TestS3UploadWithoutPrefix(t *testing.T) {
	var putPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s3, err := NewS3(S3Config{
		Endpoint:     srv.URL,
		Region:       "us-east-1",
		Bucket:       "my-bucket",
		AccessKey:    "AKIA",
		SecretKey:    "secret",
		URLTemplate:  "https://cdn.example.com/{key}",
		UsePathStyle: true,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	if _, err := s3.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if want := "/my-bucket/a.png"; putPath != want {
		t.Errorf("PUT path = %q, want %q", putPath, want)
	}
}

func TestS3UploadPresignedFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s3, err := NewS3(S3Config{
		Endpoint:     srv.URL,
		Region:       "us-east-1",
		Bucket:       "my-bucket",
		AccessKey:    "AKIA",
		SecretKey:    "secret",
		UsePathStyle: true,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	got, err := s3.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !strings.HasPrefix(got.PublicURL, srv.URL) {
		t.Errorf("PublicURL = %q, want prefix %q", got.PublicURL, srv.URL)
	}
	if !strings.Contains(got.PublicURL, "X-Amz-Signature=") {
		t.Errorf("PublicURL missing presign signature: %q", got.PublicURL)
	}
	if !strings.Contains(got.PublicURL, "X-Amz-Expires=604800") {
		t.Errorf("PublicURL missing default 7-day expiry: %q", got.PublicURL)
	}
	if got.DirectURL != got.PublicURL {
		t.Errorf("DirectURL = %q, want equal to PublicURL", got.DirectURL)
	}
}

func TestS3UploadPutError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>denied</Message></Error>`)
	}))
	defer srv.Close()

	s3, err := NewS3(S3Config{
		Endpoint:     srv.URL,
		Region:       "us-east-1",
		Bucket:       "my-bucket",
		AccessKey:    "AKIA",
		SecretKey:    "secret",
		UsePathStyle: true,
	}, srv.Client())
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	if _, err := s3.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png"); err == nil {
		t.Fatal("expected error on 403 response")
	}
}
