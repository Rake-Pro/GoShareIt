package upload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// defaultPresignSeconds is the S3 presign TTL used when PresignSeconds is
// unset; it also happens to be the maximum a presigned GET can live.
const defaultPresignSeconds = 7 * 24 * 3600

// S3Config configures the S3-compatible uploader (AWS S3, Backblaze B2,
// Cloudflare R2, MinIO, etc). AccessKey/SecretKey must already be resolved.
type S3Config struct {
	Endpoint       string // host[:port], no scheme (e.g. "s3.us-west-002.backblazeb2.com")
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	Prefix         string // object key prefix, joined with name as "prefix/name"
	URLTemplate    string // public link template; placeholders {bucket} {key} {name}. If empty, a presigned GET is used instead.
	UsePathStyle   bool   // path-style addressing (bucket in the URL path, not the host); required by most non-AWS S3-compatible providers
	PresignSeconds int    // GET presign TTL when URLTemplate is empty; default and max 604800 (7 days)
}

// S3 uploads objects to an S3-compatible bucket via the minio-go SDK.
type S3 struct {
	cfg    S3Config
	client *minio.Client
}

// NewS3 builds an uploader. If httpClient is nil, minio-go's default
// transport is used; only httpClient.Transport is honored since minio-go
// owns its own *http.Client internally.
func NewS3(cfg S3Config, httpClient *http.Client) (*S3, error) {
	var transport http.RoundTripper
	if httpClient != nil {
		transport = httpClient.Transport
	}

	endpoint := cfg.Endpoint
	secure := true
	switch {
	case strings.HasPrefix(endpoint, "http://"):
		endpoint = strings.TrimPrefix(endpoint, "http://")
		secure = false
	case strings.HasPrefix(endpoint, "https://"):
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}

	lookup := minio.BucketLookupAuto
	if cfg.UsePathStyle {
		lookup = minio.BucketLookupPath
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       secure,
		Region:       cfg.Region,
		BucketLookup: lookup,
		Transport:    transport,
	})
	if err != nil {
		return nil, fmt.Errorf("s3: new client: %w", err)
	}
	return &S3{cfg: cfg, client: client}, nil
}

func (s *S3) objectKey(name string) string {
	prefix := strings.Trim(s.cfg.Prefix, "/")
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func (s *S3) presignSeconds() int {
	if s.cfg.PresignSeconds > 0 {
		return s.cfg.PresignSeconds
	}
	return defaultPresignSeconds
}

// Upload puts the object, then resolves a link either from URLTemplate or a
// presigned GET.
func (s *S3) Upload(ctx context.Context, name string, body io.Reader, size int64, mime string) (UploadResult, error) {
	key := s.objectKey(name)

	opts := minio.PutObjectOptions{}
	if mime != "" {
		opts.ContentType = mime
	}
	putSize := size
	if putSize <= 0 {
		putSize = -1
	}
	if _, err := s.client.PutObject(ctx, s.cfg.Bucket, key, body, putSize, opts); err != nil {
		return UploadResult{}, fmt.Errorf("s3: put object: %w", err)
	}

	link, err := s.link(ctx, key, name)
	if err != nil {
		return UploadResult{}, err
	}
	return UploadResult{PublicURL: link, DirectURL: link}, nil
}

func (s *S3) link(ctx context.Context, key, name string) (string, error) {
	if s.cfg.URLTemplate != "" {
		return renderURLTemplate(s.cfg.URLTemplate, map[string]string{
			"bucket": s.cfg.Bucket,
			"key":    key,
			"name":   name,
		}), nil
	}
	expiry := time.Duration(s.presignSeconds()) * time.Second
	u, err := s.client.PresignedGetObject(ctx, s.cfg.Bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("s3: presign: %w", err)
	}
	return u.String(), nil
}
