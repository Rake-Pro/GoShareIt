package main

import (
	"encoding/base64"
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
		return nil, fmt.Errorf("unknown upload.destination %q", cfg.Upload.Destination)
	}
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
