// Package settings is the backend of the goshareit-settings UI: it loads the
// raw config for editing and saves it back with validation, handling the
// secret files (Nextcloud app password, GitHub token) that never live inline
// in the YAML. Pure Go and GUI-free so it is testable on linux.
package settings

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

// Service is bound into the Wails frontend. All methods are invoked from JS.
// PickDir and OpenURL are injected by the GUI shell (native dialogs/browser);
// they stay nil in tests and get portable fallbacks where possible.
type Service struct {
	ConfigPath string
	Version    string // app version shown in the UI footer

	PickDir func() (string, error) // native directory picker
	OpenURL func(url string) error // native browser open; nil -> osOpenURL
	Close   func()                 // close the settings window; nil in tests
}

// LoadResult is what the frontend edits. Secrets are never returned - only
// whether they are set.
type LoadResult struct {
	Config      *config.Config `json:"config"`
	ConfigPath  string         `json:"configPath"`
	HasPassword bool           `json:"hasPassword"`
	HasToken    bool           `json:"hasToken"`
	// Destination secrets, one flag per non-Nextcloud secret below.
	HasS3SecretKey    bool   `json:"hasS3SecretKey"`
	HasSFTPPassword   bool   `json:"hasSFTPPassword"`
	HasSFTPPassphrase bool   `json:"hasSFTPPassphrase"`
	HasWebDAVPassword bool   `json:"hasWebDAVPassword"`
	HasCustomSecret   bool   `json:"hasCustomSecret"`
	Version           string `json:"version"`
}

// SaveRequest carries the edited config plus optional new secret values
// (write-only: empty means "leave the current secret untouched").
type SaveRequest struct {
	Config      *config.Config `json:"config"`
	NewPassword string         `json:"newPassword"`
	NewToken    string         `json:"newToken"`
	// Destination secrets, one write-only field per non-Nextcloud secret.
	NewS3SecretKey    string `json:"newS3SecretKey"`
	NewSFTPPassword   string `json:"newSFTPPassword"`
	NewSFTPPassphrase string `json:"newSFTPPassphrase"`
	NewWebDAVPassword string `json:"newWebDAVPassword"`
	NewCustomSecret   string `json:"newCustomSecret"`
}

// Load reads the config for editing. A missing file yields the starter
// defaults so first-run users edit a sensible template.
func (s *Service) Load() (*LoadResult, error) {
	cfg, err := config.LoadRaw(s.ConfigPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		cfg = config.NewDefault()
	}
	applyEditingDefaults(cfg)
	return s.loadResult(cfg), nil
}

// loadResult builds a LoadResult for cfg: the secret presence flags, never
// the secrets themselves.
func (s *Service) loadResult(cfg *config.Config) *LoadResult {
	return &LoadResult{
		Config:            cfg,
		ConfigPath:        s.ConfigPath,
		HasPassword:       secretPresent(cfg.Nextcloud.PasswordFile, cfg.Nextcloud.PasswordEnv),
		HasToken:          secretPresent(cfg.Update.TokenFile, ""),
		HasS3SecretKey:    secretPresent(cfg.S3.SecretKeyFile, cfg.S3.SecretKeyEnv),
		HasSFTPPassword:   secretPresent(cfg.SFTP.PasswordFile, cfg.SFTP.PasswordEnv),
		HasSFTPPassphrase: secretPresent(cfg.SFTP.PassphraseFile, cfg.SFTP.PassphraseEnv),
		HasWebDAVPassword: secretPresent(cfg.WebDAV.PasswordFile, cfg.WebDAV.PasswordEnv),
		HasCustomSecret:   secretPresent(cfg.Custom.SecretFile, cfg.Custom.SecretEnv),
		Version:           s.Version,
	}
}

// Save persists the edited config: writes any new secrets to their files,
// marshals the YAML (0600), then runs the full loader so the user gets real
// validation errors immediately instead of at next app start.
func (s *Service) Save(req *SaveRequest) error {
	if req == nil || req.Config == nil {
		return fmt.Errorf("settings: empty save request")
	}
	cfg := req.Config
	applyEditingDefaults(cfg)

	if req.NewPassword != "" {
		if cfg.Nextcloud.PasswordEnv != "" {
			return fmt.Errorf("settings: password is sourced from env var %s; unset password_env to use a file", cfg.Nextcloud.PasswordEnv)
		}
		if err := writeSecret(cfg.Nextcloud.PasswordFile, req.NewPassword); err != nil {
			return err
		}
	}
	if req.NewToken != "" {
		if err := writeSecret(cfg.Update.TokenFile, req.NewToken); err != nil {
			return err
		}
	}
	if err := saveSecretField(req.NewS3SecretKey, "s3 secret key", cfg.S3.SecretKeyFile, cfg.S3.SecretKeyEnv); err != nil {
		return err
	}
	if err := saveSecretField(req.NewSFTPPassword, "sftp password", cfg.SFTP.PasswordFile, cfg.SFTP.PasswordEnv); err != nil {
		return err
	}
	if err := saveSecretField(req.NewSFTPPassphrase, "sftp passphrase", cfg.SFTP.PassphraseFile, cfg.SFTP.PassphraseEnv); err != nil {
		return err
	}
	if err := saveSecretField(req.NewWebDAVPassword, "webdav password", cfg.WebDAV.PasswordFile, cfg.WebDAV.PasswordEnv); err != nil {
		return err
	}
	if err := saveSecretField(req.NewCustomSecret, "custom secret", cfg.Custom.SecretFile, cfg.Custom.SecretEnv); err != nil {
		return err
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("settings: marshal config: %w", err)
	}
	header := "# GoShareIt configuration. Managed by the settings UI; comments are not preserved.\n"
	dir := filepath.Dir(s.ConfigPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("settings: create config dir: %w", err)
	}

	// Validate BEFORE installing: an invalid config must never reach the real
	// path - the host restarts on config mtime change and would fatal on load,
	// leaving the app dead until the file is hand-edited. Same-dir temp file +
	// rename keeps the install atomic.
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("settings: temp config: %w", err)
	}
	_, werr := tmp.Write(append([]byte(header), out...))
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("settings: write temp config: write=%v close=%v", werr, cerr)
	}
	if _, err := config.LoadFile(tmp.Name()); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("not saved - the config does not validate: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.ConfigPath); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("settings: install config: %w", err)
	}
	return nil
}

// CloseWindow closes the settings window. The frontend calls it after a
// successful save so the host (which blocks on this process and applies the
// config on exit) restarts immediately instead of waiting for the user to
// close the window by hand.
func (s *Service) CloseWindow() error {
	if s.Close == nil {
		return fmt.Errorf("settings: close is not available")
	}
	s.Close()
	return nil
}

// PickDirectory opens the native directory picker and returns the chosen
// path ("" on cancel).
func (s *Service) PickDirectory() (string, error) {
	if s.PickDir == nil {
		return "", fmt.Errorf("settings: no directory picker available")
	}
	return s.PickDir()
}

// ResetDefaults returns the factory-default config for this OS. Nothing is
// persisted - the frontend swaps its model and the user must still Save.
// Secret files on disk are untouched, so their presence flags carry over.
func (s *Service) ResetDefaults() (*LoadResult, error) {
	cfg, err := config.StarterDefaults()
	if err != nil {
		return nil, err
	}
	applyEditingDefaults(cfg)
	return s.loadResult(cfg), nil
}

// Presets returns the built-in custom-uploader starting points (imgur,
// catbox, 0x0) for the settings UI's preset picker, so the data lives in one
// place instead of being duplicated in JS.
func (s *Service) Presets() map[string]upload.CustomConfig {
	return upload.CustomPresets()
}

// LoginResult carries the outcome of a browser sign-in back to the frontend.
// The app password is NOT persisted here - the frontend submits it as
// SaveRequest.NewPassword, so "nothing is applied until Save" stays true.
type LoginResult struct {
	LoginName   string `json:"loginName"`
	AppPassword string `json:"appPassword"`
}

// BrowserLogin runs the Nextcloud Login Flow v2 against baseURL: opens the
// browser (where the server-side auth - password, OIDC/SSO, 2FA - happens),
// waits for completion, and returns the minted credential. Blocks up to 5
// minutes. Persisting is deferred to Save.
func (s *Service) BrowserLogin(baseURL string) (*LoginResult, error) {
	baseURL = strings.TrimSpace(baseURL)
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("enter the server URL first (https://...)")
	}
	cur, err := s.Load()
	if err != nil {
		return nil, err
	}
	if cur.Config.Nextcloud.PasswordEnv != "" {
		return nil, fmt.Errorf("password is sourced from env var %s; unset password_env to use browser sign-in", cur.Config.Nextcloud.PasswordEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}

	start, err := startLoginFlow(ctx, client, baseURL)
	if err != nil {
		return nil, err
	}
	openURL := s.OpenURL
	if openURL == nil {
		openURL = osOpenURL
	}
	if err := openURL(start.Login); err != nil {
		return nil, err
	}
	result, err := pollLoginFlow(ctx, client, start, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return &LoginResult{LoginName: result.LoginName, AppPassword: result.AppPassword}, nil
}

// applyEditingDefaults fills the fields the UI relies on: secret file paths
// default into the app root, and the update section gets an explicit enabled.
func applyEditingDefaults(cfg *config.Config) {
	dir, err := config.Dir()
	if err != nil {
		return
	}
	// Tilde form keeps the YAML portable across machines.
	tilde := func(name string) string { return "~/" + filepath.Base(dir) + "/" + name }
	if cfg.Nextcloud.PasswordFile == "" && cfg.Nextcloud.PasswordEnv == "" {
		cfg.Nextcloud.PasswordFile = tilde("app-password.secret")
	}
	if cfg.Update.TokenFile == "" {
		cfg.Update.TokenFile = tilde("github-token.secret")
	}
	if cfg.Update.Enabled == nil {
		t := true
		cfg.Update.Enabled = &t
	}
	if cfg.Upload.Enabled == nil {
		t := true
		cfg.Upload.Enabled = &t
	}
	if cfg.S3.SecretKeyFile == "" && cfg.S3.SecretKeyEnv == "" {
		cfg.S3.SecretKeyFile = tilde("s3-secret-key.secret")
	}
	if cfg.SFTP.PasswordFile == "" && cfg.SFTP.PasswordEnv == "" {
		cfg.SFTP.PasswordFile = tilde("sftp-password.secret")
	}
	if cfg.SFTP.PassphraseFile == "" && cfg.SFTP.PassphraseEnv == "" {
		cfg.SFTP.PassphraseFile = tilde("sftp-key-passphrase.secret")
	}
	if cfg.WebDAV.PasswordFile == "" && cfg.WebDAV.PasswordEnv == "" {
		cfg.WebDAV.PasswordFile = tilde("webdav-password.secret")
	}
	if cfg.Custom.SecretFile == "" && cfg.Custom.SecretEnv == "" {
		cfg.Custom.SecretFile = tilde("custom-secret.secret")
	}
}

// saveSecretField writes value to file, unless value is empty (nothing to
// do) or env is set (the secret is env-sourced, so writing a file would be
// silently ignored at load - same rule as the Nextcloud password).
func saveSecretField(value, label, file, env string) error {
	if value == "" {
		return nil
	}
	if env != "" {
		return fmt.Errorf("settings: %s is sourced from env var %s; unset the corresponding _env setting to use a file", label, env)
	}
	return writeSecret(file, value)
}

func writeSecret(path, value string) error {
	if path == "" {
		return fmt.Errorf("settings: no secret file path configured")
	}
	full := config.ExpandHome(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("settings: create secret dir: %w", err)
	}
	if err := os.WriteFile(full, []byte(strings.TrimSpace(value)+"\n"), 0o600); err != nil {
		return fmt.Errorf("settings: write secret %s: %w", full, err)
	}
	return nil
}

// secretPresent reports whether a secret is effectively configured: a
// non-empty file at path, or a non-empty env var.
func secretPresent(file, env string) bool {
	if env != "" {
		return os.Getenv(env) != ""
	}
	if file == "" {
		return false
	}
	b, err := os.ReadFile(config.ExpandHome(file))
	return err == nil && len(strings.TrimSpace(string(b))) > 0
}
