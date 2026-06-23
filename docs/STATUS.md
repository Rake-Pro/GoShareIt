# GoShareIt - Project Status & Remaining Work

Snapshot date: 2026-06-24. Last commit: `1f45824` (P4b). Branch: `main` (pushed to origin).
P1-P4b code-complete; macOS recording + Gio editor still need on-device verification. See "Recent fixes" and "P3b design forks" below.

GoShareIt is a ShareX-style screenshot/recording + Nextcloud-upload tool. Goal: 1:1 ShareX
screenshot-capture parity on **macOS and Windows from one Go codebase**. Architecture, rationale,
and the cross-platform feasibility analysis are in the design docs (`docs/DESIGN-recording.md`,
`docs/DESIGN-editor.md`).

## Architecture (one-paragraph map)

Pure-Go **core** under `internal/core/` (CGO-free, linux-testable) depends only on interface seams:
`capture.Capturer`, `capture.Recorder`, `upload.Uploader`, `clipboard.Clipboard`, `hotkey.Manager`,
`notify.Notifier`, `tray.Tray`, `edit.Editor`. Per-OS implementations live in `platform/darwin/` and
`platform/windows/`; `cmd/goshareit/wire_<goos>.go` injects them. The portable Nextcloud uploader lives
in the core. The annotation editor is a **separate `goshareit-editor` binary** (Gio, out-of-process)
launched by `edit.Launcher` - this keeps the menu-bar host CGO-free and avoids the systray main-thread
conflict. Upload = WebDAV PUT to `cloud.rake.pro/remote.php/dav/files/imgshare/<name>` + OCS public share;
clipboard link is `/s/{token}/preview` for images, `/download` otherwise.

## Phase status

| Phase | Scope | State |
|---|---|---|
| P1 | macOS screenshots (region/full/window), hotkeys, tray, upload, notify | **DONE, verified on hardware** |
| P2 | Windows screenshots (kbinani/GDI, ms-screenclip, RegisterHotKey, systray, toast) | **Code DONE; `GOOS=windows` build verified; UNTESTED on real Windows** |
| P3a | Recording video: macOS native AVFoundation->mp4 (no ffmpeg), Windows ffmpeg gdigrab; `Recorder` Start/Stop, record hotkey + tray toggle | **Code DONE; linux+windows build verified; cgo/recording UNTESTED on device** |
| P4a | Annotation editor MVP: Gio out-of-process (crop/arrow/rect/text, undo, confirm/cancel); pure-Go `annotate` ops | **Code DONE; builds verified; Gio UI UNTESTED on device** |
| P4b | Editor: blur, pixelate, highlight, step-numbers, line, freehand | **Code DONE; 17 annotate pixel tests pass (CGO off); GOOS=windows build verified; Gio UI UNTESTED on device** |
| P3b | GIF (frame-sampling, no ffmpeg) + interactive region selector + region recording | **Code DONE; linux build/vet/test + GOOS=windows verified; Gio overlay + macOS cropRect untested on device (coordinate/DPI accuracy is the key risk)** |
| P4c | Editor: in-window copy/save/upload buttons, full ShareX/Greenshot parity | **NOT STARTED** |

## On-device validation still owed (nothing below has run on real hardware)

- **Windows (all of P2 + P3a Windows):** build on a Windows box (host + editor cross-compile CGO-free,
  but never run). Verify: ms-screenclip clipboard round-trip; foreground-window capture on multi-monitor/DPI;
  toast notifications (unregistered AppUserModelID may be dropped); `RegisterHotKey`; systray; ffmpeg
  recording (needs ffmpeg on PATH) and clean `q`-stop mp4 finalization.
- **macOS recording (P3a):** AVFoundation recorder compiles only on a Mac. Verify: cgo links
  (Foundation/AVFoundation/CoreMedia/CoreGraphics); Screen Recording TCC held by the signed `.app`;
  Start->Stop yields a playable mp4; `Stop` not called on the main thread.
- **Editor (P4a):** Gio UI compiles only with cgo (mac) / on Windows. Verify: `goshareit-editor` window
  opens/foregrounds/dismisses; exit codes 0/64/1 reach the host; view transform + window->image coordinate
  mapping; drag-to-draw; single-line text via `widget.Editor`; crop coordinate folding; the bundled
  `goshareit-editor` beside the host in `Contents/MacOS/` is found by the launcher.

## Recent fixes (2026-06-24, on top of P1-P4b)

- **macOS TCC staleness FIXED at the root:** `dev-build.sh` now auto-discovers the `GoShareIt Dev`
  cert and always signs, so Accessibility/Screen Recording/Input Monitoring grants persist across
  rebuilds (the recurring "remove and re-add" dance was caused by shipping UNSIGNED bundles). Plus a
  darwin permission preflight (`platform/darwin/permissions.{go,m}`, called from `wire_darwin.go`)
  that requests all three on startup so the user gets real prompts. (cgo - verify on a Mac.)
- **Windows hotkey mapping FIXED:** `Cmd/Command/Super/Meta` now map to `Ctrl` (not OS-reserved `Win`),
  so a shared macOS config registers on Windows. `Win` still maps to the logo key.
- **Recorder temp-leak FIXED:** both recorders delete the temp mp4 after reading bytes.
- **Editor text UX FIXED:** selecting the Text tool focuses the toolbar field; it clears after placing.
- **Tray:** separate greyed Start/Stop recording items; hotkeys shown on every menu label.
- **Logging:** zerolog everywhere (first-run msg + editor errors converted).

## Known issues / TODOs (in priority-ish order)

1. **`LastRegion` falls back to interactive** on both macOS (`screencapture -i` gives no rect) and Windows
   (ms-screenclip gives no rect). Needs a custom overlay to capture the rect. TODO(P3b).
2. **Video region records full display** on both platforms (no interactive video-region picker yet). P3b.
3. **Menu-bar uses a text title, no icon.** Add an `.icns`/template icon - HELD pending a design decision
   (generated glyph vs user-supplied `.icns`); `systray.SetIcon` on both trays + `build/macos/`.
4. **First-run via `open` is silent** (stderr hidden). A GUI first-run dialog would help; low priority.
5. **Notifier OpenURL/thumbnail ignored** on both platforms (P1 limitation).

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

## Build / run quick reference

- Local macOS dev loop: `make dev-run` (build host+editor -> bundle `.app` -> sign with
  `$DEVELOPER_ID_APP` -> launch). Self-signed `GoShareIt Dev` cert is fine for local; grant the `.app`
  Accessibility + Input Monitoring (and Screen Recording on first capture).
- Config: first run scaffolds `~/.config/goshareit/{config.yaml,app-password.secret}`; the app never
  fatals when unconfigured. Enable the editor with `editor.enabled: true` + `on_modes: [region]`.
- CI: `.github/workflows/ci.yml` - linux core (CGO off) + Windows cross-build + macOS cgo build of
  host+editor. Signing gated behind `DEVELOPER_ID_APP` secret.
- Helper container note (for the assistant): Go is at `/home/claude/go` (set `GOROOT=/home/claude/go`,
  `GOTMPDIR=/home/claude/gotmp`; `/tmp` is noexec). Use `go get` not `go mod tidy` on linux (tidy prunes
  the darwin/windows-only deps). Old prototype is parked under `_prototype/`.

## Housekeeping

- **Rotate the `imgshare@rake.pro` app password** used during testing (it was shared in chat). Nextcloud ->
  Settings -> Security -> Devices & sessions.
- Test screenshot + its public share were left on imgshare's Nextcloud root (kept intentionally; delete
  whenever).

## Resume plan (after container restart)

1. Re-clone `https://github.com/Rake-Pro/GoShareIt` (workspace is ephemeral; repo is the source of truth).
2. Read this file + the design docs.
3. Decide next: either (a) on-device validation of P2/P3a/P4a on real Mac/Windows and fix what breaks,
   or (b) implement P3b (GIF + region) / P4b (blur/highlight/steps).
