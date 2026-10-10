// Package settings is the backend of the goshareit-settings UI: it loads the
// raw config for editing and saves it back with validation, handling the
// secret files (Nextcloud app password, destination secrets) that never live
// inline in the YAML. Pure Go and GUI-free so it is testable on linux.
package settings

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/engines"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

// ExitSaved is the settings helper's process exit code when the user saved at
// least once. The host restarts to apply the new config only on this code; a
// plain window close (or Discard changes) exits 0 and the host does nothing.
const ExitSaved = 42

// Service is bound into the Wails frontend. All methods are invoked from JS.
// PickDir and OpenURL are injected by the GUI shell (native dialogs/browser);
// they stay nil in tests and get portable fallbacks where possible.
type Service struct {
	ConfigPath string
	Version    string // app version shown in the UI footer
	// Packaged marks a Microsoft Store (MSIX) install. Settings the package
	// manifest owns - start at login - are shown greyed out there instead of
	// accepting an edit the host would then ignore.
	Packaged bool

	PickDir func() (string, error) // native directory picker
	OpenURL func(url string) error // native browser open; nil -> osOpenURL
	Close   func()                 // close the settings window; nil in tests
	// CheckHotkey validates one chord the way the host will bind it on this
	// OS and returns its canonical form (aliases such as Cmd/Ctrl or
	// Option/Alt resolved per OS), which the duplicate check compares. nil
	// skips validation; duplicates are then compared on the spelled chord.
	CheckHotkey func(chord string) (string, error)
	// OCREngine builds the text-recognition engine whose probe fills the
	// "Text recognition" status line; nil = engines.New, the same factory
	// the host and the editor use.
	OCREngine func(engines.Config) ocr.Engine

	// ocrEng is reused across Loads while its config is unchanged: on
	// Windows every engine owns a worker OS thread for the process lifetime.
	ocrMu  sync.Mutex
	ocrEng ocr.Engine
	ocrCfg engines.Config

	saved atomic.Bool // set once Save succeeds; drives the ExitSaved exit code

	// dirty mirrors the page's unsaved-edits state (SetDirty); closeWarned
	// records that one close was already held back for it.
	dirty       atomic.Bool
	closeWarned atomic.Bool

	loginMu     sync.Mutex
	loginCancel context.CancelFunc // set while BrowserLogin runs

	// loadedUpload is upload.enabled as the page loaded it (Load), so Save
	// can tell "the user flipped Upload captures" from "the form still holds
	// the old value while the tray toggle changed the file".
	uploadMu        sync.Mutex
	loadedUpload    bool
	loadedUploadSet bool
}

// SetDirty is called by the page whenever its unsaved-edits state changes.
func (s *Service) SetDirty(dirty bool) {
	s.dirty.Store(dirty)
	if !dirty {
		s.closeWarned.Store(false)
	}
}

// HoldClose reports whether a window close should be held back so the page
// can warn about unsaved edits. It holds only the first close per dirty
// state, so closing again goes through.
func (s *Service) HoldClose() bool {
	return s.dirty.Load() && s.closeWarned.CompareAndSwap(false, true)
}

// DidSave reports whether Save succeeded at least once in this session.
func (s *Service) DidSave() bool { return s.saved.Load() }

// LoadResult is what the frontend edits. Secrets are never returned - only
// whether they are set.
type LoadResult struct {
	Config      *config.Config `json:"config"`
	ConfigPath  string         `json:"configPath"`
	HasPassword bool           `json:"hasPassword"`
	// Destination secrets, one flag per non-Nextcloud secret below.
	HasS3SecretKey    bool `json:"hasS3SecretKey"`
	HasSFTPPassword   bool `json:"hasSFTPPassword"`
	HasSFTPPassphrase bool `json:"hasSFTPPassphrase"`
	HasWebDAVPassword bool `json:"hasWebDAVPassword"`
	HasCustomSecret   bool `json:"hasCustomSecret"`
	// Public host presets: one key per host, so the flag is scoped to the
	// destination it was computed for (HostSecretFor). HostSecrets lists
	// every host that already has a key on disk / in env.
	HasHostSecret bool            `json:"hasHostSecret"`
	HostSecretFor string          `json:"hostSecretFor"`
	HostSecrets   map[string]bool `json:"hostSecrets"`
	Version       string          `json:"version"`
	OS            string          `json:"os"`
	Packaged      bool            `json:"packaged"`
	// What this build/session can actually do, so the page greys out
	// settings the host would ignore: the updater is off on Store builds,
	// the what's-new window is not offered on Linux, and recording is not
	// available in Wayland sessions.
	UpdaterAvailable   bool `json:"updaterAvailable"`
	ChangelogSupported bool `json:"changelogSupported"`
	RecordingSupported bool `json:"recordingSupported"`
	// Text recognition probe for the status line; OCRReason (reason and
	// hint) is also the live gate text on the OCR fields when unavailable.
	OCRAvailable bool     `json:"ocrAvailable"`
	OCREngine    string   `json:"ocrEngine"`
	OCRVersion   string   `json:"ocrVersion"`
	OCRLangs     []string `json:"ocrLangs"`
	OCRReason    string   `json:"ocrReason"`
}

// SaveRequest carries the edited config plus optional new secret values
// (write-only: empty means "leave the current secret untouched").
type SaveRequest struct {
	Config      *config.Config `json:"config"`
	NewPassword string         `json:"newPassword"`
	// Destination secrets, one write-only field per non-Nextcloud secret.
	NewS3SecretKey    string `json:"newS3SecretKey"`
	NewSFTPPassword   string `json:"newSFTPPassword"`
	NewSFTPPassphrase string `json:"newSFTPPassphrase"`
	NewWebDAVPassword string `json:"newWebDAVPassword"`
	NewCustomSecret   string `json:"newCustomSecret"`
	// NewHostSecret is written for Config.Upload.Destination when that is a
	// public host preset id.
	NewHostSecret string `json:"newHostSecret"`
}

// Load reads the config for editing. A missing file yields the starter
// defaults so first-run users edit a sensible template.
func (s *Service) Load() (*LoadResult, error) {
	res, err := s.load()
	if err == nil {
		s.uploadMu.Lock()
		s.loadedUpload, s.loadedUploadSet = res.Config.UploadEnabled(), true
		s.uploadMu.Unlock()
	}
	return res, err
}

// load is Load without recording the loaded upload switch (BrowserLogin
// reads the file again mid-session).
func (s *Service) load() (*LoadResult, error) {
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
	hostSecrets := map[string]bool{}
	for id := range upload.CustomPresets() {
		hostSecrets[id] = secretPresent(cfg.HostSecretSource(id))
	}
	st := s.probeOCR(cfg)
	return &LoadResult{
		Config:            cfg,
		ConfigPath:        s.ConfigPath,
		HasPassword:       secretPresent(cfg.Nextcloud.PasswordFile, cfg.Nextcloud.PasswordEnv),
		HasS3SecretKey:    secretPresent(cfg.S3.SecretKeyFile, cfg.S3.SecretKeyEnv),
		HasSFTPPassword:   secretPresent(cfg.SFTP.PasswordFile, cfg.SFTP.PasswordEnv),
		HasSFTPPassphrase: secretPresent(cfg.SFTP.PassphraseFile, cfg.SFTP.PassphraseEnv),
		HasWebDAVPassword: secretPresent(cfg.WebDAV.PasswordFile, cfg.WebDAV.PasswordEnv),
		HasCustomSecret:   secretPresent(cfg.Custom.SecretFile, cfg.Custom.SecretEnv),
		HasHostSecret:     hostSecrets[cfg.Upload.Destination],
		HostSecretFor:     cfg.Upload.Destination,
		HostSecrets:       hostSecrets,
		Version:           s.Version,
		OS:                runtime.GOOS,
		Packaged:          s.Packaged,

		UpdaterAvailable:   !s.Packaged,
		ChangelogSupported: runtime.GOOS != "linux",
		RecordingSupported: runtime.GOOS != "linux" || !waylandSession(),

		OCRAvailable: st.Available,
		OCREngine:    st.Engine,
		OCRVersion:   st.Version,
		OCRLangs:     st.Langs,
		OCRReason:    st.Explain(),
	}
}

// ocrEngine returns the cached engine for c, building a new one only when
// the engine config changed.
func (s *Service) ocrEngine(c engines.Config) ocr.Engine {
	s.ocrMu.Lock()
	defer s.ocrMu.Unlock()
	if s.ocrEng != nil && slices.Equal(s.ocrCfg.Langs, c.Langs) && s.ocrCfg.TesseractPath == c.TesseractPath {
		return s.ocrEng
	}
	newEngine := s.OCREngine
	if newEngine == nil {
		newEngine = engines.New
	}
	s.ocrEng, s.ocrCfg = newEngine(c), c
	return s.ocrEng
}

// probeOCR asks the configured engine whether text recognition works here,
// bounded to 3 s like the host's probe.
func (s *Service) probeOCR(cfg *config.Config) ocr.Status {
	eng := s.ocrEngine(engines.Config{Langs: cfg.OCR.Languages, TesseractPath: config.ExpandHome(cfg.OCR.TesseractPath)})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan ocr.Status, 1)
	go func() { done <- eng.Probe(ctx) }()
	select {
	case st := <-done:
		return st
	case <-ctx.Done():
		return ocr.Status{Reason: "Text recognition did not answer in time."}
	}
}

// waylandSession mirrors platform/linux.IsWayland (the host wires no
// recorder there); that package only builds on Linux, so the rule is
// repeated here: XDG_SESSION_TYPE decides when set, else WAYLAND_DISPLAY.
func waylandSession() bool {
	switch strings.ToLower(os.Getenv("XDG_SESSION_TYPE")) {
	case "wayland":
		return true
	case "x11":
		return false
	}
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

// SaveError is how Save refuses a save. Field names the settings-form field
// at fault in the frontend schema's terms ("Nextcloud.BaseURL", "@password";
// "" when no single field is), Problem is a plain phrase that reads after
// that field's label ("is required"), and Message is a full plain sentence
// for when there is no field. Error() keeps the technical detail for logs.
// Wails hands the JSON form to the page as the rejected call's cause.
type SaveError struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
	Message string `json:"message"`
	detail  string
}

func (e *SaveError) Error() string { return e.detail }

// uiFields maps config.FieldError keys to the frontend schema paths.
var uiFields = map[string]string{
	"theme":                    "Theme",
	"upload.destination":       "Upload.Destination",
	"upload.share_expire_days": "Upload.ShareExpireDays",
	"nextcloud.base_url":       "Nextcloud.BaseURL",
	"nextcloud.username":       "Nextcloud.Username",
	"nextcloud.dav_user":       "Nextcloud.DavUser",
	"nextcloud.password":       "@password",
	"s3.endpoint":              "S3.Endpoint",
	"s3.bucket":                "S3.Bucket",
	"s3.access_key":            "S3.AccessKey",
	"s3.secret_key":            "@s3_secret_key",
	"sftp.host":                "SFTP.Host",
	"sftp.user":                "SFTP.User",
	"sftp.password":            "@sftp_password",
	"sftp.passphrase":          "@sftp_passphrase",
	"sftp.private_key_file":    "SFTP.PrivateKeyFile",
	"webdav.base_url":          "WebDAV.BaseURL",
	"webdav.password":          "@webdav_password",
	"custom.url":               "Custom.URL",
	"custom.secret":            "@custom_secret",
}

// saveErrorFrom converts a loader error into a SaveError, keeping the field
// and plain problem when the loader attached them.
func saveErrorFrom(err error) *SaveError {
	detail := "not saved - the config does not validate: " + err.Error()
	var fe *config.FieldError
	if errors.As(err, &fe) {
		field := uiFields[fe.Field]
		if strings.HasPrefix(fe.Field, "hosts.") {
			field = "@host_secret"
		}
		return &SaveError{Field: field, Problem: fe.Problem, Message: "A setting " + fe.Problem + ".", detail: detail}
	}
	return &SaveError{Message: "The settings could not be checked: " + err.Error(), detail: detail}
}

// keepUploadToggle: when the form's "Upload captures" still has the value the
// page loaded, the user did not touch it, so the file's current value wins.
// That keeps a tray or hotkey upload toggle made while the window was open
// from being undone by Save.
func (s *Service) keepUploadToggle(cfg *config.Config) {
	s.uploadMu.Lock()
	loaded, ok := s.loadedUpload, s.loadedUploadSet
	s.uploadMu.Unlock()
	if !ok || cfg.UploadEnabled() != loaded {
		return
	}
	cur, err := config.LoadRaw(s.ConfigPath)
	if err != nil || cur.Upload.Enabled == nil {
		return
	}
	v := *cur.Upload.Enabled
	cfg.Upload.Enabled = &v
}

// checkHotkeys refuses a chord the host could not bind and a chord used by two
// hotkeys, naming the field so the page can point at it. The Capture Text
// chord counts only while text recognition is on (textOn), the only time the
// host binds it; the page greys that field otherwise.
func (s *Service) checkHotkeys(h *config.HotkeysConfig, textOn bool) error {
	text := h.Text
	if !textOn {
		text = ""
	}
	fields := []struct{ field, value string }{
		{"Hotkeys.Region", h.Region},
		{"Hotkeys.FullScreen", h.FullScreen},
		{"Hotkeys.Window", h.Window},
		{"Hotkeys.RegionEdit", h.RegionEdit},
		{"Hotkeys.FullScreenEdit", h.FullScreenEdit},
		{"Hotkeys.WindowEdit", h.WindowEdit},
		{"Hotkeys.UploadToggle", h.UploadToggle},
		{"Hotkeys.Record", h.Record},
		{"Hotkeys.Text", text},
		{"Hotkeys.Quit", h.Quit},
	}
	seen := map[string]bool{}
	for _, f := range fields {
		for _, chord := range strings.Split(f.value, ",") {
			chord = strings.TrimSpace(chord)
			if chord == "" {
				continue
			}
			canonical := chord
			if s.CheckHotkey != nil {
				c, err := s.CheckHotkey(chord)
				if err != nil {
					return &SaveError{
						Field:   f.field,
						Problem: "uses " + chord + ", which cannot be set up as a hotkey on this computer; pick another key combination",
						detail:  fmt.Sprintf("settings: hotkey %s %q: %v", f.field, chord, err),
					}
				}
				canonical = c
			}
			key := chordKey(canonical)
			if seen[key] {
				return &SaveError{
					Field:   f.field,
					Problem: "uses " + chord + ", which another hotkey already uses",
					detail:  fmt.Sprintf("settings: hotkey %s %q is a duplicate", f.field, chord),
				}
			}
			seen[key] = true
		}
	}
	return nil
}

// chordKey normalizes a chord for duplicate detection: case, token order and
// same-meaning spellings do not matter ("shift+control+1" == "Ctrl+Shift+1").
// Off macOS, Cmd is bound as Control (the accelerator spelling "cmd" means
// CmdOrCtrl there), so it folds to ctrl here as well: the per-OS checker keeps
// the two spellings apart and would otherwise let Cmd+X sit next to Ctrl+X.
func chordKey(chord string) string {
	alias := map[string]string{"command": "cmd", "control": "ctrl", "option": "alt", "opt": "alt"}
	if runtime.GOOS != "darwin" {
		alias["cmd"], alias["command"] = "ctrl", "ctrl"
	}
	var tokens []string
	for _, t := range strings.Split(chord, "+") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			if a, ok := alias[t]; ok {
				t = a
			}
			tokens = append(tokens, t)
		}
	}
	sort.Strings(tokens)
	return strings.Join(tokens, "+")
}

// stagedSecret is a new secret value written next to its target file but not
// yet renamed over it.
type stagedSecret struct {
	tmp, target string
}

// stageSecret writes value to a 0600 temp file in the target's directory, so
// the later rename is atomic and never crosses a filesystem.
func stageSecret(path, value string) (stagedSecret, error) {
	if path == "" {
		return stagedSecret{}, fmt.Errorf("settings: no secret file path configured")
	}
	full := config.ExpandHome(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return stagedSecret{}, fmt.Errorf("settings: create secret dir: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(full), "."+filepath.Base(full)+".pending-*")
	if err != nil {
		return stagedSecret{}, fmt.Errorf("settings: stage secret: %w", err)
	}
	_, werr := f.WriteString(strings.TrimSpace(value) + "\n")
	cerr := f.Close()
	if werr == nil {
		werr = os.Chmod(f.Name(), 0o600)
	}
	if werr != nil || cerr != nil {
		os.Remove(f.Name())
		return stagedSecret{}, fmt.Errorf("settings: write secret %s: write=%v close=%v", full, werr, cerr)
	}
	return stagedSecret{tmp: f.Name(), target: full}, nil
}

// writeTempConfig writes cfg as YAML to a temp file in dir and returns its
// path.
func writeTempConfig(dir string, cfg *config.Config) (string, error) {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("settings: marshal config: %w", err)
	}
	header := "# GoShareIt configuration. Managed by the settings UI; comments are not preserved.\n"
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return "", fmt.Errorf("settings: temp config: %w", err)
	}
	_, werr := tmp.Write(append([]byte(header), out...))
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("settings: write temp config: write=%v close=%v", werr, cerr)
	}
	return tmp.Name(), nil
}

// Save persists the edited config and any new secrets, validating before
// anything is written over the current files. New secrets are staged next
// to their targets and a candidate config that points at the staged copies
// runs through the full loader, so the user gets real validation errors
// immediately. Only when it passes are the secrets and then the config
// renamed into place; a rejected save leaves every stored secret and the
// config exactly as they were.
func (s *Service) Save(req *SaveRequest) error {
	if req == nil || req.Config == nil {
		return &SaveError{Message: "There was nothing to save.", detail: "settings: empty save request"}
	}
	cfg := req.Config
	applyEditingDefaults(cfg)
	s.keepUploadToggle(cfg)
	if err := s.checkHotkeys(&cfg.Hotkeys, cfg.OCREnabled()); err != nil {
		return err
	}

	// cand is what gets validated: cfg with each new secret's file field
	// pointed at its staged copy. Hosts is copied so repointing a host key
	// never leaks into the config that is installed.
	cand := *cfg
	cand.Hosts = make(map[string]config.HostConfig, len(cfg.Hosts)+1)
	for k, v := range cfg.Hosts {
		cand.Hosts[k] = v
	}
	type secretField struct {
		value, field, file, env string
		point                   func(staged string)
	}
	fields := []secretField{
		{req.NewPassword, "@password", cfg.Nextcloud.PasswordFile, cfg.Nextcloud.PasswordEnv, func(p string) { cand.Nextcloud.PasswordFile = p }},
		{req.NewS3SecretKey, "@s3_secret_key", cfg.S3.SecretKeyFile, cfg.S3.SecretKeyEnv, func(p string) { cand.S3.SecretKeyFile = p }},
		{req.NewSFTPPassword, "@sftp_password", cfg.SFTP.PasswordFile, cfg.SFTP.PasswordEnv, func(p string) { cand.SFTP.PasswordFile = p }},
		{req.NewSFTPPassphrase, "@sftp_passphrase", cfg.SFTP.PassphraseFile, cfg.SFTP.PassphraseEnv, func(p string) { cand.SFTP.PassphraseFile = p }},
		{req.NewWebDAVPassword, "@webdav_password", cfg.WebDAV.PasswordFile, cfg.WebDAV.PasswordEnv, func(p string) { cand.WebDAV.PasswordFile = p }},
		{req.NewCustomSecret, "@custom_secret", cfg.Custom.SecretFile, cfg.Custom.SecretEnv, func(p string) { cand.Custom.SecretFile = p }},
	}
	if dest := cfg.Upload.Destination; config.IsHostDestination(dest) {
		file, env := cfg.HostSecretSource(dest)
		fields = append(fields, secretField{req.NewHostSecret, "@host_secret", file, env, func(p string) {
			h := cand.Hosts[dest]
			h.SecretFile, h.SecretEnv = p, ""
			cand.Hosts[dest] = h
		}})
	}

	var staged []stagedSecret
	installed := false
	defer func() {
		if !installed {
			for _, st := range staged {
				os.Remove(st.tmp)
			}
		}
	}()
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		if f.env != "" {
			// Env-sourced: a written file would be silently ignored at load.
			return &SaveError{
				Field:   f.field,
				Problem: "comes from the environment variable " + f.env + ", so it cannot be changed here",
				detail:  fmt.Sprintf("settings: %s is sourced from env var %s; unset the corresponding _env setting to use a file", f.field, f.env),
			}
		}
		st, err := stageSecret(f.file, f.value)
		if err != nil {
			return &SaveError{Message: "Not saved: a password or key could not be written.", detail: err.Error()}
		}
		staged = append(staged, st)
		f.point(st.tmp)
	}

	dir := filepath.Dir(s.ConfigPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return &SaveError{Message: "Not saved: the settings folder could not be created.", detail: fmt.Sprintf("settings: create config dir: %v", err)}
	}

	// Validate BEFORE installing anything: an invalid config must never reach
	// the real path - the host would fail to load it on its next start.
	check, err := writeTempConfig(dir, &cand)
	if err != nil {
		return &SaveError{Message: "Not saved: the settings file could not be written.", detail: err.Error()}
	}
	_, lerr := config.LoadFile(check)
	os.Remove(check)
	if lerr != nil {
		return saveErrorFrom(lerr)
	}

	tmp, err := writeTempConfig(dir, cfg)
	if err != nil {
		return &SaveError{Message: "Not saved: the settings file could not be written.", detail: err.Error()}
	}
	// Install secrets, then the config. Each replaced secret's previous
	// content is kept in memory, so a failure part way through puts every
	// file back and "Not saved" stays true.
	type previous struct {
		target  string
		data    []byte
		existed bool
	}
	var replaced []previous
	rollback := func() {
		for i := len(replaced) - 1; i >= 0; i-- {
			p := replaced[i]
			if p.existed {
				_ = os.WriteFile(p.target, p.data, 0o600)
			} else {
				os.Remove(p.target)
			}
		}
	}
	for _, st := range staged {
		old, rerr := os.ReadFile(st.target)
		if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			rollback()
			os.Remove(tmp)
			return &SaveError{Message: "Not saved: a password or key could not be stored.", detail: fmt.Sprintf("settings: read current secret %s: %v", st.target, rerr)}
		}
		if err := os.Rename(st.tmp, st.target); err != nil {
			rollback()
			os.Remove(tmp)
			return &SaveError{Message: "Not saved: a password or key could not be stored.", detail: fmt.Sprintf("settings: install secret %s: %v", st.target, err)}
		}
		replaced = append(replaced, previous{target: st.target, data: old, existed: rerr == nil})
	}
	if err := os.Rename(tmp, s.ConfigPath); err != nil {
		rollback()
		os.Remove(tmp)
		return &SaveError{Message: "Not saved: the settings file could not be replaced.", detail: fmt.Sprintf("settings: install config: %v", err)}
	}
	installed = true
	s.saved.Store(true)
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
	s.dirty.Store(false) // an explicit Save or Discard: never hold this close
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

// Presets returns the built-in public-host presets (request template plus
// notes) for the settings UI's preset picker, so the data lives in one place
// instead of being duplicated in JS.
func (s *Service) Presets() map[string]upload.Preset {
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
// minutes, or until CancelLogin. allowInsecure is the form's current "Allow
// insecure http://" switch, so it applies before the user saves. Persisting
// is deferred to Save.
func (s *Service) BrowserLogin(baseURL string, allowInsecure bool) (*LoginResult, error) {
	baseURL = strings.TrimSpace(baseURL)
	cur, err := s.load()
	if err != nil {
		return nil, err
	}
	// The flow returns a freshly minted app password over this connection, so
	// it gets the same TLS requirement as the saved config.
	if err := config.ValidateBaseURL("nextcloud.base_url", baseURL, allowInsecure); err != nil {
		var fe *config.FieldError
		if errors.As(err, &fe) {
			return nil, &SaveError{Field: "Nextcloud.BaseURL", Problem: fe.Problem, detail: err.Error()}
		}
		return nil, err
	}
	if cur.Config.Nextcloud.PasswordEnv != "" {
		return nil, fmt.Errorf("password is sourced from env var %s; unset password_env to use browser sign-in", cur.Config.Nextcloud.PasswordEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	s.loginMu.Lock()
	s.loginCancel = cancel
	s.loginMu.Unlock()
	defer func() {
		s.loginMu.Lock()
		s.loginCancel = nil
		s.loginMu.Unlock()
	}()
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

// CancelLogin ends a running BrowserLogin; it then returns a "cancelled"
// error. A no-op when no sign-in is running.
func (s *Service) CancelLogin() {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.loginCancel != nil {
		s.loginCancel()
	}
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
	if cfg.Update.Enabled == nil {
		t := true
		cfg.Update.Enabled = &t
	}
	if cfg.Update.AutoInstall == nil {
		t := true
		cfg.Update.AutoInstall = &t
	}
	if cfg.Update.ShowChangelog == nil {
		t := true
		cfg.Update.ShowChangelog = &t
	}
	if cfg.Upload.Enabled == nil {
		t := true
		cfg.Upload.Enabled = &t
	}
	if cfg.OCR.Enabled == nil {
		t := true
		cfg.OCR.Enabled = &t
	}
	if cfg.OCR.AutoRun == nil {
		t := true
		cfg.OCR.AutoRun = &t
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
