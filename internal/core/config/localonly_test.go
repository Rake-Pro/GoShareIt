package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Local-only mode: upload.enabled=false makes the whole nextcloud section and
// credentials optional.
func TestLoadLocalOnlyNeedsNoCredentials(t *testing.T) {
	p := writeCfg(t, "upload:\n  enabled: false\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UploadEnabled() {
		t.Error("UploadEnabled = true, want false")
	}
	if cfg.Password() != "" {
		t.Errorf("password = %q, want empty", cfg.Password())
	}
}

// An empty configured password file is tolerated in local-only mode (the exact
// first-run state) but still rejected when uploads are on.
func TestEmptyPasswordFileOnlyFatalWhenUploading(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	secret := filepath.Join(home, "app-password.secret")
	if err := os.WriteFile(secret, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	body := "nextcloud:\n  base_url: https://cloud.example.com\n  username: u@example.com\n" +
		"  password_file: " + secret + "\nupload:\n  enabled: %s\n"

	if _, err := Load(writeCfg(t, strings.Replace(body, "%s", "false", 1))); err != nil {
		t.Fatalf("local-only with empty secret should load: %v", err)
	}
	if _, err := Load(writeCfg(t, strings.Replace(body, "%s", "true", 1))); err == nil {
		t.Fatal("uploads enabled with empty secret must fail")
	}
}

// Default (key absent) stays upload-enabled for existing configs.
func TestUploadEnabledDefaultsTrue(t *testing.T) {
	var c Config
	c.applyDefaults()
	if !c.UploadEnabled() {
		t.Error("UploadEnabled default = false, want true")
	}
}

// Re-enabling later: local-only mode still best-effort resolves an existing
// secret so flipping the toggle back needs nothing else.
func TestLocalOnlyStillResolvesExistingSecret(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "s")
	if err := os.WriteFile(secret, []byte("pw-x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := writeCfg(t, "nextcloud:\n  password_file: "+secret+"\nupload:\n  enabled: false\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Password() != "pw-x" {
		t.Errorf("password = %q, want pw-x", cfg.Password())
	}
}
