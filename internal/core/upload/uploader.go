// Package upload defines the upload seam and the Nextcloud implementation.
package upload

import (
	"context"
	"io"
)

// UploadResult holds the share links produced for an uploaded file.
type UploadResult struct {
	PublicURL  string // viewer page, stored in history
	DirectURL  string // raw-bytes download link, copied to clipboard
	ShareToken string
}

// Uploader uploads a single blob and returns its share links.
type Uploader interface {
	Upload(ctx context.Context, name string, body io.Reader, size int64, mime string) (UploadResult, error)
}
