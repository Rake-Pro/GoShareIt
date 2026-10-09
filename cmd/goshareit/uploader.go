package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

// buildUploader constructs the Uploader for cfg.Upload.Destination. Extracted
// from main() so destination selection is table-testable.
func buildUploader(cfg *config.Config) (upload.Uploader, error) {
	switch cfg.Upload.Destination {
	case "s3":
		return upload.NewS3(upload.S3Config{
			Endpoint:       cfg.S3.Endpoint,
			Region:         cfg.S3.Region,
			Bucket:         cfg.S3.Bucket,
			AccessKey:      cfg.S3.AccessKey,
			SecretKey:      cfg.S3SecretKey(),
			Prefix:         cfg.S3.Prefix,
			URLTemplate:    cfg.S3.URLTemplate,
			UsePathStyle:   cfg.S3.UsePathStyle,
			PresignSeconds: cfg.S3.PresignSeconds,
		}, nil)
	case "sftp":
		if cfg.SFTP.HostKeyFingerprint == "" {
			log.Warn().Msg("sftp.host_key_fingerprint is empty - the SFTP host key will not be verified")
		}
		return upload.NewSFTP(upload.SFTPConfig{
			Host:                 cfg.SFTP.Host,
			Port:                 cfg.SFTP.Port,
			User:                 cfg.SFTP.User,
			Password:             cfg.SFTPPassword(),
			PrivateKeyPEM:        cfg.SFTPPrivateKeyPEM(),
			PrivateKeyPassphrase: cfg.SFTPPassphrase(),
			RemoteDir:            cfg.SFTP.RemoteDir,
			URLTemplate:          cfg.SFTP.URLTemplate,
			HostKeyFingerprint:   cfg.SFTP.HostKeyFingerprint,
		}), nil
	case "webdav":
		return upload.NewWebDAV(upload.WebDAVConfig{
			BaseURL:     cfg.WebDAV.BaseURL,
			Username:    cfg.WebDAV.Username,
			Password:    cfg.WebDAVPassword(),
			RemoteDir:   cfg.WebDAV.RemoteDir,
			URLTemplate: cfg.WebDAV.URLTemplate,
		}, nil), nil
	case "custom":
		secret := cfg.CustomSecret()
		return upload.NewCustom(upload.CustomConfig{
			Method:                    cfg.Custom.Method,
			URL:                       substituteSecretValue(cfg.Custom.URL, secret),
			Headers:                   substituteSecret(cfg.Custom.Headers, secret),
			Body:                      cfg.Custom.Body,
			FileField:                 cfg.Custom.FileField,
			ExtraFields:               substituteSecret(cfg.Custom.ExtraFields, secret),
			ResponseURLPath:           cfg.Custom.ResponseURLPath,
			ResponseDirectURLPath:     cfg.Custom.ResponseDirectURLPath,
			ResponseDeleteURLPath:     cfg.Custom.ResponseDeleteURLPath,
			ResponseURLRegex:          cfg.Custom.ResponseURLRegex,
			ResponseURLTemplate:       cfg.Custom.ResponseURLTemplate,
			ResponseDirectURLTemplate: cfg.Custom.ResponseDirectURLTemplate,
			ResponseDeleteURLTemplate: cfg.Custom.ResponseDeleteURLTemplate,
		}, nil), nil
	case "nextcloud", "":
		return upload.NewNextcloud(upload.NextcloudConfig{
			BaseURL:         cfg.Nextcloud.BaseURL,
			Username:        cfg.Nextcloud.Username,
			DavUser:         cfg.Nextcloud.DavUser,
			Password:        cfg.Password(),
			RemoteDir:       cfg.Nextcloud.RemoteDir,
			ShareExpireDays: cfg.Upload.ShareExpireDays,
			SharePassword:   cfg.Upload.SharePassword,
		}, nil), nil
	default:
		preset, ok := upload.CustomPresets()[cfg.Upload.Destination]
		if !ok {
			return nil, fmt.Errorf("unknown upload.destination %q", cfg.Upload.Destination)
		}
		secret := cfg.HostSecret()
		if secret == "" && preset.Secret != "" && !preset.SecretOptional {
			// Missing key: fail per upload with a pointer to Settings rather
			// than refusing to start the app.
			return missingSecretUploader{host: preset.Label, secret: preset.Secret}, nil
		}
		pc := preset.CustomConfig
		pc.URL = substituteSecretValue(pc.URL, secret)
		pc.Headers = substituteSecret(pc.Headers, secret)
		pc.ExtraFields = substituteSecret(pc.ExtraFields, secret)
		return upload.NewCustom(pc, nil), nil
	}
}

// missingSecretUploader stands in for a public host whose key is not set.
type missingSecretUploader struct{ host, secret string }

func (m missingSecretUploader) Upload(context.Context, string, io.Reader, int64, string) (upload.UploadResult, error) {
	return upload.UploadResult{}, fmt.Errorf("%s needs your %s: open Settings > Upload and enter it", m.host, m.secret)
}

// uploaderFor is buildUploader that never fails: a destination whose client
// cannot be built gets an unusableUploader, so the app always starts (a
// local-only config may name a half-set-up destination, and a broken one
// should fail per upload, with a notification, not at startup).
func uploaderFor(cfg *config.Config) upload.Uploader {
	u, err := buildUploader(cfg)
	if err != nil {
		log.Warn().Err(err).Bool("uploads_enabled", cfg.UploadEnabled()).Msg("build uploader; uploads will fail until the destination is fixed")
		return unusableUploader{err: err}
	}
	return u
}

// unusableUploader stands in when the destination's client cannot even be
// built (e.g. an S3 endpoint that is blank or not a host name). Starting the
// app must never fail on that: local-only mode does not need it at all, and
// with uploads on each upload reports the reason instead.
type unusableUploader struct{ err error }

func (u unusableUploader) Upload(context.Context, string, io.Reader, int64, string) (upload.UploadResult, error) {
	return upload.UploadResult{}, fmt.Errorf("the upload destination is not set up correctly; check Settings > Upload (%v)", u.err)
}

// substituteSecret replaces the secret placeholders in each map value, so
// custom-uploader tokens never live in the YAML. A nil map stays nil.
func substituteSecret(m map[string]string, secret string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = substituteSecretValue(v, secret)
	}
	return out
}

// substituteSecretValue fills {secret} (verbatim), {secret:base64} (the
// secret base64-encoded) and {secret:basic} (HTTP Basic credentials with an
// empty username, i.e. base64(":" + secret), the convention API-key hosts such
// as pixeldrain use).
func substituteSecretValue(v, secret string) string {
	v = strings.ReplaceAll(v, "{secret:base64}", base64.StdEncoding.EncodeToString([]byte(secret)))
	v = strings.ReplaceAll(v, "{secret:basic}", base64.StdEncoding.EncodeToString([]byte(":"+secret)))
	return strings.ReplaceAll(v, "{secret}", secret)
}
