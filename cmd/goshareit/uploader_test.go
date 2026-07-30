package main

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

func TestBuildUploader(t *testing.T) {
	tests := []struct {
		name    string
		cfg     func() *config.Config
		want    string
		wantErr bool
	}{
		{
			name: "nextcloud",
			cfg: func() *config.Config {
				var c config.Config
				c.Upload.Destination = "nextcloud"
				c.Nextcloud.BaseURL = "https://cloud.example.com"
				c.Nextcloud.Username = "u@example.com"
				return &c
			},
			want: "*upload.Nextcloud",
		},
		{
			name: "empty destination falls back to nextcloud",
			cfg: func() *config.Config {
				var c config.Config
				return &c
			},
			want: "*upload.Nextcloud",
		},
		{
			name: "s3",
			cfg: func() *config.Config {
				var c config.Config
				c.Upload.Destination = "s3"
				c.S3.Endpoint = "s3.example.com"
				c.S3.Bucket = "bucket"
				c.S3.AccessKey = "AKIA"
				return &c
			},
			want: "*upload.S3",
		},
		{
			name: "sftp",
			cfg: func() *config.Config {
				var c config.Config
				c.Upload.Destination = "sftp"
				c.SFTP.Host = "files.example.com"
				c.SFTP.User = "uploads"
				return &c
			},
			want: "*upload.SFTP",
		},
		{
			name: "webdav",
			cfg: func() *config.Config {
				var c config.Config
				c.Upload.Destination = "webdav"
				c.WebDAV.BaseURL = "https://dav.example.com"
				return &c
			},
			want: "*upload.WebDAV",
		},
		{
			name: "custom",
			cfg: func() *config.Config {
				var c config.Config
				c.Upload.Destination = "custom"
				c.Custom.URL = "https://up.example.com"
				return &c
			},
			want: "*upload.Custom",
		},
		{
			name: "unknown destination errors",
			cfg: func() *config.Config {
				var c config.Config
				c.Upload.Destination = "dropbox"
				return &c
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildUploader(tt.cfg())
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildUploader: %v", err)
			}
			if gotType := typeName(got); gotType != tt.want {
				t.Errorf("buildUploader() type = %s, want %s", gotType, tt.want)
			}
		})
	}
}

// TestBuildUploaderCustomSecretSubstitution verifies the {secret} placeholder
// is substituted in both header and extra-field values, end to end through
// buildUploader and a real Upload() call, while unrelated values are left
// untouched.
func TestBuildUploaderCustomSecretSubstitution(t *testing.T) {
	var gotAuth, gotStatic string
	gotFields := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotStatic = r.Header.Get("X-Static")
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
			if part.FileName() != "" {
				io.Copy(io.Discard, part)
				continue
			}
			b, _ := io.ReadAll(part)
			gotFields[part.FormName()] = string(b)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "secret")
	if err := os.WriteFile(secretPath, []byte("tok-123"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgYAML := "upload:\n  destination: custom\n" +
		"custom:\n" +
		"  url: \"" + srv.URL + "\"\n" +
		"  secret_file: \"" + secretPath + "\"\n" +
		"  headers:\n    Authorization: \"Bearer {secret}\"\n    X-Static: value\n" +
		"  extra_fields:\n    token: \"{secret}-suffix\"\n"
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	uploader, err := buildUploader(loaded)
	if err != nil {
		t.Fatalf("buildUploader: %v", err)
	}
	if _, err := uploader.Upload(context.Background(), "a.png", strings.NewReader("x"), 1, "image/png"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotAuth != "Bearer tok-123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer tok-123")
	}
	if gotStatic != "value" {
		t.Errorf("X-Static header = %q, want %q (must be left untouched)", gotStatic, "value")
	}
	if gotFields["token"] != "tok-123-suffix" {
		t.Errorf("token field = %q, want %q", gotFields["token"], "tok-123-suffix")
	}
}

func typeName(v upload.Uploader) string {
	switch v.(type) {
	case *upload.Nextcloud:
		return "*upload.Nextcloud"
	case *upload.S3:
		return "*upload.S3"
	case *upload.SFTP:
		return "*upload.SFTP"
	case *upload.WebDAV:
		return "*upload.WebDAV"
	case *upload.Custom:
		return "*upload.Custom"
	default:
		return "unknown"
	}
}
