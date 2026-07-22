// Package config loads and validates GoShareIt configuration. The Nextcloud
// password is never stored inline; it is resolved from a file or env var.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvConfigPath overrides the config file path when set.
const EnvConfigPath = "GOSHAREIT_CONFIG_PATH"

// Config is the root configuration.
type Config struct {
	Nextcloud    NextcloudConfig    `yaml:"nextcloud"`
	Upload       UploadConfig       `yaml:"upload"`
	AfterCapture AfterCaptureConfig `yaml:"after_capture"`
	AfterUpload  AfterUploadConfig  `yaml:"after_upload"`
	Hotkeys      HotkeysConfig      `yaml:"hotkeys"`
	Editor       EditorConfig       `yaml:"editor"`
	Update       UpdateConfig       `yaml:"update"`
	Logging      LoggingConfig      `yaml:"logging"`

	// password and updateToken are resolved at load time, never serialized.
	password    string `yaml:"-"`
	updateToken string `yaml:"-"`
}

// NextcloudConfig holds connection settings. The password itself comes from
// PasswordFile or PasswordEnv, never inline.
type NextcloudConfig struct {
	BaseURL      string `yaml:"base_url"`
	Username     string `yaml:"username"`
	DavUser      string `yaml:"dav_user"`
	PasswordFile string `yaml:"password_file"`
	PasswordEnv  string `yaml:"password_env"`
	RemoteDir    string `yaml:"remote_dir"`
}

// UploadConfig controls naming and sharing.
type UploadConfig struct {
	DirectLink       bool   `yaml:"direct_link"`
	FilenameTemplate string `yaml:"filename_template"`
	ShareExpireDays  int    `yaml:"share_expire_days"`
	SharePassword    string `yaml:"share_password"`
}

// AfterCaptureConfig controls post-capture, pre-upload behavior.
type AfterCaptureConfig struct {
	CopyImageToClipboard bool   `yaml:"copy_image_to_clipboard"`
	SaveLocal            bool   `yaml:"save_local"`
	SaveDir              string `yaml:"save_dir"`
}

// AfterUploadConfig controls post-upload behavior.
type AfterUploadConfig struct {
	CopyURLToClipboard bool `yaml:"copy_url_to_clipboard"`
	Notify             bool `yaml:"notify"`
}

// HotkeysConfig holds declarative hotkey bindings.
type HotkeysConfig struct {
	Region     string `yaml:"region"`
	FullScreen string `yaml:"fullscreen"`
	Window     string `yaml:"window"`
	Record     string `yaml:"record"`
	Quit       string `yaml:"quit"`
}

// EditorConfig controls the optional post-capture annotation editor. When
// Enabled is false (default) the app uses a NoopEditor and behavior is unchanged.
type EditorConfig struct {
	Enabled        bool     `yaml:"enabled"`
	OnModes        []string `yaml:"on_modes"`
	HelperPath     string   `yaml:"helper_path"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
	DefaultTool    string   `yaml:"default_tool"`
	StrokeWidth    int      `yaml:"stroke_width"`
	Color          string   `yaml:"color"`
	Tools          []string `yaml:"tools"`
}

// UpdateConfig controls self-update from GitHub Releases. Enabled defaults to
// true; TokenFile is optional and only needed while the repo is private (a
// fine-grained read-only PAT; the file lives beside the other secrets and is
// provisioned per machine, never committed anywhere).
type UpdateConfig struct {
	Enabled       *bool  `yaml:"enabled"`
	Repo          string `yaml:"repo"`
	TokenFile     string `yaml:"token_file"`
	IntervalHours int    `yaml:"interval_hours"`
}

// UpdateEnabled reports the effective enabled state (default true).
func (c *Config) UpdateEnabled() bool {
	return c.Update.Enabled == nil || *c.Update.Enabled
}

// UpdateToken returns the resolved GitHub token ("" when not configured).
func (c *Config) UpdateToken() string { return c.updateToken }

// LoggingConfig controls logging.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// Password returns the resolved Nextcloud password.
func (c *Config) Password() string { return c.password }

// expandHome expands a leading ~ or ~/ to the user's home directory. Go does
// not do this automatically, so config paths like ~/.config/... need it.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return h
			}
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

// NewDefault returns an empty Config with defaults applied (no file read).
func NewDefault() *Config {
	var c Config
	c.applyDefaults()
	return &c
}

// LoadRaw parses the config file and applies defaults, but skips secret
// resolution and validation. The settings UI uses it so an incomplete or
// not-yet-valid config is still editable.
func LoadRaw(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

// ExpandHome exposes tilde expansion for callers that handle config-relative
// secret paths (the settings service).
func ExpandHome(p string) string { return expandHome(p) }

// Load reads and validates config from path. If the GOSHAREIT_CONFIG_PATH env
// var is set it overrides path. Defaults are applied before validation.
func Load(path string) (*Config, error) {
	if env := os.Getenv(EnvConfigPath); env != "" {
		path = env
	}
	return LoadFile(path)
}

// LoadFile is Load without the env-var override: it validates exactly the
// given file. The settings service uses it to vet a candidate config before
// installing it, where the override would silently validate the wrong file.
func LoadFile(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config: no path provided")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.resolvePassword(); err != nil {
		return nil, err
	}
	if err := cfg.resolveUpdateToken(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Upload.FilenameTemplate == "" {
		c.Upload.FilenameTemplate = "goshareit_{datetime}_{rand}.{ext}"
	}
	if c.Nextcloud.DavUser == "" && c.Nextcloud.Username != "" {
		if i := strings.IndexByte(c.Nextcloud.Username, '@'); i >= 0 {
			c.Nextcloud.DavUser = c.Nextcloud.Username[:i]
		} else {
			c.Nextcloud.DavUser = c.Nextcloud.Username
		}
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Update.Repo == "" {
		c.Update.Repo = "Rake-Pro/GoShareIt"
	}
	if c.Update.IntervalHours <= 0 {
		c.Update.IntervalHours = 24
	}
}

// resolveUpdateToken reads the optional GitHub token file. A missing or empty
// file is not an error: the updater then calls the API anonymously, which
// works once the repo is public.
func (c *Config) resolveUpdateToken() error {
	file := strings.TrimSpace(c.Update.TokenFile)
	if file == "" {
		return nil
	}
	b, err := os.ReadFile(expandHome(file))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("config: read update.token_file %s: %w", file, err)
	}
	c.updateToken = strings.TrimSpace(string(b))
	return nil
}

func (c *Config) resolvePassword() error {
	file := strings.TrimSpace(c.Nextcloud.PasswordFile)
	env := strings.TrimSpace(c.Nextcloud.PasswordEnv)
	switch {
	case file != "" && env != "":
		return fmt.Errorf("config: set exactly one of nextcloud.password_file or nextcloud.password_env, not both")
	case file != "":
		b, err := os.ReadFile(expandHome(file))
		if err != nil {
			return fmt.Errorf("config: read password_file %s: %w", file, err)
		}
		pw := strings.TrimSpace(string(b))
		if pw == "" {
			return fmt.Errorf("config: password_file %s is empty", file)
		}
		c.password = pw
	case env != "":
		pw := os.Getenv(env)
		if pw == "" {
			return fmt.Errorf("config: password_env %s is unset or empty", env)
		}
		c.password = pw
	default:
		return fmt.Errorf("config: set exactly one of nextcloud.password_file or nextcloud.password_env")
	}
	return nil
}

func (c *Config) validate() error {
	if c.Nextcloud.BaseURL == "" {
		return fmt.Errorf("config: nextcloud.base_url is required")
	}
	if !strings.HasPrefix(c.Nextcloud.BaseURL, "http://") && !strings.HasPrefix(c.Nextcloud.BaseURL, "https://") {
		return fmt.Errorf("config: nextcloud.base_url must start with http:// or https://")
	}
	if c.Nextcloud.Username == "" {
		return fmt.Errorf("config: nextcloud.username is required")
	}
	if c.Nextcloud.DavUser == "" {
		return fmt.Errorf("config: nextcloud.dav_user could not be derived; set it explicitly")
	}
	if c.Upload.ShareExpireDays < 0 {
		return fmt.Errorf("config: upload.share_expire_days must be >= 0")
	}
	return nil
}
