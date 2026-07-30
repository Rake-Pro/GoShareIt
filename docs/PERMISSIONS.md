# macOS Permissions

GoShareIt needs two macOS privacy (TCC) permissions. Both are granted at runtime
by the user in System Settings; neither can be declared in `Info.plist`. They are
remembered per app **only when the app is code-signed** (an ad-hoc/unsigned build
gets re-prompted or silently denied after every rebuild).

## 1. Screen Recording

- **Why:** capturing the screen, a region, or a window is screen recording as far
  as macOS is concerned.
- **Grant it:** System Settings > Privacy & Security > Screen Recording > enable
  **GoShareIt**.
- The first capture attempt triggers the system prompt (purpose string from
  `NSScreenCaptureUsageDescription`). After enabling, macOS may require you to
  quit and reopen the app.

## 2. Accessibility / Input Monitoring (global hotkeys)

- **Why:** the global capture hotkeys are registered system-wide via
  `golang.design/x/hotkey`, which needs the OS to deliver key events to the app
  even when it is not focused.
- **Grant it:** System Settings > Privacy & Security > **Accessibility** > enable
  **GoShareIt**. If hotkeys still do not fire, also enable it under **Input
  Monitoring** in the same Privacy & Security list.

## Persistence and signing

TCC ties a grant to the app's code signature (Team ID + bundle id). What
matters for persistence is a **stable signing identity**, not notarization:
a GoShareIt build signed with the same identity every time keeps its
permissions across launches and updates. Unsigned or
re-signed-with-a-different-identity builds are treated as a new app and must
be re-authorized.

Release builds from CI are codesigned with a stable, team-anchored Developer
ID Application identity (as of v0.0.8 - see [RELEASE.md](RELEASE.md)), so
downloaded builds keep TCC grants across updates the same way a locally
signed dev build does, and the identity now carries full Gatekeeper credit
(the build is also notarized). Switching from the previous interim
self-signed "RakePro-Dev" identity required a one-time full remove-and-re-add
of these permissions on any install that already had them granted (a stale
grant under the old identity shadows the new one - see "Resetting for
testing" below); from v0.0.8 on the identity is stable across updates and
certificate renewals, so no further re-grant is expected.

## Resetting for testing

To re-test the first-run prompt, reset the relevant TCC entries:

```
tccutil reset ScreenCapture pro.rake.goshareit
tccutil reset Accessibility pro.rake.goshareit
tccutil reset ListenEvent pro.rake.goshareit   # Input Monitoring
```

Omit the bundle id to reset the permission for every app. Replace
`pro.rake.goshareit` if you built with a different `BUNDLE_ID`.
