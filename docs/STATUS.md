# GoShareIt - Project Status & Remaining Work

Snapshot date: 2026-07-30. Current release: **v0.0.8** (first release signed
with a real Developer ID Application certificate and notarized by Apple -
notarytool `Accepted`, ticket stapled; there is no v0.0.7, see CHANGELOG).
Branch: `main` (prod is protected; releases go out via the promotion-PR flow,
see docs/RELEASE.md). Repo history was squashed to a single baseline commit
("GoShareIt 0.0.1 baseline") on 2026-07-22 and versioning restarted from
0.0.1; `1.0.0` is reserved for the eventual public release.

P1-P4b + GIF + region selector/recording are all code-complete and build green
(linux core CGO-off + GOOS=windows cross-build + all unit tests). v0.0.6
shipped a feature batch on top of that: selectable upload destinations
(S3/SFTP/WebDAV/custom HTTP alongside Nextcloud), app-wide light/dark/system
theming for the editor and settings window, settings Save auto-applying
without a manual window close, a native Update Now/Later dialog on manual
update checks, and the macOS app icon (.icns). That batch is code-complete
and container-parse-checked (JS only) but **not yet click-through-validated
on real hardware** - see "On-device validation still owed" below. On-device
validation of the earlier core flows (install, first-run, tray, screenshot
capture, upload, settings UI, local-only mode) remains verified on real
Windows and macOS hardware as before. The only unbuilt roadmap item is P4c
(in-editor copy/save/upload buttons) plus the small LastRegion-overlay wiring.

GoShareIt is a screenshot/recording + Nextcloud-upload tool. Goal: full-featured screenshot
capture (region, full screen, window) on **macOS and Windows from one Go codebase**. Architecture, rationale,
and the cross-platform feasibility analysis are in the design docs (`docs/DESIGN-recording.md`,
`docs/DESIGN-editor.md`).

## Architecture (one-paragraph map)

Pure-Go **core** under `internal/core/` (CGO-free, linux-testable) depends only on interface seams:
`capture.Capturer`, `capture.Recorder`, `upload.Uploader`, `clipboard.Clipboard`, `hotkey.Manager`,
`notify.Notifier`, `tray.Tray`, `edit.Editor`. Per-OS implementations live in `platform/darwin/` and
`platform/windows/`; `cmd/goshareit/wire_<goos>.go` injects them. The portable Nextcloud uploader lives
in the core. The annotation editor is a **separate `goshareit-editor` binary** (Gio, out-of-process)
launched by `edit.Launcher` - this keeps the menu-bar host CGO-free and avoids the systray main-thread
conflict. Upload = WebDAV PUT to `<server>/remote.php/dav/files/<user>/<name>` + OCS public share;
clipboard link is `/s/{token}/preview` for images, `/download` otherwise.

## Phase status

| Phase | Scope | State |
|---|---|---|
| P1 | macOS screenshots (region/full/window), hotkeys, tray, upload, notify | **DONE, verified on hardware** |
| P2 | Windows screenshots (kbinani/GDI, ms-screenclip, RegisterHotKey, systray, toast) | **Code DONE; install (Inno setup.exe), first-run onboarding, tray icon, screenshot capture, and upload verified on hardware. Settings UI beyond first-run, toast notifications, and PrintScreen/alternative hotkey chords still untested.** |
| P3a | Recording video: macOS native AVFoundation->mp4 (no ffmpeg), Windows ffmpeg gdigrab; `Recorder` Start/Stop, record hotkey + tray toggle | **Code DONE; linux+windows build verified; cgo/recording UNTESTED on device (both platforms)** |
| P4a | Annotation editor MVP: Gio out-of-process (crop/arrow/rect/text, undo, confirm/cancel); pure-Go `annotate` ops | **Code DONE; PARTIALLY verified on macOS hardware (window opens, arrow draws, Confirm applies + result uploads); known bug: toolbar overflows at default window width (Confirm cut off) - see BACKLOG** |
| P4b | Editor: blur, pixelate, highlight, step-numbers, line, freehand | **Code DONE; 17 annotate pixel tests pass (CGO off); blur verified end-to-end on macOS hardware; remaining tools untested on device** |
| P3b | GIF (frame-sampling, no ffmpeg) + interactive region selector + region recording | **Code DONE; linux build/vet/test + GOOS=windows verified; Gio overlay + macOS cropRect untested on device (coordinate/DPI accuracy is the key risk)** |
| P4c | Editor: in-window copy/save/upload buttons, full annotation feature set (copy/save/upload in-editor) | **NOT STARTED** |

## On-device validation still owed

- **Windows:** install (Inno setup.exe), first-run onboarding, tray icon, and
  basic screenshot capture + upload are verified on real hardware. Still owed:
  settings UI and the annotation editor beyond first-run (Gio UI compiles only
  on Windows/cgo mac - never opened on device), P3a recording (ffmpeg, needs
  ffmpeg on PATH, clean `q`-stop mp4 finalization), toast notifications
  (unregistered AppUserModelID may be dropped), foreground-window capture on
  multi-monitor/DPI, `RegisterHotKey` beyond the default chords, the updater
  apply/relaunch loop, and PrintScreen/alternative hotkey chords + `*_edit`
  variants.
- **macOS:** tray icon, settings UI (save/restart flow), Screen Recording +
  Accessibility TCC grants (persisting under the signed CI build), screenshot
  capture, upload, local-only mode + the upload toggle, and the updater check
  (verified with a PAT, correctly reports up-to-date) are verified on real
  hardware. Still owed: P3a recording (AVFoundation cgo links; Start->Stop
  yields a playable mp4; `Stop` not called on the main thread), the annotation
  editor (Gio UI: `goshareit-editor` window opens/foregrounds/dismisses; exit
  codes reach the host; view transform + window->image coordinate mapping;
  drag-to-draw; single-line text via `widget.Editor`; crop coordinate folding;
  the bundled `goshareit-editor` found by the launcher), the region selector
  overlay + cropRect coordinate accuracy, edit-variant and alternative
  hotkeys, and the updater apply/relaunch loop.
- **Both platforms:** browser sign-in (Nextcloud Login Flow v2) end-to-end has
  not been exercised on device.

## macOS TCC lesson

An unsigned or inconsistently-signed `.app` gets a new code identity on every
build, so macOS drops the Screen Recording / Accessibility / Input Monitoring
grants each time (the "remove and re-add" dance). The fix is consistent
signing, not re-granting: `scripts/dev-build.sh` auto-discovers a stable local
signing identity for local dev builds, and CI now signs every release bundle
with a stable, team-anchored Developer ID Application identity (as of
v0.0.8 - see docs/PERMISSIONS.md and docs/RELEASE.md). Switching to it from
the old self-signed "RakePro-Dev" identity required a one-time full
remove-and-re-add of TCC permissions on existing installs (a stale grant
under the old identity shadows the new one); from v0.0.8 on the identity is
stable across updates and certificate renewals, so grants persist without
further action. Verified on hardware: TCC grants persist across launches and
updates under the signed CI build.

NOTE: ongoing work now tracks in `CHANGELOG.md` (shipped) and `BACKLOG.md` (planned) at the repo
root - update BOTH with every change; this file stays as the architecture/validation reference.

## P3b - DONE (GIF + region selector + region recording)

- **GIF** via frame-sampling (`internal/core/gifrec`, ~10fps, pure-Go `image/gif`); `capture.NewCompositeRecorder`
  routes GIF vs video. Uploader: `png/jpeg -> /preview`, `gif/video -> /download`. Tray "Start GIF Recording".
- **Region selector**: out-of-process Gio overlay = `goshareit-editor --region` (dim fullscreen, drag a box,
  writes `x,y,w,h`, exit 0/64). Host `region.Launcher` (internal/core/region) execs it -> `image.Rectangle`.
- **Region recording**: `RegionRecorder` optional extension (`StartRegion(ctx, mode, rect)`); base `Recorder`
  unchanged so it degrades to full-screen. Implemented on darwin (AVFoundation `cropRect`, pixel->point + Y-flip),
  windows (ffmpeg `-offset/-video_size`, even-dim rounding), gifrec (pure-Go crop). Tray "Start Region Recording"
  launches the overlay then records the rect.
- **Still owed:** `LastRegion` (replay last region as a screenshot) is NOT wired to the overlay yet - it still
  falls back to interactive. Could store the overlay's last rect and feed `screencapture -R` / `CaptureRect`.

## On-device verification specific to the region selector (highest risk)

- The Gio overlay must render fullscreen + borderless + dimmed on mac/win; drag box + `WxH` readout correct.
- **Coordinate accuracy is the #1 risk:** the overlay's window->screen pixel mapping must match the screen-pixel
  rect the platform `StartRegion` expects. macOS `cropRect` does a pixel->point divide + Y-flip against display
  height (Retina scale 2, multi-display, non-primary origin all suspect). Windows gdigrab `-offset` is relative
  to the virtual desktop. Verify the recorded area matches the selected box pixel-for-pixel. v1 targets the
  PRIMARY display only.

## Release pipeline + self-update

- **Promotion flow (org standard):** `sync-prod.yml` opens a bot `main -> prod` PR; the user's merge
  (MERGE COMMIT only) triggers `release.yml`, which mints the next semver tag (`release:minor`/`release:major`
  PR labels; patch default) and builds all artifacts in the same run. `ci.yml` gained an aggregate `build`
  job - prod protection requires that exact check name.
- **Artifacts per release:** macOS universal (lipo arm64+amd64) `.dmg` + `.zip` (zip = whole `.app`, the
  updater feed); Windows amd64 Inno Setup per-user installer (`%LOCALAPPDATA%\Programs\GoShareIt`, no
  admin) + loose-exe `.zip` (updater feed); Linux `.tar.gz` (EXPERIMENTAL - wire_linux.go still runs
  fakes); `checksums.txt` (sha256, updater fails closed without it). Asset names must stay in sync with
  `update.AssetName()`.
- **macOS signing in CI is gated on secrets** (`MACOS_CERT_P12`(+`_PASSWORD`) to sign; `AC_APPLE_ID`/
  `AC_PASSWORD`/`TEAM_ID` to notarize). As of v0.0.8 the secrets carry a real Developer ID Application
  certificate: the build keychain imports Apple's Developer ID CA intermediates before signing, and the
  workflow signs by the identity's derived SHA-1 hash rather than the `DEVELOPER_ID_APP` name string
  (`DEVELOPER_ID_APP` is no longer load-bearing for CI, only for local `scripts/sign.sh`). Release builds
  are signed AND notarized (notarytool `Accepted`, ticket stapled) - full Gatekeeper credit, not just TCC
  persistence; a build with no signing secrets at all falls back to ad-hoc signing the whole bundle so it
  still has one consistent identity.
- **Self-update:** `internal/core/update` polls the GitHub Releases API (`update:` config section; optional
  fine-grained read-only PAT in `<app root>/github-token.secret` while the repo is private -
  anonymous API takes over when it goes public). Tray item "Check for Updates" -> "Install Update vX.Y.Z";
  background check 30s after launch + every `interval_hours`. Dev builds (`0.0.0-dev`) never auto-check.
  Apply swaps the whole `.app` (darwin, via ditto; same-identity signing preserves TCC grants) or the
  sibling exes (windows/linux, rename-aside). Version is stamped via ldflags into
  `internal/core/version.Version`; Windows release builds add `-H windowsgui`.
- **Verified on device:** the periodic update check, including with a PAT configured, correctly reports
  "up to date" on macOS. **Still untested:** the actual apply/relaunch path on any platform, and the
  Windows Inno installer's own upgrade-in-place behavior (a fresh install via setup.exe is verified).

## Tray icon + settings UI

- **Tray icon shipped:** embedded glyph (capture corners + dot, `internal/icon`, regenerate via
  `scripts/gen-tray-icon.py`): black template PNG on macOS (adaptive menu bar), multi-size ICO on
  Windows. Text-title fallback stays when icon bytes are absent. Verified rendering on both macOS and
  Windows hardware.
- **Settings UI:** `goshareit-settings` - a THIRD sibling binary (Wails v2 + vanilla-JS embedded
  frontend, no node build; v3 was still alpha). Same out-of-process pattern as the editor. Tray gains
  "Settings...". Backend `internal/settings.Service` (pure Go, linux-tested): Load = raw config parse
  (config.LoadRaw - no validation, editable while incomplete), Save = write secrets to their files +
  marshal YAML (comments not preserved) + full config.Load validation so errors surface in the UI.
  Secrets are write-only (never returned to the frontend). Host restarts itself (update.Relaunch)
  when the settings process exits with a changed config mtime.
- **Build notes:** wails v2 windows is CGO-free (cross-compiles from linux); production builds need
  `-tags desktop,production`. Settings binary is bundled in the .app, the windows zip + Inno installer,
  and lipo'd universal on macOS. Linux: excluded (//go:build darwin||windows).
- **Verified on device:** macOS - full save/restart flow, TCC grants persisting under the signed build.
  Windows - the first-run onboarding path only. **Still untested:** the Windows settings UI and general
  save/restart flow beyond first-run.
- **First-run onboarding:** an unloadable config no longer exits silently - the host opens the
  settings UI, blocks, and retries the load after it closes. Found on the first real Windows install:
  the old scaffold-and-exit flow plus `-H windowsgui` (invisible stderr) looked like "app doesn't start".
  All logs now also mirror to `<app root>/goshareit.log` (5MB truncate) for on-device diagnosis. Verified
  on both macOS and Windows hardware.

## Known issues / TODOs (in priority-ish order)

1. **`LastRegion` falls back to interactive** on both macOS (`screencapture -i` gives no rect) and Windows
   (ms-screenclip gives no rect). Needs a custom overlay to capture the rect. TODO(P3b).
2. **Video region records full display** on both platforms (no interactive video-region picker yet). P3b.
3. ~~Menu-bar uses a text title, no icon~~ DONE: embedded tray glyph shipped (see above).
4. ~~First-run is silent~~ DONE: settings-UI onboarding + file logging (see above).
5. **Notifier OpenURL/thumbnail ignored** on both platforms (P1 limitation).

## Build / run quick reference

- Local macOS dev loop: `make dev-run` (build host+editor -> bundle `.app` -> sign with
  `$DEVELOPER_ID_APP` -> launch). Self-signed `GoShareIt Dev` cert is fine for local; grant the `.app`
  Accessibility + Input Monitoring (and Screen Recording on first capture).
- Config: app root is `~/.goshareit` (macOS/Linux) / `%USERPROFILE%\goshareit` (Windows); first run
  scaffolds `config.yaml`, `app-password.secret` and `github-token.secret` there; the app never fatals
  when unconfigured. Pre-v1.1 roots (`~/.config/goshareit`, `~/Library/Application Support/GoShareIt`)
  remain read fallbacks for existing installs. History lives in the same root (`history.jsonl`).
  Enable the editor with `editor.enabled: true` + `on_modes: [region]`.
- CI: `.github/workflows/ci.yml` - linux core (CGO off) + Windows cross-build + macOS cgo build of
  host+editor+settings. `.github/workflows/release.yml` - the actual release build/sign/publish path,
  see docs/RELEASE.md. Old prototype code is parked under `_prototype/`.
