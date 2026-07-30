package upload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
)

// CustomConfig configures a generic ShareX-style HTTP uploader.
type CustomConfig struct {
	Method                string            // "POST" or "PUT"; default "POST"
	URL                   string            // supports {name} {mime} placeholders
	Headers               map[string]string // values support {name} {mime} placeholders
	Body                  string            // "multipart" or "raw"; default "multipart"
	FileField             string            // multipart field name for the file; default "file"
	ExtraFields           map[string]string // additional multipart form fields; values support {name} {mime}. Ignored for raw bodies.
	ResponseURLPath       string            // dot path into a JSON response for the public URL, e.g. "data.link"; array indices are numeric segments
	ResponseDirectURLPath string            // same, for the direct URL; falls back to the resolved public URL when empty
	ResponseURLRegex      string            // used for the public URL when ResponseURLPath is empty; the first capture group is the URL
}

// Custom uploads via an arbitrary HTTP endpoint, ShareX "custom uploader"
// style: multipart or raw body, JSON dot-path or regex response parsing.
type Custom struct {
	cfg    CustomConfig
	client *http.Client
}

// NewCustom builds an uploader. If client is nil, http.DefaultClient is used.
func NewCustom(cfg CustomConfig, client *http.Client) *Custom {
	if client == nil {
		client = http.DefaultClient
	}
	return &Custom{cfg: cfg, client: client}
}

func (c *Custom) method() string {
	if c.cfg.Method == "" {
		return http.MethodPost
	}
	return strings.ToUpper(c.cfg.Method)
}

func (c *Custom) fileField() string {
	if c.cfg.FileField == "" {
		return "file"
	}
	return c.cfg.FileField
}

func (c *Custom) isRawBody() bool {
	return strings.EqualFold(c.cfg.Body, "raw")
}

// substitute replaces {name}/{mime} placeholders verbatim (no escaping):
// callers fill in header values, form fields, and full URLs that may already
// contain their own scheme/query, so blanket URL-escaping would be wrong.
func substitute(s, name, mime string) string {
	out := strings.ReplaceAll(s, "{name}", name)
	out = strings.ReplaceAll(out, "{mime}", mime)
	return out
}

// Upload sends the file to the configured endpoint and extracts the share
// links from the response.
func (c *Custom) Upload(ctx context.Context, name string, body io.Reader, size int64, mime string) (UploadResult, error) {
	url := substitute(c.cfg.URL, name, mime)

	var reqBody io.Reader
	contentType := mime
	if c.isRawBody() {
		reqBody = body
	} else {
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		contentType = mw.FormDataContentType()
		go func() {
			pw.CloseWithError(c.writeMultipart(mw, name, mime, body))
		}()
		reqBody = pr
	}

	req, err := http.NewRequestWithContext(ctx, c.method(), url, reqBody)
	if err != nil {
		return UploadResult{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, substitute(v, name, mime))
	}
	if c.isRawBody() && size > 0 {
		req.ContentLength = size
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return UploadResult{}, fmt.Errorf("custom upload: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return UploadResult{}, fmt.Errorf("custom upload: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(b))
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		return UploadResult{}, fmt.Errorf("custom upload: unexpected status %d: %s", resp.StatusCode, snippet)
	}

	publicURL, err := c.resolvePublicURL(b)
	if err != nil {
		return UploadResult{}, err
	}
	directURL := publicURL
	if c.cfg.ResponseDirectURLPath != "" {
		directURL, err = jsonDotPath(b, c.cfg.ResponseDirectURLPath)
		if err != nil {
			return UploadResult{}, err
		}
	}
	return UploadResult{PublicURL: publicURL, DirectURL: directURL}, nil
}

// writeMultipart streams ExtraFields then the file part into mw, closing it
// when done. Run on its own goroutine, paired with an io.Pipe, so the
// request can start streaming before the whole body is buffered.
func (c *Custom) writeMultipart(mw *multipart.Writer, name, mime string, body io.Reader) error {
	for k, v := range c.cfg.ExtraFields {
		if err := mw.WriteField(k, substitute(v, name, mime)); err != nil {
			return err
		}
	}
	fw, err := createFormFile(mw, c.fileField(), name, mime)
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, body); err != nil {
		return err
	}
	return mw.Close()
}

func (c *Custom) resolvePublicURL(body []byte) (string, error) {
	switch {
	case c.cfg.ResponseURLPath != "":
		return jsonDotPath(body, c.cfg.ResponseURLPath)
	case c.cfg.ResponseURLRegex != "":
		return regexCapture(body, c.cfg.ResponseURLRegex)
	default:
		return strings.TrimSpace(string(body)), nil
	}
}

// jsonDotPath walks a JSON document following a dot-separated path (e.g.
// "data.link"); a path segment that parses as an integer indexes into a
// JSON array instead of an object key.
func jsonDotPath(body []byte, path string) (string, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return "", fmt.Errorf("custom upload: parse json response: %w", err)
	}
	for _, seg := range strings.Split(path, ".") {
		if idx, err := strconv.Atoi(seg); err == nil {
			arr, ok := v.([]any)
			if !ok || idx < 0 || idx >= len(arr) {
				return "", fmt.Errorf("custom upload: response path %q: index %d not found", path, idx)
			}
			v = arr[idx]
			continue
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return "", fmt.Errorf("custom upload: response path %q: %q is not an object", path, seg)
		}
		next, ok := obj[seg]
		if !ok {
			return "", fmt.Errorf("custom upload: response path %q: key %q not found", path, seg)
		}
		v = next
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("custom upload: response path %q: value is not a string", path)
	}
	return s, nil
}

func regexCapture(body []byte, pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("custom upload: compile response regex: %w", err)
	}
	m := re.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf("custom upload: response regex %q did not match (with a capture group)", pattern)
	}
	return string(m[1]), nil
}

func createFormFile(mw *multipart.Writer, fieldname, filename, mime string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", multipart.FileContentDisposition(fieldname, filename))
	if mime == "" {
		mime = "application/octet-stream"
	}
	h.Set("Content-Type", mime)
	return mw.CreatePart(h)
}
