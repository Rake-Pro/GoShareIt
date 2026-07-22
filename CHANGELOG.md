# Changelog

All notable changes to GoShareIt are documented here. Every code or feature
change lands with an entry under Unreleased; releases move that section under
their version. Planned work lives in [BACKLOG.md](BACKLOG.md).

## [Unreleased]

### Added
- Hotkey parser: punctuation keys on both OSes (` - = [ ] ; ' , . / \ and
  Space, plus word aliases like `backtick`/`minus`), US layout.

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
