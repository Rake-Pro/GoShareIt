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

// Plain http would put the basic-auth app password on the wire in cleartext,
// so it is rejected for remote hosts, allowed for local ones, and allowed
// anywhere only behind the explicit opt-in.
func TestValidateBaseURLRequiresTLS(t *testing.T) {
	cases := []struct {
		url           string
		allowInsecure bool
		wantErr       bool
	}{
		{"https://cloud.example.com", false, false},
		{"http://cloud.example.com", false, true},
		{"http://cloud.example.com", true, false},
		{"http://localhost:8080", false, false},
		{"http://127.0.0.1:8080", false, false},
		{"http://[::1]:8080", false, false},
		{"http://192.168.1.50", false, false},
		{"http://10.0.0.5", false, false},
		{"http://172.16.4.4", false, false},
		{"http://8.8.8.8", false, true},
		{"ftp://cloud.example.com", true, true},
		{"cloud.example.com", false, true},
		{"https://", false, true},
	}
	for _, tc := range cases {
		err := ValidateBaseURL("nextcloud.base_url", tc.url, tc.allowInsecure)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateBaseURL(%q, allowInsecure=%v) = %v, wantErr=%v", tc.url, tc.allowInsecure, err, tc.wantErr)
		}
	}
}

// The opt-in has to actually take effect through a loaded config, and webdav
// must be held to the same rule as nextcloud.
func TestInsecureBaseURLOptIn(t *testing.T) {
	dir := t.TempDir()
	pwPath := writeFile(t, dir, "pw.secret", "p")
	body := "nextcloud:\n  base_url: http://cloud.example.com\n  username: u\n  password_file: " + pwPath + "\n"

	cfgPath := writeFile(t, dir, "config.yaml", body)
	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error on plain http nextcloud.base_url")
	}

	optIn := writeFile(t, dir, "optin.yaml", body+"upload:\n  allow_insecure_http: true\n")
	if _, err := Load(optIn); err != nil {
		t.Fatalf("allow_insecure_http should permit http: %v", err)
	}

	dav := writeFile(t, dir, "dav.yaml", "upload:\n  destination: webdav\nwebdav:\n  base_url: http://dav.example.com\n")
	if _, err := Load(dav); err == nil {
		t.Fatal("expected error on plain http webdav.base_url")
	}
}

// custom.url gets the same rule: the resolved secret is substituted into the
// request headers, so the endpoint carries a credential too. Its {name}/{mime}
// placeholders must still parse.
func TestInsecureCustomURL(t *testing.T) {
	dir := t.TempDir()
	secret := writeFile(t, dir, "custom.secret", "tok")
	body := func(u string) string {
		return "upload:\n  destination: custom\ncustom:\n  url: " + u + "\n  secret_file: " + secret + "\n"
	}

	bad := writeFile(t, dir, "bad.yaml", body("http://up.example.com/{name}"))
	if _, err := Load(bad); err == nil {
		t.Fatal("expected error on plain http custom.url")
	}

	tmpl := writeFile(t, dir, "tmpl.yaml", body("https://up.example.com/upload/{name}?type={mime}"))
	if _, err := Load(tmpl); err != nil {
		t.Fatalf("placeholders must not break URL validation: %v", err)
	}

	lan := writeFile(t, dir, "lan.yaml", body("http://192.168.1.20:8080/upload"))
	if _, err := Load(lan); err != nil {
		t.Fatalf("LAN custom.url should be allowed: %v", err)
	}

	optIn := writeFile(t, dir, "optin.yaml",
		"upload:\n  destination: custom\n  allow_insecure_http: true\ncustom:\n  url: http://up.example.com\n  secret_file: "+secret+"\n")
	if _, err := Load(optIn); err != nil {
		t.Fatalf("allow_insecure_http should permit http: %v", err)
	}
}
