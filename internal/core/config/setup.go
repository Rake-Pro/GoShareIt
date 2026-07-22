package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// StarterConfig is written on first run so the app never fails merely because it
// has not been configured yet. The user only needs to put their Nextcloud app
// password in the file referenced by password_file. {confdir} is rendered to the
// per-OS app root by WriteStarter.
const StarterConfig = `# GoShareIt configuration. Set base_url/username for your Nextcloud server,
# then put an app password in the file referenced by password_file (a leading
# ~ is expanded) - or just use "Sign in with browser" in the settings UI.
nextcloud:
  base_url: ""          # e.g. https://cloud.example.com
  username: ""          # e.g. you@example.com
  dav_user: ""
  password_file: {confdir}/app-password.secret
  remote_dir: ""

upload:
  enabled: true           # false = local-only mode (Nextcloud section optional)
  direct_link: true
  filename_template: "goshareit_{datetime}_{rand}.{ext}"
  share_expire_days: 0
  share_password: ""

after_capture:
  copy_image_to_clipboard: false
  save_local: false
  save_dir: ""

after_upload:
  copy_url_to_clipboard: true
  notify: true

# Each hotkey may hold comma-separated alternatives, e.g.
#   region: "Cmd+Shift+1, Ctrl+PrintScreen"
# The *_edit variants capture the same way but always open the annotation
# editor, regardless of the editor settings below.
hotkeys:
  region: "{region_hotkey}"
  fullscreen: "{mod}+Shift+9"
  window: "{mod}+Shift+0"
  region_edit: ""
  fullscreen_edit: ""
  window_edit: ""
  upload_toggle: ""
  record: "{mod}+Shift+R"
  quit: "{mod}+Shift+Q"

editor:
  enabled: false          # master switch; false -> current behavior (no editor)
  on_modes: [region]      # which capture modes open the editor (region|fullscreen|window)
  helper_path: ""         # "" -> goshareit-editor next to the host binary
  timeout_seconds: 0      # 0 -> no timeout
  default_tool: arrow
  stroke_width: 3
  color: "#ff3b30"
  tools: [crop, arrow, rect, text, blur, highlight, step]

update:
  enabled: true           # self-update from GitHub Releases
  repo: Rake-Pro/GoShareIt
  # Optional while the repo is private: a fine-grained read-only PAT
  # (Contents: Read on the repo above). Missing/empty file -> anonymous API,
  # which works once the repo is public.
  token_file: {confdir}/github-token.secret
  interval_hours: 24

logging:
  level: "info"
`

// dirName is the app root folder name in the user's home directory: dotted on
// unix/macOS, undotted on Windows (dot-prefixed folders are alien there).
func dirName() string {
	if runtime.GOOS == "windows" {
		return "goshareit"
	}
	return ".goshareit"
}

// Dir returns the app's per-user root (config, secrets, history):
// ~/.goshareit on macOS/Linux, %USERPROFILE%\goshareit on Windows.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: locate home dir: %w", err)
	}
	return filepath.Join(home, dirName()), nil
}

// StarterYAML renders the starter config template for the current OS: {mod}
// is the native primary modifier (Cmd/Ctrl - the windows backend maps
// Cmd->Ctrl anyway, but the config should show what the user actually
// presses), region gets the platform-conventional chord on windows, and
// {confdir} becomes the per-OS app root.
func StarterYAML() string {
	mod, region := "Cmd", "Cmd+Shift+1"
	if runtime.GOOS == "windows" {
		mod, region = "Ctrl", "Win+Ctrl+PrintScreen"
	}
	r := strings.ReplaceAll(StarterConfig, "{confdir}", "~/"+dirName())
	r = strings.ReplaceAll(r, "{mod}", mod)
	return strings.ReplaceAll(r, "{region_hotkey}", region)
}

// StarterDefaults parses the rendered starter template into a Config. This is
// the canonical "factory defaults" object (settings UI reset).
func StarterDefaults() (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(StarterYAML()), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse starter: %w", err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

// DefaultConfigPath returns the canonical writable config location.
func DefaultConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// WriteStarter creates the config dir, writes StarterConfig to configPath if it
// does not exist, and creates an empty 0600 app-password file beside it if
// missing. It returns the secret file path so the caller can tell the user where
// to put the password. Existing files are never overwritten.
func WriteStarter(configPath string) (secretPath string, err error) {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("config: create %s: %w", dir, err)
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(StarterYAML()), 0o600); err != nil {
			return "", fmt.Errorf("config: write starter %s: %w", configPath, err)
		}
	}
	secretPath = filepath.Join(dir, "app-password.secret")
	if _, err := os.Stat(secretPath); os.IsNotExist(err) {
		if err := os.WriteFile(secretPath, nil, 0o600); err != nil {
			return "", fmt.Errorf("config: create secret %s: %w", secretPath, err)
		}
	}
	// Optional GitHub token for self-update while the repo is private. An empty
	// file is fine (anonymous API); scaffolded so the path in the starter config
	// always exists with sane permissions.
	tokenPath := filepath.Join(dir, "github-token.secret")
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		if err := os.WriteFile(tokenPath, nil, 0o600); err != nil {
			return "", fmt.Errorf("config: create secret %s: %w", tokenPath, err)
		}
	}
	return secretPath, nil
}
