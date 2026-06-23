# GoShareIt

A ShareX-style screenshot capture + Nextcloud upload tool. Capture a region,
window, or full screen; the image is uploaded to Nextcloud, a public share link
is created, and the direct download URL is copied to your clipboard.

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

Copy `config.example.yaml` to `config.yaml` (gitignored) and edit. The config
path can be overridden with `GOSHAREIT_CONFIG_PATH`.

The Nextcloud app password is **never** stored inline. Set exactly one of:

- `nextcloud.password_file` - path to a `0600` file containing the password
  (read and whitespace-trimmed), or
- `nextcloud.password_env` - the name of an environment variable holding it.

Generate a Nextcloud app password under Settings -> Security -> Devices & sessions.

## Validated upload flow

1. **WebDAV PUT** to
   `{base_url}/remote.php/dav/files/{dav_user}/{remote_dir}/{name}` with HTTP
   Basic auth and `Content-Type: {mime}`. `dav_user` defaults to the part of
   `username` before `@` (e.g. `imgshare`). Success = 201 (200/204 accepted).
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

## Packaging & Release

Packaging, CI, and macOS signing/notarization tooling lives under `.github/`,
`build/macos/`, `scripts/`, and `docs/`. The macOS app ships as a signed,
notarized `.app` (menubar-only, `LSUIElement`).

- [docs/RELEASE.md](docs/RELEASE.md) - release runbook (cert + notary profile
  prerequisites, the `make release` flow, verification).
- [docs/PERMISSIONS.md](docs/PERMISSIONS.md) - the Screen Recording and
  Accessibility/Input Monitoring permissions the app needs and how to grant them.

### Make targets

| Target         | What it does                                            |
|----------------|---------------------------------------------------------|
| `test`         | build/test the pure-Go core (`CGO_ENABLED=0`)           |
| `vet`          | `go vet ./...`                                           |
| `fmt-check`    | fail if any file is not gofmt-clean                     |
| `build-darwin` | build the cgo binary for the host arch into `dist/`     |
| `bundle`       | assemble `dist/GoShareIt.app`                           |
| `sign`         | codesign with Hardened Runtime + entitlements           |
| `notarize`     | submit to Apple notary service and staple the ticket    |
| `release`      | `bundle` -> `sign` -> `notarize` -> staple              |
| `clean`        | remove `dist/`                                          |

Signing/notarization require `DEVELOPER_ID_APP`, `TEAM_ID`, `AC_NOTARY_PROFILE`,
and `BUNDLE_ID` (see docs/RELEASE.md).
