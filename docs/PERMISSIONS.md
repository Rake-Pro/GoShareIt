# macOS Permissions

GoShareIt needs one macOS privacy (TCC) permission plus the standard
Notifications consent. Both are granted at runtime by the user; they cannot be
declared in `Info.plist`. They are remembered per app **only when the app is
code-signed** (an ad-hoc/unsigned build gets re-prompted or silently denied
after every rebuild).

## 1. Screen Recording

- **Why:** capturing the screen, a region, or a window is screen recording as far
  as macOS is concerned.
- **Grant it:** System Settings > Privacy & Security > Screen Recording > enable
  **GoShareIt**.
- The first capture attempt triggers the system prompt (purpose string from
  `NSScreenCaptureUsageDescription`). After enabling, macOS may require you to
  quit and reopen the app.

## 2. Notifications

- **Why:** the "uploaded, link copied" toast is a `UNUserNotificationCenter`
  notification, delivered through the Wails v3 notifications service.
- **Grant it:** the first notification triggers the system consent prompt.
  Later changes live in System Settings > Notifications > **GoShareIt**.
- Denied or unbundled (plain `go build`, no `.app`) builds log the failure and
  skip the toast; the upload itself and the clipboard copy are unaffected.

## Global hotkeys need no permission

Global capture hotkeys are registered with Carbon's `RegisterEventHotKey` (via
the Wails v3 global-shortcut manager), which the OS delivers without any TCC
grant. Accessibility and Input Monitoring are **no longer requested or
required**; they were needed only by the previous `golang.design/x/hotkey`
event-tap backend. An install that granted them for an older build can revoke
them.

## Text recognition needs no permission

The editor's text recognition uses Apple Vision (`VNRecognizeTextRequest`) on
the image it already holds. Capture Text does the same in the host process,
on a region taken by the same capture as Capture Region (so it needs only the
Screen Recording permission that capture already has). Vision is a public system framework that runs on
device: no entitlement, no TCC prompt, no network. This is expected, not yet
verified on a signed, notarized build (see BACKLOG.md).

## Persistence and signing

TCC ties a grant to the app's code signature (Team ID + bundle id). What
matters for persistence is a **stable signing identity**, not notarization:
a GoShareIt build signed with the same identity every time keeps its
permissions across launches and updates. Unsigned or
re-signed-with-a-different-identity builds are treated as a new app and must
be re-authorized.

Release builds from CI are codesigned with a stable, team-anchored Developer
ID Application identity (as of v0.0.8; see [RELEASE.md](RELEASE.md)), so
downloaded builds keep TCC grants across updates the same way a locally
signed dev build does, and the identity now carries full Gatekeeper credit
(the build is also notarized). Switching from the previous interim
self-signed "RakePro-Dev" identity required a one-time full remove-and-re-add
of these permissions on any install that already had them granted (a stale
grant under the old identity shadows the new one; see "Resetting for
testing" below); from v0.0.8 on the identity is stable across updates and
certificate renewals, so no further re-grant is expected.

## Resetting for testing

To re-test the first-run prompt, reset the relevant TCC entries:

```
tccutil reset ScreenCapture pro.rake.goshareit
```

Notification consent is not a TCC service; reset it by removing GoShareIt
under System Settings > Notifications, or with `tccutil reset All
pro.rake.goshareit` alongside the entry above.

Omit the bundle id to reset the permission for every app. Replace
`pro.rake.goshareit` if you built with a different `BUNDLE_ID`.

## Linux

Nothing is granted up front. On X11 the app reads the screen and grabs keys
directly. On Wayland the compositor mediates through XDG desktop portals:
the Screenshot portal may ask once whether GoShareIt may take screenshots,
and the GlobalShortcuts portal may show a binding dialog the first time the
hotkeys are registered (GNOME 45+, KDE Plasma 5.27+). Both answers are kept
by the portal's permission store; reset them from the desktop's Settings >
Apps (GNOME) or `flatpak permission-reset` style tooling if you need the
prompts again. Windows and Linux have no notification permission.
