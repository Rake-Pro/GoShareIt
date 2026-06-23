package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExpandsTildePasswordFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	secretDir := filepath.Join(home, ".config", "goshareit")
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretDir, "app-password.secret"), []byte("tilde-secret-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := "nextcloud:\n" +
		"  base_url: https://cloud.rake.pro\n" +
		"  username: imgshare@rake.pro\n" +
		"  password_file: ~/.config/goshareit/app-password.secret\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Password(); got != "tilde-secret-123" {
		t.Fatalf("password = %q, want %q (tilde not expanded?)", got, "tilde-secret-123")
	}
	if c.Nextcloud.DavUser != "imgshare" {
		t.Fatalf("dav_user = %q, want imgshare", c.Nextcloud.DavUser)
	}
}
