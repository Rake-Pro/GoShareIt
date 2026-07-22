package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalYAML = `
nextcloud:
  base_url: https://cloud.example.com
  username: uploads@example.com
  password_file: PWFILE
upload:
  direct_link: true
after_upload:
  copy_url_to_clipboard: true
  notify: true
`

func writeFile(t *testing.T, dir, nameStr, content string) string {
	t.Helper()
	p := filepath.Join(dir, nameStr)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPasswordFile(t *testing.T) {
	dir := t.TempDir()
	pwPath := writeFile(t, dir, "pw.secret", "  s3cret\n")
	yaml := strings.ReplaceAll(minimalYAML, "PWFILE", pwPath)
	cfgPath := writeFile(t, dir, "config.yaml", yaml)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Password() != "s3cret" {
		t.Errorf("password = %q, want s3cret (trimmed)", cfg.Password())
	}
	if cfg.Nextcloud.DavUser != "uploads" {
		t.Errorf("dav_user = %q, want uploads (derived)", cfg.Nextcloud.DavUser)
	}
	if cfg.Upload.FilenameTemplate != "goshareit_{datetime}_{rand}.{ext}" {
		t.Errorf("default template not applied: %q", cfg.Upload.FilenameTemplate)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("default level = %q", cfg.Logging.Level)
	}
}

func TestLoadPasswordEnv(t *testing.T) {
	dir := t.TempDir()
	yaml := `
nextcloud:
  base_url: https://cloud.example.com
  username: uploads@example.com
  password_env: GSIT_TEST_PW
`
	cfgPath := writeFile(t, dir, "config.yaml", yaml)
	t.Setenv("GSIT_TEST_PW", "envpw")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Password() != "envpw" {
		t.Errorf("password = %q", cfg.Password())
	}
}

func TestLoadBothSourcesError(t *testing.T) {
	dir := t.TempDir()
	yaml := `
nextcloud:
  base_url: https://cloud.example.com
  username: uploads@example.com
  password_file: /tmp/x
  password_env: FOO
`
	cfgPath := writeFile(t, dir, "config.yaml", yaml)
	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error when both password sources set")
	}
}

func TestLoadNoPasswordSourceError(t *testing.T) {
	dir := t.TempDir()
	yaml := `
nextcloud:
  base_url: https://cloud.example.com
  username: uploads@example.com
`
	cfgPath := writeFile(t, dir, "config.yaml", yaml)
	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error when no password source set")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	dir := t.TempDir()
	pwPath := writeFile(t, dir, "pw.secret", "p")
	yaml := strings.ReplaceAll(minimalYAML, "PWFILE", pwPath)
	cfgPath := writeFile(t, dir, "config_override.yaml", yaml)
	t.Setenv(EnvConfigPath, cfgPath)

	cfg, err := Load("/nonexistent/path.yaml")
	if err != nil {
		t.Fatalf("Load with env override: %v", err)
	}
	if cfg.Nextcloud.Username != "uploads@example.com" {
		t.Errorf("env override not used")
	}
}

func TestValidateBaseURL(t *testing.T) {
	dir := t.TempDir()
	pwPath := writeFile(t, dir, "pw.secret", "p")
	yaml := `
nextcloud:
  base_url: ftp://bad
  username: u
  password_file: ` + pwPath + `
`
	cfgPath := writeFile(t, dir, "config.yaml", yaml)
	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error on non-http base_url")
	}
}
