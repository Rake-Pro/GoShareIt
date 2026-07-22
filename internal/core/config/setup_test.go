package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStarterCreatesConfigAndSecret(t *testing.T) {
	dir := t.TempDir()
	// The starter config references ~/<app root>/app-password.secret, so HOME
	// must point at dir for that path to resolve to the secret we create.
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // windows equivalent
	appDir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(appDir, "config.yaml")

	secret, err := WriteStarter(cfgPath)
	if err != nil {
		t.Fatalf("WriteStarter: %v", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("secret not created: %v", err)
	}

	// Secret must be 0600 and empty.
	info, err := os.Stat(secret)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secret perm = %o, want 600", perm)
	}
	if info.Size() != 0 {
		t.Errorf("secret size = %d, want 0", info.Size())
	}

	// The starter ships neutral (no server preconfigured): it must parse raw,
	// and pass the strict loader once the user supplies server + password.
	if _, err := LoadRaw(cfgPath); err != nil {
		t.Fatalf("LoadRaw starter config: %v", err)
	}
	if err := os.WriteFile(secret, []byte("pw-123"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	filled := strings.Replace(string(raw), `base_url: ""`, `base_url: "https://cloud.example.com"`, 1)
	filled = strings.Replace(filled, `username: ""`, `username: "user@example.com"`, 1)
	if err := os.WriteFile(cfgPath, []byte(filled), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load starter config: %v", err)
	}
	if c.Password() != "pw-123" {
		t.Errorf("password = %q", c.Password())
	}
	if c.Nextcloud.DavUser != "user" {
		t.Errorf("dav_user = %q", c.Nextcloud.DavUser)
	}
}

func TestWriteStarterDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("nextcloud:\n  base_url: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteStarter(cfgPath); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "nextcloud:\n  base_url: x\n" {
		t.Errorf("existing config was overwritten: %q", b)
	}
}
