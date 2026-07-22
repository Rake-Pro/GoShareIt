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

TCC ties a grant to the app's code signature (Team ID + bundle id). A properly
signed + notarized GoShareIt keeps its permissions across launches and updates.
Unsigned or re-signed-with-a-different-identity builds are treated as a new app
and must be re-authorized.

## Resetting for testing

To re-test the first-run prompt, reset the relevant TCC entries:

```
tccutil reset ScreenCapture pro.rake.goshareit
tccutil reset Accessibility pro.rake.goshareit
tccutil reset ListenEvent pro.rake.goshareit   # Input Monitoring
```

Omit the bundle id to reset the permission for every app. Replace
`pro.rake.goshareit` if you built with a different `BUNDLE_ID`.
