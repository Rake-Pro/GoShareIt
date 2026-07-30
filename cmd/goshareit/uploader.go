package main

import (
	"fmt"
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
			Method:                cfg.Custom.Method,
			URL:                   cfg.Custom.URL,
			Headers:               substituteSecret(cfg.Custom.Headers, secret),
			Body:                  cfg.Custom.Body,
			FileField:             cfg.Custom.FileField,
			ExtraFields:           substituteSecret(cfg.Custom.ExtraFields, secret),
			ResponseURLPath:       cfg.Custom.ResponseURLPath,
			ResponseDirectURLPath: cfg.Custom.ResponseDirectURLPath,
			ResponseURLRegex:      cfg.Custom.ResponseURLRegex,
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
		return nil, fmt.Errorf("unknown upload.destination %q", cfg.Upload.Destination)
	}
}

// substituteSecret replaces a literal "{secret}" placeholder in each map
// value with secret, so custom-uploader tokens never live in the YAML. A nil
// map stays nil.
func substituteSecret(m map[string]string, secret string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = strings.ReplaceAll(v, "{secret}", secret)
	}
	return out
}
