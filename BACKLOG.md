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
  region overlay coordinates (now also the path for still region capture),
  settings UI and editor beyond first-run, updater apply loop, PrintScreen
  hotkey chords on the direct RegisterHotKey path (with
  `hotkeys.disable_snipping_printscreen`: confirm whether the registry flip
  takes effect live or only after sign-out).
- Both (Wails v3 migration, nothing below has run on real hardware): tray icon
  and menu, greying/retitling of the recording items, global hotkeys firing
  while unfocused, the update-confirm dialog, notifications and their
  click-to-open-link, "Start at login" writing and surviving a reboot, and
  quit from both the tray item and a hotkey.
- macOS (Wails v3): that hotkeys now work with Accessibility and Input
  Monitoring revoked, and that the signed bundle still notarizes with the new
  Carbon / ServiceManagement links.
- Windows (Wails v3): toasts from the in-process WinRT path (the app has no
  icon resource, so the toast icon may be missing), the HKCU CLSID activator
  the notification service registers, and whether a toast click reaches a
  running single-instance host.
- Both: browser sign-in (Nextcloud Login Flow v2) end-to-end.
- v0.0.6 settings/editor UI (click-through on real hardware, not just
  container JS parse-checks): the consolidated Upload destination panel
  (Nextcloud/S3/SFTP/WebDAV/Custom select + presets) and the new theme
  setting (light/dark/system) in both the settings window and the
  annotation editor.

## Features

- Editor: live raster previews for blur/pixelate (render through the
  annotate ops instead of the approximate grey-box preview).
- LastRegion capture: reuse the last selected rectangle without re-picking
  (store the overlay's rect, feed `screencapture -R` / `CaptureRect`).
- Multi-monitor support for region selection and recording (v1 is
  primary-display only).
- Upload history browser (history.jsonl exists; no UI over it).
- Notifier improvements: thumbnail previews (click-to-open-link shipped
  with the Wails v3 migration).
- Linux capture backend (tray/hotkey/capture are in-memory fakes today;
  artifacts ship marked experimental).
- Custom-uploader imgur preset needs a user-registered imgur API client ID
  (not bundled) - document where to get one and where it goes in
  config.example.yaml / the settings UI preset picker.

## Wails v3 migration - DONE on branch wails-v3

Migrated at v3.0.0-beta.18 (v2 was bugfix-only with no maintenance commitment
past v3 GA). Do NOT adopt the wails3 CLI/Taskfile; plain `go build` remains our
path, and both the host and the settings binary build with `-tags production`.

Moved to Wails v3:

| Area | Was | Now |
|------|-----|-----|
| Settings UI | wails/v2 + `desktop,production` tags + a UTType CGO_LDFLAGS workaround | `application.New` + services bindings, `-tags production` |
| Tray | `fyne.io/systray` | `app.SystemTray` (`platform/wailsapp`) |
| Global hotkeys | `golang.design/x/hotkey` (CGEventTap on macOS) | `app.GlobalShortcut` (Carbon RegisterEventHotKey / Win32 RegisterHotKey), plus a PrintScreen-only direct path on Windows |
| Notifications | UNUserNotificationCenter cgo + osascript / PowerShell toast | `pkg/services/notifications`, click-to-open-link wired |
| Confirm dialogs | NSAlert cgo / PowerShell MessageBox | `app.Dialog.Question` |
| Start at login | not implemented | `app.Autostart` + `start_at_login` setting |

Stays as it is (no v3 equivalent needed): `platform/darwin` capture,
`recorder.m`, `permissions.m` (Screen Recording only), `platform/windows`
capture/recorder/clipboard/PrintScreen registry + PrintScreen hotkey path,
`kbinani/screenshot`, the Gio editor, and one PowerShell dialog for the Smart
App Control notice (the Wails dialog API cannot render Yes/No/Cancel on
Windows).

Open:

- Drop `platform/windows/hotkey.go` (the PrintScreen-only `RegisterHotKey`
  path, kept because the Wails accelerator grammar has no name for that key:
  `parseKey` rejects it and `winKeyCodes` has no entry) once Wails gains a
  "printscreen" key name, and route those chords through `app.GlobalShortcut`
  like every other chord.
- The `.app` bundle now links Carbon and ServiceManagement (global shortcuts,
  SMAppService autostart). One signed-build pass on device is owed to confirm
  the bundle still notarizes and that no new TCC prompt appears.

## Release / distribution

- Windows .exe icon: `goshareit.exe`/`-editor.exe`/`-settings.exe` ship with
  no icon resource, so Explorer/taskbar/the Inno installer's
  `UninstallDisplayIcon` all show the generic exe icon. (macOS app icon is
  done - AppIcon.icns ships as of v0.0.6.) The master logo exists at
  build/icons/goshareit_icon.png - embed it into the Windows binaries (e.g.
  a `.syso` resource) and wire it into goshareit.iss.
- Windows Authenticode signing: AT AN IMPASSE (2026-09-06). SignPath
  Foundation did not accept the project (size), and paid certificates are
  out by owner decision. CI wiring stays in release.yml, dormant, gated on
  `SIGNPATH_API_TOKEN` in case that changes. Mitigation shipped instead: the
  installer and first launch explain Smart App Control and offer the
  turn-off page (see CHANGELOG); README documents it.
- Microsoft Store distribution (second install path, alongside the GitHub
  release): CI SIDE DONE 2026-09-06 - release.yml builds the MSIX upload
  package as a workflow artifact, the host disables its updater when
  packaged, the SAC dialogs point users at the Store. OPEN, owner-side:
  (1) Partner Center individual developer account - carries a one-time
  registration fee (USD 19 at last check), owner decides; (2) reserve the
  app name, copy the Package/Identity/Name and Publisher values into repo
  variables `MSSTORE_IDENTITY_NAME` and `MSSTORE_PUBLISHER`; (3) upload the
  artifact from the next release and fill in the listing (screenshots,
  privacy policy URL = PRIVACY.md, age rating); (4) after the first
  publication swap `windows.MSStoreURL` from the search URL to
  `ms-windows-store://pdp/?ProductId=<id>`. Later: automate the upload via
  the Partner Center submission API. Untested until then: toast
  notifications and the PrintScreen registry tweak from inside the package
  (MSIX virtualizes HKCU writes), startup-task wiring.
- 1.0.0 criteria: all on-device validation above green.
