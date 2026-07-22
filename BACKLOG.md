# Backlog

Planned and open work, roughly prioritized. Update this file whenever a
feature is planned, started, or shipped (shipped items move to
[CHANGELOG.md](CHANGELOG.md)).

## On-device validation owed

- macOS: recording (AVFoundation cgo), annotation editor UI, region selector
  overlay + cropRect coordinate accuracy (Retina scale / Y-flip), settings UI,
  updater apply/relaunch loop, edit-variant and alternative hotkeys.
- Windows: annotation editor UI, recording (ffmpeg), toast notifications,
  region overlay coordinates, settings UI beyond first-run, updater apply
  loop, PrintScreen hotkey chords.

## Features

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

## Release / distribution

- App icons: .icns for the macOS bundle, .ico for the Windows exe (tray glyph
  exists; app-level icons do not).
- macOS signing + notarization in CI (secrets-gated path exists, unused).
- Windows Authenticode signing (SmartScreen).
- Public release: repo audit pass, then public visibility; updater switches
  to anonymous GitHub API automatically.
- 1.0.0 criteria: all on-device validation above green + public release.
