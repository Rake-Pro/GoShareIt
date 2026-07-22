// Package settings is the backend of the goshareit-settings UI: it loads the
// raw config for editing and saves it back with validation, handling the
// secret files (Nextcloud app password, GitHub token) that never live inline
// in the YAML. Pure Go and GUI-free so it is testable on linux.
package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
)

// Service is bound into the Wails frontend. All methods are invoked from JS.
type Service struct {
	ConfigPath string
	Version    string // app version shown in the UI footer
}

// LoadResult is what the frontend edits. Secrets are never returned - only
// whether they are set.
type LoadResult struct {
	Config      *config.Config `json:"config"`
	ConfigPath  string         `json:"configPath"`
	HasPassword bool           `json:"hasPassword"`
	HasToken    bool           `json:"hasToken"`
	Version     string         `json:"version"`
}

// SaveRequest carries the edited config plus optional new secret values
// (write-only: empty means "leave the current secret untouched").
type SaveRequest struct {
	Config      *config.Config `json:"config"`
	NewPassword string         `json:"newPassword"`
	NewToken    string         `json:"newToken"`
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
	return &LoadResult{
		Config:      cfg,
		ConfigPath:  s.ConfigPath,
		HasPassword: secretPresent(cfg.Nextcloud.PasswordFile, cfg.Nextcloud.PasswordEnv),
		HasToken:    secretPresent(cfg.Update.TokenFile, ""),
		Version:     s.Version,
	}, nil
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
