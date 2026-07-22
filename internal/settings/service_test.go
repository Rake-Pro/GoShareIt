package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
)

func testHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestLoadMissingConfigGivesDefaults(t *testing.T) {
	testHome(t)
	svc := &Service{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), Version: "1.2.3"}
	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	if res.HasPassword || res.HasToken {
		t.Error("fresh setup should have no secrets")
	}
	if res.Config.Update.Repo != "Rake-Pro/GoShareIt" {
		t.Errorf("repo default = %q", res.Config.Update.Repo)
	}
	if res.Config.Nextcloud.PasswordFile == "" {
		t.Error("password_file default missing")
	}
	if res.Version != "1.2.3" {
		t.Errorf("version = %q", res.Version)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	testHome(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	svc := &Service{ConfigPath: cfgPath}

	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := res.Config
	cfg.Nextcloud.BaseURL = "https://cloud.example.com"
	cfg.Nextcloud.Username = "user@example.com"
	cfg.Hotkeys.Region = "Ctrl+Shift+5"

	if err := svc.Save(&SaveRequest{Config: cfg, NewPassword: "pw-abc", NewToken: "tok-xyz"}); err != nil {
		t.Fatal(err)
	}

	// The saved file must pass the strict loader with the secret in place.
	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Password() != "pw-abc" {
		t.Errorf("password = %q", loaded.Password())
	}
	if loaded.UpdateToken() != "tok-xyz" {
		t.Errorf("token = %q", loaded.UpdateToken())
	}
	if loaded.Hotkeys.Region != "Ctrl+Shift+5" {
		t.Errorf("hotkey = %q", loaded.Hotkeys.Region)
	}
	if loaded.Nextcloud.DavUser != "user" {
		t.Errorf("dav_user = %q", loaded.Nextcloud.DavUser)
	}

	// Re-load via the service: secrets present, not returned.
	res2, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !res2.HasPassword || !res2.HasToken {
		t.Error("secrets should be reported present after save")
	}

	// Save again without secrets: existing ones untouched.
	if err := svc.Save(&SaveRequest{Config: res2.Config}); err != nil {
		t.Fatal(err)
	}
	if again, _ := config.Load(cfgPath); again.Password() != "pw-abc" {
		t.Error("password lost on secretless save")
	}
}

func TestSaveInvalidConfigReportsError(t *testing.T) {
	testHome(t)
	svc := &Service{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), Version: "1.2.3"}
	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	res.Config.Nextcloud.BaseURL = "not-a-url"
	res.Config.Nextcloud.Username = "u"
	err = svc.Save(&SaveRequest{Config: res.Config, NewPassword: "x"})
	if err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("expected base_url validation error, got %v", err)
	}
	// Fail closed: an invalid config must never be installed - the host
	// restarts on config change and would fatal on load.
	if _, statErr := os.Stat(svc.ConfigPath); !os.IsNotExist(statErr) {
		t.Error("invalid config must not be installed")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(svc.ConfigPath), ".config-*")); len(leftovers) > 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}
