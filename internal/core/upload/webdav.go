package upload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WebDAVConfig configures the generic WebDAV uploader. Password must already
// be resolved.
type WebDAVConfig struct {
	BaseURL     string
	Username    string
	Password    string
	RemoteDir   string
	URLTemplate string // public link template; placeholders {name} {path}. If empty, the PUT URL itself is used.
}

// WebDAV uploads via a plain WebDAV PUT with HTTP basic auth. No OCS share
// step; that's Nextcloud-specific.
type WebDAV struct {
	cfg    WebDAVConfig
	client *http.Client
}

// NewWebDAV builds an uploader. If client is nil, http.DefaultClient is used.
func NewWebDAV(cfg WebDAVConfig, client *http.Client) *WebDAV {
	if client == nil {
		client = http.DefaultClient
	}
	return &WebDAV{cfg: cfg, client: client}
}

// remotePath is the PUT path relative to BaseURL, e.g. "/sub/dir/name" or
// "/name" when RemoteDir is empty.
func (w *WebDAV) remotePath(name string) string {
	dir := strings.Trim(w.cfg.RemoteDir, "/")
	if dir == "" {
		return "/" + name
	}
	return "/" + dir + "/" + name
}

func (w *WebDAV) davURL(name string) string {
	base := strings.TrimRight(w.cfg.BaseURL, "/")
	return base + w.remotePath(name)
}

// Upload performs a single WebDAV PUT and resolves a link either from
// URLTemplate or the PUT URL itself.
func (w *WebDAV) Upload(ctx context.Context, name string, body io.Reader, size int64, mime string) (UploadResult, error) {
	putURL := w.davURL(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, body)
	if err != nil {
		return UploadResult{}, err
	}
	req.SetBasicAuth(w.cfg.Username, w.cfg.Password)
	if mime != "" {
		req.Header.Set("Content-Type", mime)
	}
	if size > 0 {
		req.ContentLength = size
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return UploadResult{}, fmt.Errorf("webdav put: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK, http.StatusNoContent:
	default:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return UploadResult{}, fmt.Errorf("webdav put: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	link := putURL
	if w.cfg.URLTemplate != "" {
		link = renderURLTemplate(w.cfg.URLTemplate, map[string]string{
			"name": name,
			"path": strings.TrimPrefix(w.remotePath(name), "/"),
		})
	}
	return UploadResult{PublicURL: link, DirectURL: link}, nil
}
