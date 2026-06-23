package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// StarterConfig is written on first run so the app never fails merely because it
// has not been configured yet. The user only needs to put their Nextcloud app
// password in the file referenced by password_file.
const StarterConfig = `# GoShareIt configuration. Review base_url/username, then put your Nextcloud
# app password in the file referenced by password_file (a leading ~ is expanded).
nextcloud:
  base_url: https://cloud.rake.pro
  username: imgshare@rake.pro
  dav_user: ""
  password_file: ~/.config/goshareit/app-password.secret
  remote_dir: ""

upload:
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

hotkeys:
  region: "Cmd+Shift+1"
  fullscreen: "Cmd+Shift+9"
  window: "Cmd+Shift+0"
  record: "Cmd+Shift+R"
  quit: "Cmd+Shift+Q"

logging:
  level: "info"
`

// DefaultConfigPath returns the canonical writable config location.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: locate home dir: %w", err)
	}
	return filepath.Join(home, ".config", "goshareit", "config.yaml"), nil
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
		if err := os.WriteFile(configPath, []byte(StarterConfig), 0o600); err != nil {
			return "", fmt.Errorf("config: write starter %s: %w", configPath, err)
		}
	}
	secretPath = filepath.Join(dir, "app-password.secret")
	if _, err := os.Stat(secretPath); os.IsNotExist(err) {
		if err := os.WriteFile(secretPath, nil, 0o600); err != nil {
			return "", fmt.Errorf("config: create secret %s: %w", secretPath, err)
		}
	}
	return secretPath, nil
}
