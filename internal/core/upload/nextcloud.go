package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NextcloudConfig configures the Nextcloud uploader. Password must already be
// resolved (the core never stores the literal password in YAML).
type NextcloudConfig struct {
	BaseURL         string
	Username        string
	DavUser         string // dav files segment; defaults to local part of Username
	Password        string
	RemoteDir       string
	ShareExpireDays int
	SharePassword   string
}

// Nextcloud uploads via WebDAV PUT and creates a public OCS share.
type Nextcloud struct {
	cfg    NextcloudConfig
	client *http.Client
}

// NewNextcloud builds an uploader. If client is nil, http.DefaultClient is used.
func NewNextcloud(cfg NextcloudConfig, client *http.Client) *Nextcloud {
	if cfg.DavUser == "" {
		cfg.DavUser = davUserFromUsername(cfg.Username)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Nextcloud{cfg: cfg, client: client}
}

func davUserFromUsername(username string) string {
	if i := strings.IndexByte(username, '@'); i >= 0 {
		return username[:i]
	}
	return username
}

// remotePath is the share/OCS path relative to the dav user's files root,
// e.g. "/sub/dir/name" or "/name" when RemoteDir is empty.
func (n *Nextcloud) remotePath(name string) string {
	dir := strings.Trim(n.cfg.RemoteDir, "/")
	if dir == "" {
		return "/" + name
	}
	return "/" + dir + "/" + name
}

func (n *Nextcloud) davURL(name string) string {
	base := strings.TrimRight(n.cfg.BaseURL, "/")
	return base + "/remote.php/dav/files/" + n.cfg.DavUser + n.remotePath(name)
}

type ocsResponse struct {
	OCS struct {
		Meta struct {
			Status     string `json:"status"`
			StatusCode int    `json:"statuscode"`
			Message    string `json:"message"`
		} `json:"meta"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	} `json:"ocs"`
}

// Upload performs the validated two-step flow: WebDAV PUT then OCS share.
func (n *Nextcloud) Upload(ctx context.Context, name string, body io.Reader, size int64, mime string) (UploadResult, error) {
	if err := n.put(ctx, name, body, size, mime); err != nil {
		return UploadResult{}, err
	}
	token, err := n.share(ctx, name)
	if err != nil {
		return UploadResult{}, err
	}
	base := strings.TrimRight(n.cfg.BaseURL, "/")
	// Static images use /preview so the link renders inline; everything else
	// (animated GIFs, video, etc.) uses /download to serve the raw bytes. A
	// GIF's /preview is a single static frame, so it must not use /preview.
	direct := base + "/s/" + token + "/download"
	if mime == "image/png" || mime == "image/jpeg" {
		direct = base + "/s/" + token + "/preview"
	}
	return UploadResult{
		PublicURL:  base + "/s/" + token,
		DirectURL:  direct,
		ShareToken: token,
	}, nil
}

func (n *Nextcloud) put(ctx context.Context, name string, body io.Reader, size int64, mime string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, n.davURL(name), body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(n.cfg.Username, n.cfg.Password)
	if mime != "" {
		req.Header.Set("Content-Type", mime)
	}
	if size > 0 {
		req.ContentLength = size
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webdav put: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK, http.StatusNoContent:
		return nil
	default:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("webdav put: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

func (n *Nextcloud) share(ctx context.Context, name string) (string, error) {
	endpoint := strings.TrimRight(n.cfg.BaseURL, "/") + "/ocs/v2.php/apps/files_sharing/api/v1/shares"

	form := url.Values{}
	form.Set("path", n.remotePath(name))
	form.Set("shareType", "3")
	form.Set("permissions", "1")
	if n.cfg.ShareExpireDays > 0 {
		form.Set("expireDate", expireDate(n.cfg.ShareExpireDays))
	}
	if n.cfg.SharePassword != "" {
		form.Set("password", n.cfg.SharePassword)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(n.cfg.Username, n.cfg.Password)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Accept", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ocs share: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("ocs share: read body: %w", err)
	}
	var parsed ocsResponse
	if err := json.Unmarshal(b, &parsed); err != nil {
		return "", fmt.Errorf("ocs share: parse json (status %d): %w", resp.StatusCode, err)
	}
	if parsed.OCS.Meta.StatusCode != 200 {
		return "", fmt.Errorf("ocs share: statuscode %d: %s", parsed.OCS.Meta.StatusCode, parsed.OCS.Meta.Message)
	}
	if parsed.OCS.Data.Token == "" {
		return "", fmt.Errorf("ocs share: empty token")
	}
	return parsed.OCS.Data.Token, nil
}

// nowFunc returns the current time; overridable in tests.
var nowFunc = time.Now

// expireDate returns the YYYY-MM-DD date `days` from now.
func expireDate(days int) string {
	return nowFunc().AddDate(0, 0, days).Format("2006-01-02")
}
