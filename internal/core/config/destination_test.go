package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUploadDestinationDefaultsToNextcloud(t *testing.T) {
	var c Config
	c.applyDefaults()
	if c.Upload.Destination != "nextcloud" {
		t.Errorf("Destination = %q, want nextcloud", c.Upload.Destination)
	}
}

func TestUploadDestinationInvalidEnum(t *testing.T) {
	p := writeCfg(t, "upload:\n  enabled: false\n  destination: dropbox\n")
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for unknown upload.destination")
	}
}

// S3: required fields fail closed when s3 is the active destination.
func TestS3RequiresFieldsWhenActive(t *testing.T) {
	dir := t.TempDir()
	body := "upload:\n  destination: s3\ns3:\n  bucket: my-bucket\n"
	if _, err := Load(writeCfg(t, body)); err == nil {
		t.Fatal("expected error: s3.endpoint missing")
	}
	secret := filepath.Join(dir, "s3.secret")
	if err := os.WriteFile(secret, []byte("shh"), 0o600); err != nil {
		t.Fatal(err)
	}
	body = "upload:\n  destination: s3\ns3:\n  endpoint: s3.example.com\n  bucket: my-bucket\n  access_key: AKIA\n  secret_key_file: " + secret + "\n"
	cfg, err := Load(writeCfg(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3SecretKey() != "shh" {
		t.Errorf("S3SecretKey() = %q, want shh", cfg.S3SecretKey())
	}
}

func TestS3SecretKeyFromEnv(t *testing.T) {
	t.Setenv("GSIT_TEST_S3_KEY", "env-secret")
	body := "upload:\n  destination: s3\ns3:\n  endpoint: s3.example.com\n  bucket: b\n  access_key: AKIA\n  secret_key_env: GSIT_TEST_S3_KEY\n"
	cfg, err := Load(writeCfg(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3SecretKey() != "env-secret" {
		t.Errorf("S3SecretKey() = %q, want env-secret", cfg.S3SecretKey())
	}
}

func TestS3SecretKeyBothSourcesError(t *testing.T) {
	body := "upload:\n  destination: s3\ns3:\n  endpoint: e\n  bucket: b\n  access_key: k\n  secret_key_file: /tmp/x\n  secret_key_env: FOO\n"
	if _, err := Load(writeCfg(t, body)); err == nil {
		t.Fatal("expected error when both s3 secret sources set")
	}
}

// SFTP: host/user + (password or key) required when active.
func TestSFTPRequiresPasswordOrKeyWhenActive(t *testing.T) {
	body := "upload:\n  destination: sftp\nsftp:\n  host: h\n  user: u\n"
	if _, err := Load(writeCfg(t, body)); err == nil {
		t.Fatal("expected error: no password or key configured")
	}
}

func TestSFTPPasswordAuthWhenActive(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "pw")
	if err := os.WriteFile(secret, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "upload:\n  destination: sftp\nsftp:\n  host: h\n  user: u\n  password_file: " + secret + "\n"
	cfg, err := Load(writeCfg(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SFTPPassword() != "s3cr3t" {
		t.Errorf("SFTPPassword() = %q, want s3cr3t", cfg.SFTPPassword())
	}
}

// A private_key_file's contents load into the resolved PEM, and satisfy the
// "password or key" requirement without a password configured.
func TestSFTPPrivateKeyFileLoadsContents(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	pem := "-----BEGIN OPENSSH PRIVATE KEY-----\nFAKEKEYDATA\n-----END OPENSSH PRIVATE KEY-----\n"
	if err := os.WriteFile(keyPath, []byte(pem), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "upload:\n  destination: sftp\nsftp:\n  host: h\n  user: u\n  private_key_file: " + keyPath + "\n"
	cfg, err := Load(writeCfg(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SFTPPrivateKeyPEM() != pem {
		t.Errorf("SFTPPrivateKeyPEM() = %q, want %q", cfg.SFTPPrivateKeyPEM(), pem)
	}
	if cfg.SFTPPassword() != "" {
		t.Errorf("SFTPPassword() = %q, want empty (key auth, no password configured)", cfg.SFTPPassword())
	}
}

func TestSFTPPrivateKeyFileMissingFailsWhenActive(t *testing.T) {
	body := "upload:\n  destination: sftp\nsftp:\n  host: h\n  user: u\n  private_key_file: /nonexistent/key.pem\n"
	if _, err := Load(writeCfg(t, body)); err == nil {
		t.Fatal("expected error: private_key_file does not exist")
	}
}

// webdav: base_url required when active.
func TestWebDAVRequiresBaseURLWhenActive(t *testing.T) {
	body := "upload:\n  destination: webdav\nwebdav:\n  username: u\n"
	if _, err := Load(writeCfg(t, body)); err == nil {
		t.Fatal("expected error: webdav.base_url missing")
	}
}

func TestWebDAVPasswordOptional(t *testing.T) {
	body := "upload:\n  destination: webdav\nwebdav:\n  base_url: https://dav.example.com\n"
	cfg, err := Load(writeCfg(t, body))
	if err != nil {
		t.Fatalf("Load: %v (webdav password should be optional)", err)
	}
	if cfg.WebDAVPassword() != "" {
		t.Errorf("WebDAVPassword() = %q, want empty", cfg.WebDAVPassword())
	}
}

// custom: url required when active; {secret} substitution is done by the
// wiring layer, but the resolved secret itself is exposed here.
func TestCustomRequiresURLWhenActive(t *testing.T) {
	body := "upload:\n  destination: custom\ncustom:\n  method: POST\n"
	if _, err := Load(writeCfg(t, body)); err == nil {
		t.Fatal("expected error: custom.url missing")
	}
}

func TestCustomSecretFromFile(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "tok")
	if err := os.WriteFile(secret, []byte("tok-abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "upload:\n  destination: custom\ncustom:\n  url: https://up.example.com\n  secret_file: " + secret + "\n"
	cfg, err := Load(writeCfg(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CustomSecret() != "tok-abc" {
		t.Errorf("CustomSecret() = %q, want tok-abc", cfg.CustomSecret())
	}
}

// Active-only scoping: an invalid inactive destination section must not
// block load/save.
func TestInactiveDestinationSectionNotValidated(t *testing.T) {
	dir := t.TempDir()
	pwPath := filepath.Join(dir, "pw.secret")
	if err := os.WriteFile(pwPath, []byte("p"), 0o600); err != nil {
		t.Fatal(err)
	}
	// destination is nextcloud (fully configured); s3/sftp/webdav/custom are
	// all left empty/invalid and must be ignored.
	body := "nextcloud:\n  base_url: https://cloud.example.com\n  username: u@example.com\n  password_file: " + pwPath + "\n" +
		"upload:\n  destination: nextcloud\n" +
		"s3:\n  endpoint: \"\"\n" +
		"sftp:\n  host: \"\"\n" +
		"webdav:\n  base_url: \"\"\n" +
		"custom:\n  url: \"\"\n"
	if _, err := Load(writeCfg(t, body)); err != nil {
		t.Fatalf("Load: %v (inactive destination sections must not be validated)", err)
	}
}

// Switching the active destination away from nextcloud makes the Nextcloud
// section optional too (mirrors local-only mode).
func TestNextcloudOptionalWhenNotActive(t *testing.T) {
	body := "upload:\n  destination: webdav\nwebdav:\n  base_url: https://dav.example.com\n"
	if _, err := Load(writeCfg(t, body)); err != nil {
		t.Fatalf("Load: %v (nextcloud section should be optional when destination != nextcloud)", err)
	}
}
