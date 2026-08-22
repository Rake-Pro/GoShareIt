# Changelog

All notable changes to GoShareIt are documented here. Every code or feature
change lands with an entry under Unreleased; releases move that section under
their version. Planned work lives in [BACKLOG.md](BACKLOG.md).

## [Unreleased]

### Fixed
- Only one GoShareIt host can run per user session. Launching a second copy
  (Start Menu shortcut, startup entry plus manual launch, `open -n`) used to
  yield duplicate tray icons and double hotkey registrations; it now exits
  quietly with a log line. Windows uses a session-local named mutex, macOS
  and Linux an `flock` on `<app root>/goshareit.lock`; both are released by
  the OS on crash. The lock waits up to 15 s so settings-save and updater
  relaunches (new process starts before the old one exits) keep working.

### Added
- Windows release binaries and the installer are Authenticode-signed via
  SignPath Foundation when `SIGNPATH_API_TOKEN` is configured (two signing
  requests per release, manual approval in SignPath, signatures verified in
  CI). Fixes Smart App Control's "Part of this app has been blocked" and the
  SmartScreen first-run warning once the project is approved. See
  docs/RELEASE.md.

### Added
- Settings UI: a "Record" button next to every hotkey field. Click it and
  press a key combination; the chord is written in the host's token format
  (Ctrl/Cmd/Win/Alt/Shift + key, F-keys, PrintScreen, punctuation). Esc
  cancels, Backspace clears the field. Manual typing still works, so
  comma-separated alternatives can be added by hand.
- Windows: `hotkeys.disable_snipping_printscreen` (settings UI, Hotkeys pane:
  "Free the PrintScreen key"). Windows 11 ships with "Use the Print screen key
  to open screen capture" on, so Snipping Tool owns PrintScreen and any
  GoShareIt chord built on it silently fails to register. When enabled the app
  flips that per-user setting off at startup (HKCU, no elevation); Windows may
  only honor it after the next sign-in. Off by default.

### Changed
- Windows: interactive region capture now uses GoShareIt's own overlay
  (`goshareit-editor --region`) and grabs the selected rectangle directly,
  instead of launching the Windows snip UI (`ms-screenclip:`) and polling the
  clipboard. The snip UI is now only a fallback when the overlay cannot run,
  so a disabled or missing Snipping Tool no longer turns region capture into a
  silent 60 s timeout. The overlay also makes "last region" replay work on
  Windows.
- Windows: the hotkey log line for an unregistrable PrintScreen chord now
  points at the new setting.

### Removed
- The updater's optional GitHub token support (`update.token_file`, settings
  UI "GitHub token" field). It existed only for the fine-grained read-only PAT
  needed while the repo was private; now that the repo is public, the updater
  calls the GitHub API anonymously and the option is dead weight. Existing
  configs with `update.token_file` set are unaffected - the field is simply
  ignored - and any `github-token.secret` file can be deleted; fresh installs
  no longer scaffold one.

### Security
- Server base URLs now require TLS. `nextcloud.base_url` and `webdav.base_url`
  are validated as real URLs and must be `https://`: both destinations
  authenticate with HTTP basic auth, and the Nextcloud Login Flow returns a
  freshly minted app password over the same connection, so plain `http://` put
  the credential on the wire in cleartext. `http://` is still accepted for
  hosts that cannot leave the local network (loopback, `localhost`, and
  RFC1918 / link-local / unique-local literals), and the new
  `upload.allow_insecure_http` opt-in (settings UI: "Allow insecure http://
  server URL") re-enables it everywhere for a TLS-less server on a trusted
  network. URLs are never silently rewritten, and an already-saved `http://`
  config does not brick the app - the loader reports the error and the host
  opens the settings UI, same as any other invalid config. (#2)
- The upload history log (`~/.goshareit/history.jsonl`) is created 0600 instead
  of 0644, in a 0700 directory, and an existing 0644 log is tightened on open.
  Entries hold public share URLs, which are capability links. (#3)
- `custom.url` is held to the same rule. The custom destination substitutes the
  resolved `{secret}` into the request headers, so its endpoint carries a
  credential exactly like the basic-auth destinations do; it previously
  accepted any URL. The secret is only ever substituted into headers and
  multipart form fields, never into the URL, so it cannot end up in a query
  string. `{name}`/`{mime}` placeholders in the URL are unaffected.

### Fixed
- Settings: the host restarts to apply changes only when Save was actually
  used. The settings helper now signals a completed save via its exit code
  (`settings.ExitSaved`) instead of the host inferring one from the config
  file's mtime, which also fired on writes the settings window never made
  (e.g. the tray upload toggle) and on a Save with no edits - both looked
  like "closing settings restarts the app". Closing the window without
  saving now always discards, and a save that leaves the config
  byte-identical skips the restart too.

### Added
- Settings UI: a Discard Changes button next to Save that closes the window
  without saving, as an explicit alternative to just closing it.

### Changed
- Settings UI: field hints ("blank = derived from username", "always opens
  the editor", ...) moved from beside each input to underneath it, so every
  input in a section is the same full width instead of being squeezed by
  however long its instruction text is. The comma-separated-alternatives
  instruction on hotkeys is now a single note under the Hotkeys heading
  rather than a hint crammed next to the first field.

## [0.0.8] - 2026-07-30

First release signed with a real Developer ID Application certificate and
notarized by Apple (notarytool: Accepted, ticket stapled). Note: there is no
v0.0.7 - two failed release attempts left dangling `v0.0.7` tags, both
deleted; numbering jumps from 0.0.6 to 0.0.8.

### Changed
- CI: dependabot bumped `actions/upload-artifact` v4 -> v7 and
  `actions/download-artifact` v4 -> v8 in `release.yml`.
- CI: a temporary sign-debug workflow was added to diagnose the Developer ID
  keychain failure below, then removed the same day once the fix landed.

### Fixed
- Release signing with a real Developer ID cert: the CI build keychain now
  imports Apple's Developer ID CA intermediates (G1+G2) before signing. A
  Keychain-exported p12 carries only the leaf + key, so the identity imported
  but was never valid on the bare runner keychain and codesign failed with
  "no identity found" (the previous self-signed identity was its own anchor
  and never exposed this).
- Release signing now signs by the identity's derived SHA-1 hash instead of
  the `DEVELOPER_ID_APP` name string: `codesign -s <name>` matches against
  the exact certificate CN, so formatting drift in the secret failed with
  "no identity found" even though the identity was present and valid (hit
  twice on the first Developer ID release attempts). The build keychain only
  ever holds the one imported identity, so its SHA-1 is derived via
  `find-identity` and used to sign instead.

## [0.0.6] - 2026-07-30

### Added
- New `notify.Confirmer` seam (`internal/core/notify`) for blocking native
  yes/no dialogs, alongside `Notifier`: darwin via `osascript display dialog`
  (120s auto-cancel, Esc/-128 treated as "no"), windows via a PowerShell WPF
  `MessageBox` (fixed Yes/No buttons - the interface's custom labels are
  advisory on this platform), wired through `core.Providers`/`core.App`
  exactly like `Notifier` (optional; nil-tolerant on linux/dev, where it is
  now backed by `fake.Confirmer`). Manually clicking "Check for Updates" and
  finding an update now pops a native "Update Now?" dialog and installs
  immediately on yes, instead of retitling the tray item and telling the user
  to click the menu again; background periodic checks are unchanged (still
  quiet: notification + tray retitle only, never a popup).
- CI: Trivy filesystem CVE scan of the module tree (org security-scanning
  standard adapted for a no-container repo): HIGH+CRITICAL reported for
  visibility, fixable CRITICALs block the aggregate `build` gate.
- Four new `Uploader` implementations in `internal/core/upload`: `S3`
  (S3-compatible buckets - AWS S3, B2, R2, MinIO - via `minio-go/v7`, public
  URL template or presigned GET), `SFTP` (via `pkg/sftp` + `x/crypto/ssh`,
  key or password auth, optional host key fingerprint pinning), `WebDAV`
  (plain PUT with basic auth, no OCS share step), and `Custom` (generic HTTP
  uploader: multipart or raw body, JSON dot-path or regex
  response parsing), plus `CustomPresets()` with starter configs for imgur,
  catbox, and 0x0.st. New deps: `github.com/minio/minio-go/v7`,
  `github.com/pkg/sftp` (both added via `go get`, not `go mod tidy`);
  `golang.org/x/crypto` moves from an indirect to a direct dependency
  (`crypto/ssh`).
- Selectable upload destination: new `upload.destination` config setting
  (`nextcloud | s3 | sftp | webdav | custom`, default `nextcloud`) plus
  matching top-level `s3:`, `sftp:`, `webdav:`, and `custom:` yaml sections
  (see `config.example.yaml`). Secrets follow the existing Nextcloud
  `password_file`/`password_env` pattern - `s3.secret_key_file`/`_env`,
  `sftp.password_file`/`_env` (or `sftp.private_key_file`, which takes
  precedence, plus an optional `sftp.passphrase_file`/`_env` for an encrypted
  key), `webdav.password_file`/`_env`, `custom.secret_file`/`_env` (the
  resolved value substitutes a literal `{secret}` placeholder in `custom`
  header and extra-field values). Validation is fail-closed but scoped to the
  active destination only, and only when `upload.enabled`; an `sftp` config
  with no `host_key_fingerprint` still loads but logs a startup warning
  (host key left unverified). `cmd/goshareit` gained `buildUploader(cfg)`,
  switching on the destination to construct the right `Uploader`; `main.go`
  now fatals through it instead of hardcoding Nextcloud. The settings UI
  (`goshareit-settings`) consolidates Nextcloud/S3/SFTP/WebDAV/Custom into a
  single Upload section: a Destination select drives one inset destination
  panel holding a sub-panel per destination (all rendered, only the selected
  one visible - hidden via the select's change handler rather than greyed),
  plus a Custom-panel preset picker (imgur/catbox/0x0) backed by a new
  `Service.Presets()` method; headers and extra fields are edited as one
  `key=value` per line. Upload-enabled-off still greys the Destination
  select, the whole destination panel, and the filename template
  (local-only mode). `SaveRequest` gained
  one write-only secret field per new destination secret
  (`NewS3SecretKey`, `NewSFTPPassword`, `NewSFTPPassphrase`,
  `NewWebDAVPassword`, `NewCustomSecret`), matching the existing
  `NewPassword`/`NewToken` pattern.
- `theme` config setting (`light | dark | system`, default system): applies
  to the annotation editor and the settings window. The editor helper
  resolves "system" via native OS detection (macOS: `AppleInterfaceStyle`
  global default; Windows: `AppsUseLightTheme` registry value), falling back
  to dark on detection error.
- Editor confirm button label now reflects the post-capture pipeline (e.g.
  "Copy & Upload", "Save & Upload", "Copy, Save & Upload", "Done") instead of
  a generic "Confirm", composed host-side from `after_capture`/`upload.enabled`.
- macOS bundle: `AppIcon.icns` is now generated (from
  `build/icons/goshareit_icon.png` via `sips`/`iconutil`) and shipped in
  `Info.plist` as `CFBundleIconFile` - the icon step in `bundle.sh` was
  previously a commented-out placeholder, so every prior build (including
  updates) fell back to the generic Finder icon.

### Changed
- Default annotation stroke width raised from 3px to 6px (`editor.stroke_width`
  default, starter config, and editor fallback): 3px was near-invisible on
  retina-resolution captures. The in-editor stroke control is relabeled from
  "w3" to "6 px" and its +/- range clamped to 1-32 (was 1-64).
- Editor toolbar restyled to actually use the theme: toolbar and canvas get
  explicit themed background fills (previously the toolbar showed through to
  the stock white Gio surface above a hardcoded dark canvas), the selected
  tool is an accent-filled button instead of "[bracket]" text, unselected
  tools/undo/redo/Cancel get a subtle themed background, Confirm is
  accent-filled, buttons share a consistent corner radius/text size/spacing,
  the color-swatch selection ring uses the theme fg color instead of a fixed
  black (invisible on dark swatches), the white swatch gets a hairline
  border in light mode, and the text field gets a themed background pill.

### Fixed
- Settings Save now applies without further user action: on success the
  settings window closes itself (new shell-injected `Service.CloseWindow`),
  which is what lets the blocked host process apply the config and restart
  immediately - previously the user had to close the window by hand before
  anything took effect (found on-device on macOS). On save failure the window
  stays open with the error.

## [0.0.5] - 2026-07-25

### Changed
- The tray Stop Recording item shows a record marker while a recording is
  active.

### Fixed
- Default record hotkey moved off `{mod}+Shift+R` to `{mod}+Shift+2`: browsers
  use Cmd/Ctrl+Shift+R for hard reload, and because the global-hotkey event tap
  consumes matched chords, every hard-reload attempt silently toggled a
  full-screen recording instead (found on-device on macOS: constant recording
  sessions and large mp4s in the save dir). Existing configs are not migrated -
  edit `hotkeys.record` manually.
- Default quit hotkey removed (was `{mod}+Shift+Q`, the macOS logout chord -
  same swallowed-shortcut class as the record collision above). Quit remains
  in the tray menu; existing configs are not migrated.
- Recording start is no longer silent: an info log with the trigger source
  (hotkey vs tray) plus a desktop notification saying how to stop; stop logs
  its trigger too. Previously the only evidence of a session was the
  upload/complete log line at stop, making runaway recordings undiagnosable.

## [0.0.4] - 2026-07-22

### Fixed
- Blur is now redaction-grade: the stroke setting acts as a strength
  multiplier and the kernel gets a region-scaled minimum radius, so
  screenshot-scale text inside a blurred region is destroyed rather than
  softened (found on-device: white-on-black text stayed readable). Guarded
  by a worst-case fine-detail test, plus an env-gated visual harness
  (TestVisualSample) for eyeballing strength against real captures.

## [0.0.3] - 2026-07-22

### Added
- Hotkey parser: punctuation keys on both OSes (` - = [ ] ; ' , . / \ and
  Space, plus word aliases like `backtick`/`minus`), US layout.

### Changed
- Windows tray icon replaced with the product logo (shutter aperture + G
  mark, full-color multi-size ICO). The macOS menu bar keeps the monochrome
  capture-corners template glyph, which suits the black-and-white menu bar
  better. Master logo at build/icons/goshareit_icon.png (also the future
  app-icon source); regenerate via scripts/gen-tray-icon.py.

### Fixed
- Editor toolbar no longer pushes Confirm/Cancel out of view at narrow
  window widths: split into a scrollable tool/swatch row and a fixed action
  row, plus a minimum window size (found on-device at MacBook default size).

## [0.0.2] - 2026-07-22

### Added
- `upload.enabled` toggle (settings: "Upload captures"): off = local-only
  mode. Nothing leaves the machine, the Nextcloud section and credentials
  become fully optional (empty password file no longer blocks saving), the
  pipeline skips upload/share-link steps while local save, clipboard image
  copy, history, and notifications keep working, and the settings UI greys
  out the server/after-upload sections while disabled. Credentials on disk
  are kept, so uploads can be re-enabled at any time.
- Upload toggle hotkey (`hotkeys.upload_toggle`) and a tray "Uploads: On/Off"
  item: flip local-only mode instantly. Enabling is refused (with a
  notification) when no server is configured; the state persists to the
  config file and takes effect immediately for the next capture.
- Release builds now ad-hoc sign the whole macOS bundle when Developer ID
  secrets are absent, so TCC permission grants (Screen Recording,
  Accessibility) persist within an install instead of re-prompting on every
  capture. Grants still need one re-approval after each update until real
  code signing lands.

## [0.0.1] - 2026-07-22

Project baseline (history squashed; versioning restarted on the road to a
1.0.0 public release).

### Added
- Screenshot capture: region, full screen, and window modes with global
  hotkeys and a menu-bar/tray menu (macOS and Windows from one Go codebase).
- Screen recording: mp4 (native AVFoundation on macOS, ffmpeg on Windows),
  GIF (pure Go), full-screen and region modes with an interactive region
  selector overlay.
- Annotation editor (separate `goshareit-editor` binary): crop, arrow, rect,
  ellipse, line, freehand, text, blur, pixelate, highlight, step numbers.
- Nextcloud upload with automatic public share links (preview links for
  images) copied to the clipboard, plus optional local copies.
- Settings UI (separate `goshareit-settings` binary, Wails): every option
  editable, native folder picker, browser sign-in via Nextcloud Login Flow v2
  (OIDC/SSO-compatible), reset-to-defaults with confirmation, fail-closed
  validated saves, automatic host restart on save.
- Hotkeys: comma-separated alternative chords per action, PrintScreen support
  on both OSes (PC keyboards deliver it as F13 on macOS), and `*_edit` hotkey
  variants that always open the annotation editor.
- First-run onboarding: an unconfigured install opens the settings UI instead
  of exiting; all logs mirror to `<app root>/goshareit.log`.
- Self-updater against GitHub Releases (checksum-verified, swaps the .app
  bundle or sibling exes, relaunches) with a tray "Check for Updates" item.
- Release pipeline: promotion-PR flow minting semver tags and building
  macOS universal dmg/zip, Windows installer + zip, and checksums per release.
