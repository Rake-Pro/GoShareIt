# Backlog

Planned and open work, roughly prioritized. Update this file whenever a
feature is planned, started, or shipped (shipped items move to
[CHANGELOG.md](CHANGELOG.md)).

## On-device validation owed

Verified on real hardware: Windows install (setup.exe), first-run onboarding,
tray icon, screenshot capture, upload; macOS tray icon, settings UI
(save/restart flow), TCC permissions (granted and persisting under the signed
build), screenshot capture, upload, local-only mode + upload toggle, and the
updater check (with a PAT, correctly reports up-to-date).

- macOS: recording (AVFoundation cgo), region selector overlay + cropRect
  coordinate accuracy (Retina scale / Y-flip), updater apply/relaunch loop,
  edit-variant and alternative/punctuation hotkeys. Annotation editor is
  PARTIALLY verified (opens, arrow + blur draw and apply on Confirm); rest
  of the tool set and text entry still owed, plus the toolbar-overflow bug
  under Features.
- Windows: annotation editor UI, recording (ffmpeg), toast notifications,
  region overlay coordinates, settings UI and editor beyond first-run,
  updater apply loop, PrintScreen hotkey chords.
- Both: browser sign-in (Nextcloud Login Flow v2) end-to-end.
- v0.0.6 settings/editor UI (click-through on real hardware, not just
  container JS parse-checks): the consolidated Upload destination panel
  (Nextcloud/S3/SFTP/WebDAV/Custom select + presets) and the new theme
  setting (light/dark/system) in both the settings window and the
  annotation editor.

## Features

- Editor: live raster previews for blur/pixelate (render through the
  annotate ops instead of the approximate grey-box preview).
- In-editor copy / save / upload buttons (finish the annotation workflow).
- LastRegion capture: reuse the last selected rectangle without re-picking
  (store the overlay's rect, feed `screencapture -R` / `CaptureRect`).
- Multi-monitor support for region selection and recording (v1 is
  primary-display only).
- Start-at-login toggle in the settings UI (installer task exists on Windows;
  macOS needs a LaunchAgent).
- Upload history browser (history.jsonl exists; no UI over it).
- Notifier improvements: click-to-open-link, thumbnail previews.
- Linux capture backend (tray/hotkey/capture are in-memory fakes today;
  artifacts ship marked experimental).
- Custom-uploader imgur preset needs a user-registered imgur API client ID
  (not bundled) - document where to get one and where it goes in
  config.example.yaml / the settings UI preset picker.

## Wails v3 migration (do at v3.0.0-rc.1, targeted 2026-09-12)

Migration is inevitable: v2 is bugfix-only with no maintenance commitment past
v3 GA (milestones: beta.2 code freeze Sep 1, rc.1 Sep 12, GA Sep 15 - expect
slippage; check the wailsapp/wails milestone board mid-September). The v3
architecture (namespaced managers, services bindings) is settled - zero
breaking changes across the beta series - and the no-node/no-CLI build path is
officially supported (`v3/examples/plain`), so our zero-tooling shape survives.
Assessed 2026-08-16; scope is roughly half a day plus an on-device pass.

Structural changes when we do it (all in `cmd/goshareit-settings/`):

- `main.go`: `wails.Run(&options.App{...})` -> `application.New(application.Options{
  Services: []application.Service{application.NewService(svc)}, Assets: ...})`,
  then `app.Window.New()` (WebviewWindowOptions: title/size) + `app.Run()`.
  The OnStartup context capture goes away - runtime calls hang off `app`
  directly: `app.Dialog` (OpenDirectoryDialog), browser-open util, `app.Quit()`.
- `frontend/index.html`: `window.go.settings.Service.<Method>` no longer
  exists. Add `<script type="module" src="/wails/runtime.js"></script>` (served
  from the binary, no npm) and a small shim mapping our methods over
  `window.wails.Call.ByName("github.com/Rake-Pro/GoShareIt/internal/settings.Service.<Method>", ...)`.
  ByName takes the full import-path FQN - keep it in ONE shim constant so a
  package move can't silently break scattered call sites.
- Makefile + scripts/dev-build.sh: build tag `desktop,production` -> just
  `production` (no `desktop` tag in v3). Drop the
  `CGO_LDFLAGS="-framework UniformTypeIdentifiers"` workaround (v3 declares it
  properly) - verify, then delete.
- macOS bundle: our own bundle/sign/notarize scripts carry over unchanged (we
  never used the wails CLI), but v3 links new system frameworks (QuartzCore,
  Carbon, ServiceManagement) - do one signed-build TCC pass on device.
- Do NOT adopt the wails3 CLI/Taskfile (their build tooling is still unsettled
  - "Wake" experiment); plain `go build` remains our path.
- Watch wailsapp/wails#5868 (v2->v3 migration validation for RC1) and the GA
  deprecation-removal gate for anything that lands between now and rc.1.
- Later option (separate decision, not this migration): v3 has native systray,
  global shortcuts, and login-item autostart - could eventually consolidate
  the tray host / hotkey / start-at-login architecture.

## Release / distribution

- Windows .exe icon: `goshareit.exe`/`-editor.exe`/`-settings.exe` ship with
  no icon resource, so Explorer/taskbar/the Inno installer's
  `UninstallDisplayIcon` all show the generic exe icon. (macOS app icon is
  done - AppIcon.icns ships as of v0.0.6.) The master logo exists at
  build/icons/goshareit_icon.png - embed it into the Windows binaries (e.g.
  a `.syso` resource) and wire it into goshareit.iss.
- Windows Authenticode signing (SmartScreen). Deliberately deferred - not a
  priority for now (owner call, 2026-07-30).
- 1.0.0 criteria: all on-device validation above green.
