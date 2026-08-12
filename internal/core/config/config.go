// Package config loads and validates GoShareIt configuration. The Nextcloud
// password is never stored inline; it is resolved from a file or env var.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvConfigPath overrides the config file path when set.
const EnvConfigPath = "GOSHAREIT_CONFIG_PATH"

// Config is the root configuration.
type Config struct {
	// Theme selects the app-wide UI theme: "light", "dark", or "system"
	// (default/empty = system). Applies to the annotation editor and the
	// settings window.
	Theme string `yaml:"theme"`

	Nextcloud    NextcloudConfig    `yaml:"nextcloud"`
	Upload       UploadConfig       `yaml:"upload"`
	S3           S3Config           `yaml:"s3"`
	SFTP         SFTPConfig         `yaml:"sftp"`
	WebDAV       WebDAVConfig       `yaml:"webdav"`
	Custom       CustomConfig       `yaml:"custom"`
	AfterCapture AfterCaptureConfig `yaml:"after_capture"`
	AfterUpload  AfterUploadConfig  `yaml:"after_upload"`
	Hotkeys      HotkeysConfig      `yaml:"hotkeys"`
	Editor       EditorConfig       `yaml:"editor"`
	Update       UpdateConfig       `yaml:"update"`
	Logging      LoggingConfig      `yaml:"logging"`

	// password, updateToken, and the destination secrets below are resolved
	// at load time, never serialized.
	password          string `yaml:"-"`
	updateToken       string `yaml:"-"`
	s3SecretKey       string `yaml:"-"`
	sftpPassword      string `yaml:"-"`
	sftpPrivateKeyPEM string `yaml:"-"`
	sftpPassphrase    string `yaml:"-"`
	webdavPassword    string `yaml:"-"`
	customSecret      string `yaml:"-"`
}

// validUploadDestinations enumerates upload.destination values.
var validUploadDestinations = map[string]bool{
	"nextcloud": true, "s3": true, "sftp": true, "webdav": true, "custom": true,
}

// S3Config configures the S3-compatible upload destination (upload.destination:
// s3). SecretKey is resolved from exactly one of SecretKeyFile/SecretKeyEnv,
// never inline.
type S3Config struct {
	Endpoint       string `yaml:"endpoint"`
	Region         string `yaml:"region"`
	Bucket         string `yaml:"bucket"`
	AccessKey      string `yaml:"access_key"`
	SecretKeyFile  string `yaml:"secret_key_file"`
	SecretKeyEnv   string `yaml:"secret_key_env"`
	Prefix         string `yaml:"prefix"`
	URLTemplate    string `yaml:"url_template"`
	UsePathStyle   bool   `yaml:"use_path_style"`
	PresignSeconds int    `yaml:"presign_seconds"`
}

// SFTPConfig configures the SFTP upload destination (upload.destination:
// sftp). Password is resolved from PasswordFile/PasswordEnv and used only
// when PrivateKeyFile is empty; PrivateKeyFile's contents are loaded into the
// resolved PEM, optionally decrypted with the resolved passphrase.
type SFTPConfig struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	User               string `yaml:"user"`
	PasswordFile       string `yaml:"password_file"`
	PasswordEnv        string `yaml:"password_env"`
	PrivateKeyFile     string `yaml:"private_key_file"`
	PassphraseFile     string `yaml:"passphrase_file"`
	PassphraseEnv      string `yaml:"passphrase_env"`
	RemoteDir          string `yaml:"remote_dir"`
	URLTemplate        string `yaml:"url_template"`
	HostKeyFingerprint string `yaml:"host_key_fingerprint"`
}

// WebDAVConfig configures the generic WebDAV upload destination
// (upload.destination: webdav). Password is resolved from exactly one of
// PasswordFile/PasswordEnv, never inline.
type WebDAVConfig struct {
	BaseURL      string `yaml:"base_url"`
	Username     string `yaml:"username"`
	PasswordFile string `yaml:"password_file"`
	PasswordEnv  string `yaml:"password_env"`
	RemoteDir    string `yaml:"remote_dir"`
	URLTemplate  string `yaml:"url_template"`
}

// CustomConfig configures a generic HTTP upload destination
// (upload.destination: custom). The resolved secret (SecretFile/SecretEnv)
// substitutes a literal "{secret}" placeholder wherever it appears in Headers
// and ExtraFields values, so tokens never live in the YAML.
type CustomConfig struct {
	Method                string            `yaml:"method"`
	URL                   string            `yaml:"url"`
	Headers               map[string]string `yaml:"headers"`
	Body                  string            `yaml:"body"`
	FileField             string            `yaml:"file_field"`
	ExtraFields           map[string]string `yaml:"extra_fields"`
	SecretFile            string            `yaml:"secret_file"`
	SecretEnv             string            `yaml:"secret_env"`
	ResponseURLPath       string            `yaml:"response_url_path"`
	ResponseDirectURLPath string            `yaml:"response_direct_url_path"`
	ResponseURLRegex      string            `yaml:"response_url_regex"`
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

// UploadConfig controls whether captures upload at all, plus naming and
// sharing. Enabled defaults to true; false = local-only mode, where the
// Nextcloud section (server, credentials) is not required at all and can be
// toggled back on later.
type UploadConfig struct {
	Enabled *bool `yaml:"enabled"`
	// Destination selects the active upload provider: nextcloud, s3, sftp,
	// webdav, or custom (empty defaults to nextcloud). DirectLink,
	// ShareExpireDays, and SharePassword are Nextcloud-only and only take
	// effect for that destination.
	Destination      string `yaml:"destination"`
	DirectLink       bool   `yaml:"direct_link"`
	FilenameTemplate string `yaml:"filename_template"`
	ShareExpireDays  int    `yaml:"share_expire_days"`
	SharePassword    string `yaml:"share_password"`
	// AllowInsecureHTTP opts out of the https requirement on the endpoint URLs
	// (nextcloud.base_url, webdav.base_url, custom.url). Each of those
	// requests carries a credential - basic auth for the first two, the
	// substituted {secret} for custom - so plain http puts it on the wire in
	// cleartext; this exists only for servers that genuinely have no TLS and
	// whose traffic never leaves a trusted network.
	AllowInsecureHTTP bool `yaml:"allow_insecure_http"`
}

// UploadEnabled reports the effective upload state (default true).
func (c *Config) UploadEnabled() bool {
	return c.Upload.Enabled == nil || *c.Upload.Enabled
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

// HotkeysConfig holds declarative hotkey bindings. Every value may hold
// comma-separated alternatives ("Cmd+Shift+1, Ctrl+PrintScreen") - each
// registers independently. The *_edit variants capture the same way but force
// the annotation editor open, independent of editor.enabled/on_modes.
type HotkeysConfig struct {
	Region         string `yaml:"region"`
	FullScreen     string `yaml:"fullscreen"`
	Window         string `yaml:"window"`
	RegionEdit     string `yaml:"region_edit"`
	FullScreenEdit string `yaml:"fullscreen_edit"`
	WindowEdit     string `yaml:"window_edit"`
	UploadToggle   string `yaml:"upload_toggle"`
	Record         string `yaml:"record"`
	Quit           string `yaml:"quit"`
}

// SetUploadEnabledFile flips upload.enabled in the config file in place so a
// runtime toggle survives restart. Comments are not preserved (same tradeoff
// as the settings UI writer).
func SetUploadEnabledFile(path string, enabled bool) error {
	cfg, err := LoadRaw(path)
	if err != nil {
		return err
	}
	cfg.Upload.Enabled = &enabled
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	header := "# GoShareIt configuration. Managed by the settings UI; comments are not preserved.\n"
	if err := os.WriteFile(path, append([]byte(header), out...), 0o600); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
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

// S3SecretKey returns the resolved S3 secret key.
func (c *Config) S3SecretKey() string { return c.s3SecretKey }

// SFTPPassword returns the resolved SFTP password.
func (c *Config) SFTPPassword() string { return c.sftpPassword }

// SFTPPrivateKeyPEM returns the resolved SFTP private key contents (PEM).
func (c *Config) SFTPPrivateKeyPEM() string { return c.sftpPrivateKeyPEM }

// SFTPPassphrase returns the resolved SFTP private key passphrase.
func (c *Config) SFTPPassphrase() string { return c.sftpPassphrase }

// WebDAVPassword returns the resolved WebDAV password.
func (c *Config) WebDAVPassword() string { return c.webdavPassword }

// CustomSecret returns the resolved custom-destination secret.
func (c *Config) CustomSecret() string { return c.customSecret }

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
	if err := cfg.resolveDestinationSecrets(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Upload.Destination == "" {
		c.Upload.Destination = "nextcloud"
	}
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
	if c.Editor.StrokeWidth <= 0 {
		// 6px default: 3px is near-invisible on retina-resolution captures.
		c.Editor.StrokeWidth = 6
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
	if !c.UploadEnabled() || c.Upload.Destination != "nextcloud" {
		// Local-only mode, or another destination is active: credentials are
		// optional. Resolve best-effort so switching back to nextcloud later
		// picks up an existing secret unchanged.
		if file != "" {
			if b, err := os.ReadFile(expandHome(file)); err == nil {
				c.password = strings.TrimSpace(string(b))
			}
		} else if env != "" {
			c.password = os.Getenv(env)
		}
		return nil
	}
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

// resolveSecretPair resolves a secret from exactly one of file/env, the same
// way Nextcloud's password is resolved (tilde expansion, trimmed). When
// required is true, both set, neither set, or an empty resolved value are all
// errors named after fieldPrefix (e.g. "s3.secret_key"). When required is
// false, resolution is best-effort and never fails: file wins over env if
// both happen to be set, and a missing/unreadable file resolves to "".
func resolveSecretPair(fieldPrefix, file, env string, required bool) (string, error) {
	file = strings.TrimSpace(file)
	env = strings.TrimSpace(env)
	if !required {
		if file != "" {
			if b, err := os.ReadFile(expandHome(file)); err == nil {
				return strings.TrimSpace(string(b)), nil
			}
			return "", nil
		}
		if env != "" {
			return os.Getenv(env), nil
		}
		return "", nil
	}
	switch {
	case file != "" && env != "":
		return "", fmt.Errorf("config: set exactly one of %[1]s_file or %[1]s_env, not both", fieldPrefix)
	case file != "":
		b, err := os.ReadFile(expandHome(file))
		if err != nil {
			return "", fmt.Errorf("config: read %s_file %s: %w", fieldPrefix, file, err)
		}
		v := strings.TrimSpace(string(b))
		if v == "" {
			return "", fmt.Errorf("config: %s_file %s is empty", fieldPrefix, file)
		}
		return v, nil
	case env != "":
		v := os.Getenv(env)
		if v == "" {
			return "", fmt.Errorf("config: %s_env %s is unset or empty", fieldPrefix, env)
		}
		return v, nil
	default:
		return "", fmt.Errorf("config: set exactly one of %s_file or %s_env", fieldPrefix, fieldPrefix)
	}
}

// resolveDestinationSecrets resolves the secrets for every non-Nextcloud
// destination. Only the active destination (when uploads are enabled) is
// fail-closed; the rest resolve best-effort so switching destinations later
// picks up an already-configured secret unchanged.
func (c *Config) resolveDestinationSecrets() error {
	active := "" // the active destination, or "" when uploads are disabled
	if c.UploadEnabled() {
		active = c.Upload.Destination
	}

	s3Key, err := resolveSecretPair("s3.secret_key", c.S3.SecretKeyFile, c.S3.SecretKeyEnv, active == "s3")
	if err != nil {
		return err
	}
	c.s3SecretKey = s3Key

	if err := c.resolveSFTPSecrets(active == "sftp"); err != nil {
		return err
	}

	webdavPw, err := resolveSecretPair("webdav.password", c.WebDAV.PasswordFile, c.WebDAV.PasswordEnv, false)
	if err != nil {
		return err
	}
	c.webdavPassword = webdavPw

	customSecret, err := resolveSecretPair("custom.secret", c.Custom.SecretFile, c.Custom.SecretEnv, false)
	if err != nil {
		return err
	}
	c.customSecret = customSecret
	return nil
}

// resolveSFTPSecrets loads the private key contents (if configured) and the
// password/passphrase. The password is required only when sftp is active and
// no private key is configured, since either auth method satisfies "password
// or key".
func (c *Config) resolveSFTPSecrets(active bool) error {
	keyFile := strings.TrimSpace(c.SFTP.PrivateKeyFile)
	if keyFile != "" {
		b, err := os.ReadFile(expandHome(keyFile))
		if err != nil {
			if active {
				return fmt.Errorf("config: read sftp.private_key_file %s: %w", keyFile, err)
			}
		} else {
			c.sftpPrivateKeyPEM = string(b)
		}
	}

	pwRequired := active && c.sftpPrivateKeyPEM == ""
	pw, err := resolveSecretPair("sftp.password", c.SFTP.PasswordFile, c.SFTP.PasswordEnv, pwRequired)
	if err != nil {
		return err
	}
	c.sftpPassword = pw

	passphrase, err := resolveSecretPair("sftp.passphrase", c.SFTP.PassphraseFile, c.SFTP.PassphraseEnv, false)
	if err != nil {
		return err
	}
	c.sftpPassphrase = passphrase
	return nil
}

// ValidateBaseURL enforces TLS on an endpoint URL that carries credentials.
// Nextcloud and WebDAV both authenticate with HTTP basic auth, the Nextcloud
// Login Flow returns a freshly minted app password over the same connection,
// and the custom destination substitutes its resolved secret into the request
// headers, so plain http means a cleartext credential on the wire. https is
// always accepted; http is accepted only when the host cannot leave the local
// network (loopback, or an RFC1918 / link-local / unique-local literal), or
// when the user has explicitly set upload.allow_insecure_http. field names the
// setting in the error so the message is actionable. The URL is never
// rewritten - an unusable value is reported, not silently "fixed".
func ValidateBaseURL(field, raw string, allowInsecure bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("config: %s is not a valid URL: %w", field, err)
	}
	if u.Host == "" {
		return fmt.Errorf("config: %s must be a full URL, e.g. https://cloud.example.com", field)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if allowInsecure || isLocalHostname(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("config: %s uses plain http://, which sends the password in cleartext; use https:// (or set upload.allow_insecure_http: true if this server has no TLS and is only reachable on a trusted network)", field)
	default:
		return fmt.Errorf("config: %s must start with https://", field)
	}
}

// isLocalHostname reports whether host is unambiguously local without doing
// any name resolution: literal loopback/private/link-local addresses, or the
// reserved localhost names. Anything that would need a DNS lookup to classify
// is treated as remote.
func isLocalHostname(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func (c *Config) validate() error {
	switch c.Theme {
	case "", "light", "dark", "system":
	default:
		return fmt.Errorf("config: theme must be one of light, dark, system, or empty (system)")
	}
	if c.Upload.ShareExpireDays < 0 {
		return fmt.Errorf("config: upload.share_expire_days must be >= 0")
	}
	if !validUploadDestinations[c.Upload.Destination] {
		return fmt.Errorf("config: upload.destination must be one of nextcloud, s3, sftp, webdav, custom")
	}
	if !c.UploadEnabled() {
		// Local-only mode: every destination section is entirely optional.
		return nil
	}
	// Fail-closed, but scoped to the active destination only: an invalid
	// section for a destination that is not selected must not block save/load.
	switch c.Upload.Destination {
	case "nextcloud":
		if c.Nextcloud.BaseURL == "" {
			return fmt.Errorf("config: nextcloud.base_url is required (or set upload.enabled: false for local-only use)")
		}
		if err := ValidateBaseURL("nextcloud.base_url", c.Nextcloud.BaseURL, c.Upload.AllowInsecureHTTP); err != nil {
			return err
		}
		if c.Nextcloud.Username == "" {
			return fmt.Errorf("config: nextcloud.username is required")
		}
		if c.Nextcloud.DavUser == "" {
			return fmt.Errorf("config: nextcloud.dav_user could not be derived; set it explicitly")
		}
	case "s3":
		if c.S3.Endpoint == "" {
			return fmt.Errorf("config: s3.endpoint is required")
		}
		// s3.endpoint is documented as scheme-less host[:port] (TLS on), but an
		// http:// prefix silently disables TLS in the uploader. SigV4 keeps the
		// secret key off the wire, yet it still exposes the access key ID and
		// full request/response bytes - so hold http to the same local-or-opt-in
		// rule as the base URLs.
		if strings.HasPrefix(c.S3.Endpoint, "http://") {
			u, err := url.Parse(c.S3.Endpoint)
			if err != nil || u.Hostname() == "" {
				return fmt.Errorf("config: s3.endpoint is not a valid URL: %v", err)
			}
			if !c.Upload.AllowInsecureHTTP && !isLocalHostname(u.Hostname()) {
				return fmt.Errorf("config: s3.endpoint uses plain http://, which disables TLS for uploads; use https:// or a bare host[:port] (or set upload.allow_insecure_http: true if this server is only reachable on a trusted network)")
			}
		}
		if c.S3.Bucket == "" {
			return fmt.Errorf("config: s3.bucket is required")
		}
		if c.S3.AccessKey == "" {
			return fmt.Errorf("config: s3.access_key is required")
		}
		if c.s3SecretKey == "" {
			return fmt.Errorf("config: set exactly one of s3.secret_key_file or s3.secret_key_env")
		}
	case "sftp":
		if c.SFTP.Host == "" {
			return fmt.Errorf("config: sftp.host is required")
		}
		if c.SFTP.User == "" {
			return fmt.Errorf("config: sftp.user is required")
		}
		if c.sftpPassword == "" && c.sftpPrivateKeyPEM == "" {
			return fmt.Errorf("config: sftp requires a password (password_file/password_env) or a private_key_file")
		}
	case "webdav":
		if c.WebDAV.BaseURL == "" {
			return fmt.Errorf("config: webdav.base_url is required")
		}
		if err := ValidateBaseURL("webdav.base_url", c.WebDAV.BaseURL, c.Upload.AllowInsecureHTTP); err != nil {
			return err
		}
	case "custom":
		if c.Custom.URL == "" {
			return fmt.Errorf("config: custom.url is required")
		}
		// The resolved secret is substituted into Headers/ExtraFields, so the
		// endpoint carries a credential the same way the basic-auth
		// destinations do. {name}/{mime} placeholders parse fine.
		if err := ValidateBaseURL("custom.url", c.Custom.URL, c.Upload.AllowInsecureHTTP); err != nil {
			return err
		}
	}
	return nil
}
