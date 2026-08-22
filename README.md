# GoShareIt

A cross-platform screenshot and screen-recording tool for macOS and Windows.
Capture a region, window, or full screen; optionally annotate it (crop, arrow,
text, blur, and more) in a light/dark/system-themed editor; upload to
Nextcloud (default), S3-compatible storage, SFTP, WebDAV, or a custom HTTP
endpoint (with imgur/catbox/0x0.st presets), with a public
share link or direct URL copied to your clipboard - or run entirely in
local-only mode with uploads off. Ships as a self-updating menu-bar/tray app.

## Binaries and first run

GoShareIt ships as three sibling binaries:

- `goshareit` - the menu-bar/tray host (always running; owns hotkeys, capture,
  upload).
- `goshareit-editor` - the out-of-process annotation editor and region
  selector overlay, launched by the host as needed.
- `goshareit-settings` - the settings UI (Wails), launched from the tray
  ("Settings...") or automatically on first run.

All app state (config, secrets, logs, history) lives in one per-user root:
`~/.goshareit` on macOS/Linux, `%USERPROFILE%\goshareit` on Windows. On an
unconfigured install the host opens the settings UI instead of exiting, so
first run is: install, launch, fill in (or skip) Nextcloud details in the
settings window, save. Nothing is required to get started - local-only mode
(below) works with zero configuration.

## Architecture: pure-Go core + thin OS shells

The core (`internal/core/...`) is **pure Go**. It builds and tests on any
platform with `CGO_ENABLED=0`, never uses cgo, never shells out, and never
imports a `platform/` package. All OS-specific behavior is expressed as
interface seams that the core depends on:

| Seam | Package | Responsibility |
|------|---------|----------------|
| `Capturer` | `internal/core/capture` | screen/region/window capture |
| `Uploader` | `internal/core/upload` | upload + share (Nextcloud impl is portable) |
| `Clipboard` | `internal/core/clipboard` | read/write text + images |
| `Notifier` | `internal/core/notify` | desktop notifications |
| `Tray` | `internal/core/tray` | menu-bar / system-tray |
| `hotkey.Manager` | `internal/core/hotkey` | global hotkeys |

Concrete OS implementations live under `platform/darwin` and `platform/windows`
(added by later phases) and are injected through `core.Providers` by per-GOOS
`cmd/goshareit/wire_<goos>.go` files. `main.go` is OS-agnostic and calls
`buildProviders(cfg)`. The committed `wire_linux.go` returns in-memory fakes so
the module builds and runs on linux for CI.

The orchestration pipeline (`internal/core/pipeline.go`):

```
capture -> after-capture (save local / copy image) -> name -> upload ->
after-upload (copy URL to clipboard, notify, append history)
```

## Build

```
CGO_ENABLED=0 go build ./...
go test ./...
```

The macOS app is built on macOS (`GOOS=darwin`), Windows on Windows. The linux
build is for the portable core and CI only; it has no real capture backend.

## Configuration

The easiest path is the **settings UI** (`goshareit-settings`, opened from the
tray or automatically on first run): every option is editable there, including
"Sign in with browser" (Nextcloud Login Flow v2, OIDC/SSO-compatible) which
sets up the server credentials without ever typing a password into a text
field. Saving restarts the host automatically.

For manual/headless setup, edit the YAML directly: copy `config.example.yaml`
to `config.yaml` in the app root (`~/.goshareit` on macOS/Linux,
`%USERPROFILE%\goshareit` on Windows) and edit. The config path can be
overridden with `GOSHAREIT_CONFIG_PATH`.

The Nextcloud app password is **never** stored inline. Set exactly one of:

- `nextcloud.password_file` - path to a `0600` file containing the password
  (read and whitespace-trimmed), or
- `nextcloud.password_env` - the name of an environment variable holding it.

Generate a Nextcloud app password under Settings -> Security -> Devices & sessions.

Set `upload.enabled: false` (or toggle "Upload captures" in settings, or the
tray "Uploads: On/Off" item, or the `upload_toggle` hotkey) for **local-only
mode**: nothing leaves the machine and the whole Nextcloud section becomes
optional. Captures still save locally / copy to clipboard / notify per your
after-capture settings.

## Validated upload flow

Applies when `upload.enabled: true` (the default; off = local-only mode, see
above).

1. **WebDAV PUT** to
   `{base_url}/remote.php/dav/files/{dav_user}/{remote_dir}/{name}` with HTTP
   Basic auth and `Content-Type: {mime}`. `dav_user` defaults to the part of
   `username` before `@` (e.g. `uploads`). Success = 201 (200/204 accepted).
2. **OCS share POST** to
   `{base_url}/ocs/v2.php/apps/files_sharing/api/v1/shares` with headers
   `OCS-APIRequest: true` and `Accept: application/json`, form body
   `path=/{remote_dir}/{name}`, `shareType=3`, `permissions=1` (plus optional
   `expireDate` and `password`). The token is read from `.ocs.data.token` and
   `.ocs.meta.statuscode` must be `200`.
3. **Links built** from the token:
   - `DirectURL = {base_url}/s/{token}/download` - raw bytes, copied to clipboard
   - `PublicURL = {base_url}/s/{token}` - viewer page, stored in history
   - `ShareToken = token`

## Releases and self-update

Releases are cut by CI (merge to `prod` mints the next semver tag and builds
all three platforms in one run - see [docs/RELEASE.md](docs/RELEASE.md) for
the full flow): a macOS universal `.dmg`/`.zip`, a Windows Inno Setup
installer + `.zip`, and an experimental Linux `.tar.gz`, plus a
`checksums.txt`. The macOS `.app` is codesigned with a Developer ID
certificate and notarized in CI (menubar-only, `LSUIElement`); see
docs/RELEASE.md for the signing flow.

The app self-updates: `goshareit` polls the GitHub Releases API anonymously
on an interval, and the tray "Check for Updates" item lets you check and
install on demand. No credentials or configuration are needed for updates.

- [docs/RELEASE.md](docs/RELEASE.md) - the CI release path, signing secrets,
  and the local `make release` runbook.
- [docs/PERMISSIONS.md](docs/PERMISSIONS.md) - the Screen Recording and
  Accessibility/Input Monitoring permissions the app needs and how to grant them.

### Make targets

| Target         | What it does                                                     |
|----------------|-------------------------------------------------------------------|
| `test`         | build/test the pure-Go core (`CGO_ENABLED=0`)                    |
| `vet`          | `go vet ./...`                                                    |
| `fmt-check`    | fail if any file is not gofmt-clean                               |
| `build-darwin` | build the cgo host, editor, and settings binaries for the host arch into `dist/` |
| `bundle`       | assemble `dist/GoShareIt.app` from those binaries                 |
| `sign`         | codesign with Hardened Runtime + entitlements                     |
| `notarize`     | submit to Apple notary service and staple the ticket              |
| `release`      | `bundle` -> `sign` -> `notarize` -> staple                        |
| `dev` / `dev-run` | local dev loop: build -> bundle -> sign with a local identity (`dev-run` also launches it) |
| `clean`        | remove `dist/`                                                    |

Signing/notarization require `DEVELOPER_ID_APP`, `TEAM_ID`, `AC_NOTARY_PROFILE`,
and `BUNDLE_ID` (see docs/RELEASE.md).

## Privacy

GoShareIt collects no user data and has no telemetry. See [PRIVACY.md](PRIVACY.md).

## License

MIT - see [LICENSE](LICENSE).
