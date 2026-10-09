package settings

import (
	"errors"
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
	if res.HasPassword {
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

	if err := svc.Save(&SaveRequest{Config: cfg, NewPassword: "pw-abc"}); err != nil {
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
	if !res2.HasPassword {
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

// Destination secrets round-trip the same way NewPassword does: write on
// non-empty, leave untouched when omitted, reject writes when the
// corresponding *_env is set.
func TestSaveDestinationSecrets(t *testing.T) {
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
	cfg.Upload.Destination = "s3"
	cfg.S3.Endpoint = "s3.example.com"
	cfg.S3.Bucket = "bucket"
	cfg.S3.AccessKey = "AKIA"

	if err := svc.Save(&SaveRequest{
		Config:            cfg,
		NewPassword:       "pw-abc",
		NewS3SecretKey:    "s3-secret",
		NewSFTPPassword:   "sftp-pw",
		NewSFTPPassphrase: "sftp-pass",
		NewWebDAVPassword: "webdav-pw",
		NewCustomSecret:   "custom-secret",
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.S3SecretKey() != "s3-secret" {
		t.Errorf("S3SecretKey() = %q", loaded.S3SecretKey())
	}
	if loaded.SFTPPassword() != "sftp-pw" {
		t.Errorf("SFTPPassword() = %q", loaded.SFTPPassword())
	}
	if loaded.SFTPPassphrase() != "sftp-pass" {
		t.Errorf("SFTPPassphrase() = %q", loaded.SFTPPassphrase())
	}
	if loaded.WebDAVPassword() != "webdav-pw" {
		t.Errorf("WebDAVPassword() = %q", loaded.WebDAVPassword())
	}
	if loaded.CustomSecret() != "custom-secret" {
		t.Errorf("CustomSecret() = %q", loaded.CustomSecret())
	}

	res2, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	for name, has := range map[string]bool{
		"HasS3SecretKey":    res2.HasS3SecretKey,
		"HasSFTPPassword":   res2.HasSFTPPassword,
		"HasSFTPPassphrase": res2.HasSFTPPassphrase,
		"HasWebDAVPassword": res2.HasWebDAVPassword,
		"HasCustomSecret":   res2.HasCustomSecret,
	} {
		if !has {
			t.Errorf("%s = false, want true after save", name)
		}
	}

	// Save again without secrets: existing ones untouched.
	if err := svc.Save(&SaveRequest{Config: res2.Config}); err != nil {
		t.Fatal(err)
	}
	if again, _ := config.Load(cfgPath); again.S3SecretKey() != "s3-secret" {
		t.Error("s3 secret key lost on secretless save")
	}
}

func TestSaveDestinationSecretRejectedWhenEnvSourced(t *testing.T) {
	testHome(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	svc := &Service{ConfigPath: cfgPath}
	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := res.Config
	cfg.Upload.Destination = "webdav"
	cfg.WebDAV.BaseURL = "https://dav.example.com"
	cfg.WebDAV.PasswordEnv = "GSIT_TEST_WEBDAV_PW"

	err = svc.Save(&SaveRequest{Config: cfg, NewWebDAVPassword: "should-not-write"})
	if err == nil || !strings.Contains(err.Error(), "env var") {
		t.Fatalf("expected env-sourced rejection, got %v", err)
	}
}

// DidSave drives the helper's ExitSaved exit code: it must flip only on a
// successful Save, never on load or a rejected one.
func TestDidSaveOnlyAfterSuccessfulSave(t *testing.T) {
	testHome(t)
	svc := &Service{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}
	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	if svc.DidSave() {
		t.Error("DidSave() = true before any save")
	}

	bad := *res.Config
	bad.Nextcloud.BaseURL = "not-a-url"
	bad.Nextcloud.Username = "u"
	if err := svc.Save(&SaveRequest{Config: &bad, NewPassword: "x"}); err == nil {
		t.Fatal("expected invalid config to be rejected")
	}
	if svc.DidSave() {
		t.Error("DidSave() = true after a failed save")
	}

	res.Config.Nextcloud.BaseURL = "https://cloud.example.com"
	res.Config.Nextcloud.Username = "user"
	if err := svc.Save(&SaveRequest{Config: res.Config, NewPassword: "pw"}); err != nil {
		t.Fatal(err)
	}
	if !svc.DidSave() {
		t.Error("DidSave() = false after a successful save")
	}
}

func TestPresets(t *testing.T) {
	svc := &Service{}
	presets := svc.Presets()
	for _, key := range []string{"imgur", "catbox", "0x0"} {
		if _, ok := presets[key]; !ok {
			t.Errorf("Presets() missing %q", key)
		}
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

// A rejected Save must not touch stored secrets: the new value is only
// staged, validation fails on another field, and the old secret file keeps
// its content with no staging files left behind.
func TestSaveRejectedKeepsExistingSecrets(t *testing.T) {
	home := testHome(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	svc := &Service{ConfigPath: cfgPath}
	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg := res.Config
	cfg.Nextcloud.BaseURL = "https://cloud.example.com"
	cfg.Nextcloud.Username = "user@example.com"
	if err := svc.Save(&SaveRequest{Config: cfg, NewPassword: "old-pw", NewS3SecretKey: "old-s3"}); err != nil {
		t.Fatal(err)
	}
	pwFile := config.ExpandHome(cfg.Nextcloud.PasswordFile)
	s3File := config.ExpandHome(cfg.S3.SecretKeyFile)

	res, err = svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	bad := res.Config
	bad.Nextcloud.BaseURL = "" // fails validation
	err = svc.Save(&SaveRequest{Config: bad, NewPassword: "new-pw", NewS3SecretKey: "new-s3"})
	var se *SaveError
	if !errors.As(err, &se) {
		t.Fatalf("Save err = %v, want *SaveError", err)
	}
	if se.Field != "Nextcloud.BaseURL" || se.Problem == "" {
		t.Errorf("SaveError = %+v, want field Nextcloud.BaseURL with a problem", se)
	}
	for file, want := range map[string]string{pwFile: "old-pw", s3File: "old-s3"} {
		b, err := os.ReadFile(file)
		if err != nil || strings.TrimSpace(string(b)) != want {
			t.Errorf("%s = %q (err %v), want %q kept after a rejected save", file, b, err, want)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(home, "*", "*.pending-*")); len(left) > 0 {
		t.Errorf("staged secrets left behind: %v", left)
	}
	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("installed config no longer loads: %v", err)
	}
	if loaded.Password() != "old-pw" {
		t.Errorf("password = %q, want old-pw", loaded.Password())
	}
}

// Save refuses a hotkey the host could not bind and a chord used twice,
// naming the field at fault.
func TestSaveRejectsBadAndDuplicateHotkeys(t *testing.T) {
	testHome(t)
	svc := &Service{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}
	// Mimics the Windows/Linux host: Cmd is an alias for Ctrl.
	svc.CheckHotkey = func(chord string) (string, error) {
		if strings.Contains(strings.ToLower(chord), "printscreen") {
			return "", errors.New("no name for it")
		}
		return strings.ReplaceAll(strings.ToLower(chord), "cmd", "ctrl"), nil
	}
	for _, tc := range []struct {
		name  string
		set   func(h *config.HotkeysConfig)
		field string
	}{
		{"unbindable", func(h *config.HotkeysConfig) { h.Window = "Ctrl+PrintScreen" }, "Hotkeys.Window"},
		{"duplicate", func(h *config.HotkeysConfig) { h.Region = "Ctrl+Shift+5"; h.Quit = "shift+ctrl+5" }, "Hotkeys.Quit"},
		{"alias duplicate", func(h *config.HotkeysConfig) { h.Region = "Cmd+Shift+5"; h.Window = "Control+Shift+5" }, "Hotkeys.Window"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := svc.Load()
			if err != nil {
				t.Fatal(err)
			}
			res.Config.Hotkeys = config.HotkeysConfig{}
			tc.set(&res.Config.Hotkeys)
			off := false
			res.Config.Upload.Enabled = &off
			err = svc.Save(&SaveRequest{Config: res.Config})
			var se *SaveError
			if !errors.As(err, &se) || se.Field != tc.field {
				t.Fatalf("Save err = %v (%+v), want SaveError on %s", err, se, tc.field)
			}
		})
	}
}

// A failure after validation (here: the config file cannot be replaced)
// puts the previous secrets back, so "Not saved" is true for them too.
func TestSaveInstallFailureRestoresSecrets(t *testing.T) {
	testHome(t)
	svc := &Service{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}
	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	off := false
	res.Config.Upload.Enabled = &off
	if err := svc.Save(&SaveRequest{Config: res.Config, NewPassword: "old-pw"}); err != nil {
		t.Fatal(err)
	}
	pwFile := config.ExpandHome(res.Config.Nextcloud.PasswordFile)
	s3File := config.ExpandHome(res.Config.S3.SecretKeyFile)

	// A non-empty directory where the config file should go: the final
	// rename fails after both secrets were already moved into place.
	blocked := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.MkdirAll(filepath.Join(blocked, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	svc2 := &Service{ConfigPath: blocked}
	err = svc2.Save(&SaveRequest{Config: res.Config, NewPassword: "new-pw", NewS3SecretKey: "new-s3"})
	if err == nil {
		t.Fatal("Save over a directory succeeded")
	}
	if b, _ := os.ReadFile(pwFile); strings.TrimSpace(string(b)) != "old-pw" {
		t.Errorf("password file = %q after failed install, want old-pw", b)
	}
	if _, err := os.Stat(s3File); !os.IsNotExist(err) {
		t.Errorf("s3 secret created by a failed install (stat err %v), want absent", err)
	}
}

// A tray upload toggle made while the settings window was open survives a
// Save that did not touch "Upload captures".
func TestSaveKeepsTrayUploadToggle(t *testing.T) {
	testHome(t)
	svc := &Service{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}
	res, err := svc.Load()
	if err != nil {
		t.Fatal(err)
	}
	off := false
	res.Config.Upload.Enabled = &off
	if err := svc.Save(&SaveRequest{Config: res.Config}); err != nil {
		t.Fatal(err)
	}

	form, err := svc.Load() // the window opens: uploads off
	if err != nil {
		t.Fatal(err)
	}
	// Tray toggle while it is open (a Catbox destination needs no setup).
	form.Config.Upload.Destination = "catbox"
	if err := svc.Save(&SaveRequest{Config: form.Config}); err != nil {
		t.Fatal(err)
	}
	if err := config.SetUploadEnabledFile(svc.ConfigPath, true); err != nil {
		t.Fatal(err)
	}
	form.Config.Hotkeys.Region = "Ctrl+Shift+7" // an unrelated edit
	if err := svc.Save(&SaveRequest{Config: form.Config}); err != nil {
		t.Fatal(err)
	}
	got, err := config.LoadRaw(svc.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !got.UploadEnabled() {
		t.Error("Save from the open window undid the tray upload toggle")
	}
}
