# GoShareIt - Project Status & Remaining Work

Snapshot date: 2026-06-23. Last commit at pause: `3363aee` (P4a). Branch: `main` (pushed to origin).

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
| P3b | GIF + video region crop | **NOT STARTED** |
| P4b | Editor: blur/pixelate, highlight, step-numbers, freehand | **NOT STARTED** |
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

## Known issues / TODOs (in priority-ish order)

1. **Windows hotkey defaults are OS-reserved.** macOS `Cmd+Shift+{1,9,0,R}` map to `Win+Shift+{...}` on
   Windows, which Windows reserves -> `RegisterHotKey` will likely fail. Add per-OS default hotkeys (or map
   `Cmd`->`Ctrl` on Windows) in config/wiring before Windows testing.
2. **Recorder temp files leak.** Both `platform/darwin/recorder.go` and `platform/windows/recorder.go` read
   the recorded `.mp4` into bytes but do not delete the temp file afterward (unless SaveLocal). Add cleanup.
3. **`LastRegion` falls back to interactive** on both macOS (`screencapture -i` gives no rect) and Windows
   (ms-screenclip gives no rect). Needs a custom overlay to capture the rect. TODO(P2/P3b).
4. **Video region records full display** on both platforms (no interactive video-region picker yet). P3b.
5. **Menu-bar uses a text title, no icon.** Add an `.icns`/template icon (`build/macos/`, see `bundle.sh`).
6. **First-run via `open` is silent** (stderr hidden). A GUI first-run dialog would help; low priority.
7. **Notifier OpenURL/thumbnail ignored** on both platforms (P1 limitation).

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
