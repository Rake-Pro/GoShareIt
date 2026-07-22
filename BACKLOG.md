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

## Features

- Editor BUG (macOS, on-device): the toolbar overflows at the default window
  width - the Confirm button is cut off until the window is widened
  (MacBook default size). Fix candidates: wrap the toolbar to a second row,
  pin Confirm/Cancel to the window edge with the tool list truncating
  first, or raise the window's minimum/initial width to fit the toolbar.
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

## Release / distribution

- App icons: .icns for the macOS bundle, .ico for the Windows exe (tray glyph
  exists; app-level icons do not).
- Replace the interim self-signed macOS cert with a real Developer ID
  Application cert: the current CI secrets carry a self-signed "RakePro-Dev"
  identity (TeamIdentifier not set - fine for TCC persistence on personal
  machines, zero Gatekeeper credit). Create the cert via Xcode/portal
  (Account Holder), re-export the p12, replace MACOS_CERT_P12(+_PASSWORD)
  and DEVELOPER_ID_APP; expect one TCC re-grant on the identity change.
- macOS notarization in CI (secrets-gated path exists; add AC_APPLE_ID /
  AC_PASSWORD / TEAM_ID once the real cert is in).
- Windows Authenticode signing (SmartScreen).
- Public release: repo audit pass, then public visibility; updater switches
  to anonymous GitHub API automatically.
- 1.0.0 criteria: all on-device validation above green + public release.
