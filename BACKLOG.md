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
  of the tool set and text entry still owed. The toolbar-overflow bug is
  fixed in Unreleased (wrapping toolbar, see CHANGELOG); its on-device check
  is listed below.
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
- Linux (new desktop shell, nothing has run on real hardware): tray icon and
  menu under a StatusNotifierItem host (KDE, GNOME + AppIndicator), global
  hotkeys on X11 (`XGrabKey`) and on Wayland (GlobalShortcuts portal binding
  dialog), D-Bus notifications and click-to-open-link, the GTK confirm
  dialog, "Start at login" (`~/.config/autostart`), the settings UI
  (WebKitGTK), the Gio editor and region overlay (freeze-frame crop
  accuracy on X11, and on Wayland against the portal's stitched
  multi-monitor image), window capture on X11 (frame extents) and the
  portal picker on Wayland, LastRegion replay, ffmpeg `x11grab` recording
  (full + region offsets on a multi-monitor X11 layout), GIF via frame
  sampling, the clipboard on both session types, the updater's tar.gz
  install, and the dark-theme detection (portal Settings / gsettings).
- v0.0.6 settings/editor UI (click-through on real hardware, not just
  container JS parse-checks): the consolidated Upload destination panel
  (Nextcloud/S3/SFTP/WebDAV/Custom select + presets) and the new theme
  setting (light/dark/system) in both the settings window and the
  annotation editor.
- Text recognition and the v0.4.0 editor round (Unreleased; compiled and
  unit-tested, never run on a device): Apple Vision recognition (the
  macos-build CI job runs a synthetic-image test; the signed, notarized
  bundle must still launch with no new TCC prompt); Windows OCR with and
  without an OCR language pack (`go test ./platform/windows/winocr/` on a
  Windows machine: the bindings are hand-checked vtable code); tesseract on
  X11 and Wayland, including installing it while the host runs and the
  host's clipboard re-assert of copied text after the editor closes; the
  greyed Copy text reason matching Settings; drag-select, double-click
  line, Ctrl/Cmd+C text vs image, Copy text with no selection (all text),
  Redact selection, Quick redact and its single undo step, an opaque Redact in the saved PNG; zoom keys and
  a pixel-exact 100% on HiDPI; single-key tools not firing while typing;
  anti-aliased arrows/lines/text at stroke 1, 6 and 32.
- Capture Text and the v0.4.1 editor round (Unreleased; compiled and
  unit-tested, never run on a device): the tray item reading "checking..."
  only briefly at start, then greyed with the reason in its title (and as a
  tooltip on macOS/Linux) while recognition is unavailable, enabling itself
  on Linux within 30 s of installing tesseract; the Settings hotkey field
  editable while unavailable and greyed only with text recognition off; on
  Windows an overlay failure reported instead of the snip fallback;
  the default `{mod}+Shift+8` hotkey registering on each OS; region ->
  "Text copied (N characters)" notification and the text pasting in another
  app, also on Wayland after a while (the host holds the selection); Esc in
  the overlay stays quiet; a hotkey press while unavailable notifies the
  reason. Editor: Select (`V`) picks thin strokes and filled shapes, drag
  moves, Delete/Backspace removes (macOS Delete key), arrow and Shift+arrow
  nudges, swatch and `[`/`]` restyle the selection, Esc order (drag, then
  selection, then cancel), undo/redo across moves, deletes and crop edits;
  crop handles: eight squares, resize cursors, hit areas the same size on a
  HiDPI display at any zoom, Enter confirms with a crop mid-drag.
- Editor toolbar and Live Text round (Unreleased; compiled and unit-tested,
  never run on a device): at the default 1000 dp window and at the 640 dp
  minimum, no button or swatch is cut off and nothing scrolls; tool labels
  lose their "(V)" keys only when they would not fit on one row; the actions
  wrap right-aligned with Cancel and Confirm visible; the Text tool's field
  stretches into a shared row; Select: I-beam over text, drag-select,
  double-click line, a drag on a shape over text moves the shape, Cmd/Ctrl+C
  text vs image, Cmd/Ctrl+A, Esc clearing the selection first; Copy text
  ("Reading text..." while running, greyed with the reason when OCR is off,
  the "N lines of text found" hint once, naming V outside Select); Redact
  present with a customised `editor.tools`; the toolbar height staying put
  on V/A/T at 900 dp; text under a Redact box not selectable or copied, and
  back after Undo; auto_run off with default_tool select gives the I-beam
  without a tool switch. The I-beam covers each line box plus 8 image px,
  gaps between words included (accepted as Live Text-like).

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
- Linux follow-ups (the shell shipped in Unreleased): video/GIF recording
  on Wayland (ScreenCast portal + PipeWire consumer, or an external tool
  such as `wf-recorder`/`gpu-screen-recorder` behind a config knob); a
  PrintScreen hotkey path on X11 (direct `XGrabKey` of the Print keysym,
  like the Windows RegisterHotKey split) since the Wails key table has no
  name for it; a GIF path on Wayland that does not round-trip the portal
  per frame; region selection beyond the first xinerama screen; a
  `.desktop`/icon install step or a package (AppImage/deb) instead of the
  bare tarball.

## Wails v3 migration: DONE on branch wails-v3

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
  done: AppIcon.icns ships as of v0.0.6.) The master logo exists at
  build/icons/goshareit_icon.png; embed it into the Windows binaries (e.g.
  a `.syso` resource) and wire it into goshareit.iss.
- Windows Authenticode signing: AT AN IMPASSE (2026-09-06). SignPath
  Foundation did not accept the project (size), and paid certificates are
  out by owner decision. CI wiring stays in release.yml, dormant, gated on
  `SIGNPATH_API_TOKEN` in case that changes. Mitigation shipped instead: the
  installer and first launch explain Smart App Control and offer the
  turn-off page (see CHANGELOG); README documents it.
- Microsoft Store distribution (second install path, alongside the GitHub
  release): CI SIDE DONE 2026-09-06: release.yml builds the MSIX upload
  package as a workflow artifact, the host disables its updater when
  packaged, the SAC dialogs point users at the Store. OPEN, owner-side:
  (1) Partner Center individual developer account, carries a one-time
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
