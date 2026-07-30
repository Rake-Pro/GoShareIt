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

## Release / distribution

- Windows .exe icon: `goshareit.exe`/`-editor.exe`/`-settings.exe` ship with
  no icon resource, so Explorer/taskbar/the Inno installer's
  `UninstallDisplayIcon` all show the generic exe icon. (macOS app icon is
  done - AppIcon.icns ships as of v0.0.6.) The master logo exists at
  build/icons/goshareit_icon.png - embed it into the Windows binaries (e.g.
  a `.syso` resource) and wire it into goshareit.iss.
- Windows Authenticode signing (SmartScreen). Deliberately deferred - not a
  priority for now (owner call, 2026-07-30).
- Public release: repo audit pass, then public visibility; updater switches
  to anonymous GitHub API automatically.
- 1.0.0 criteria: all on-device validation above green + public release.
