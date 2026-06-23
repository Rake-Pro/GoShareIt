// Package config loads and validates GoShareIt configuration. The Nextcloud
// password is never stored inline; it is resolved from a file or env var.
package config

import (
	"fmt"
	"os"
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
	Logging      LoggingConfig      `yaml:"logging"`

	// password is resolved at load time, never serialized.
	password string `yaml:"-"`
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
	Quit       string `yaml:"quit"`
}

// LoggingConfig controls logging.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// Password returns the resolved Nextcloud password.
func (c *Config) Password() string { return c.password }

// Load reads and validates config from path. If the GOSHAREIT_CONFIG_PATH env
// var is set it overrides path. Defaults are applied before validation.
func Load(path string) (*Config, error) {
	if env := os.Getenv(EnvConfigPath); env != "" {
		path = env
	}
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
}

func (c *Config) resolvePassword() error {
	file := strings.TrimSpace(c.Nextcloud.PasswordFile)
	env := strings.TrimSpace(c.Nextcloud.PasswordEnv)
	switch {
	case file != "" && env != "":
		return fmt.Errorf("config: set exactly one of nextcloud.password_file or nextcloud.password_env, not both")
	case file != "":
		b, err := os.ReadFile(file)
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
