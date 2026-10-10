# GoShareIt

A cross-platform screenshot and screen-recording tool for macOS and Windows,
with a Linux build in beta.
Capture a region, window, or full screen; optionally annotate it (crop, arrow,
text, blur, and more) in a light/dark/system-themed editor, or copy the text
in a region straight to the clipboard (on-device text recognition); upload to
Nextcloud (default), S3-compatible storage, SFTP, WebDAV, or a custom HTTP
endpoint (with imgur/catbox/0x0.st presets), with a public
share link or direct URL copied to your clipboard, or run entirely in
local-only mode with uploads off. Ships as a self-updating menu-bar/tray app.

## Binaries and first run

GoShareIt ships as three sibling binaries:

- `goshareit`: the menu-bar/tray host (always running; owns hotkeys, capture,
  upload).
- `goshareit-editor`: the out-of-process annotation editor and region
  selector overlay, launched by the host as needed.
- `goshareit-settings`: the settings UI (Wails), launched from the tray
  ("Settings...") or automatically on first run.

The region selector covers the primary display only; on a multi-monitor
setup, capture another display with full-screen capture.

All app state (config, secrets, logs, history) lives in one per-user root:
`~/.goshareit` on macOS/Linux, `%USERPROFILE%\goshareit` on Windows. First
run is: install, launch, and the settings window opens. A new install starts
in local-only mode (uploads off), so nothing is required to get started:
close the window and capture, or set up an upload destination, turn on
"Upload captures" and save. If an existing config cannot be loaded, the
settings window opens too, and if the upload setup is still incomplete
after it closes, the app runs in local-only mode instead of exiting.

### Windows says "Part of this app has been blocked"

That is Smart App Control (Windows 11, clean installs only). The GitHub
download of GoShareIt is not code-signed: signing certificates cost money
the project does not pay, and the free open-source signing programs did not
accept a project this size. Smart App Control has no per-app exception, so
you have two options:

- Turn Smart App Control off: Windows Security > App & browser control >
  Smart App Control settings > Off. The installer and the app offer to open
  that page. Windows lets you turn it back on later without a reinstall on
  current builds.
- Install the Microsoft Store build instead (uninstall the GitHub copy
  first). The Store signs it with Microsoft's certificate, so it runs with
  Smart App Control on, and the Store delivers its updates.

| Smart App Control state | GitHub download | Microsoft Store build |
|---|---|---|
| Off, or not present (Windows 10, upgraded Windows 11) | Runs. SmartScreen may warn once: "More info", then "Run anyway". | Runs. |
| Evaluation | Runs. Installer and first launch warn once and offer both options. | Runs. |
| On | Installer and exes are blocked before they start. | Runs. |

Both builds share the same config root and behave the same; the Store build
only turns off the built-in updater. The macOS build is signed and notarized.

### Linux (beta)

The Linux build is new and has not yet been validated on real desktops; it
builds and is tested in CI, and the release tarball is marked beta until the
on-device list in BACKLOG.md is green. It is a `.tar.gz` of the same three
binaries. Install:

```
tar -xzf GoShareIt_<ver>_linux_amd64.tar.gz -C ~/.local/bin
curl -fsSLo ~/.local/share/applications/goshareit.desktop https://raw.githubusercontent.com/Rake-Pro/GoShareIt/main/build/linux/goshareit.desktop
curl -fsSLo ~/.icons/goshareit.png https://raw.githubusercontent.com/Rake-Pro/GoShareIt/main/build/icons/goshareit_icon.png
```

The desktop entry and icon are optional (the tray is the app's UI); a
package with them built in is on the backlog.

Keep the three binaries next to each other: the host finds the editor and
settings helpers as siblings, and the in-app updater replaces them in place.

Runtime packages (Debian/Ubuntu names; the host and settings UI are GTK 3 +
WebKitGTK apps): `libgtk-3-0`, `libwebkit2gtk-4.1-0`, `xdg-desktop-portal`
plus your desktop's portal backend (`xdg-desktop-portal-gnome`, `-kde`,
`-wlr`, ...), `ffmpeg` for video recording, and optionally `tesseract-ocr`
plus a language pack (`tesseract-ocr-eng`) for text recognition in the
editor (see [Text recognition](#text-recognition-ocr)).

| | X11 session | Wayland session |
|---|---|---|
| Region, full screen, last region | direct from the X server | XDG Screenshot portal (whole desktop, then cropped by the app's own overlay) |
| Window capture | the focused window (`_NET_ACTIVE_WINDOW`, frame included) | the compositor's own picker (pick a window there) |
| Video / GIF recording | ffmpeg `x11grab` / frame sampling | not offered (needs the ScreenCast portal, see BACKLOG.md) |
| Global hotkeys | `XGrabKey` | GlobalShortcuts portal (GNOME 45+, KDE Plasma 5.27+); the desktop may show a binding dialog once |
| Tray | StatusNotifierItem (GNOME needs the AppIndicator extension) | same |

`PrintScreen` cannot be bound on Linux (no name for it in the shortcut
backend); the default region chord is `Ctrl+Shift+1`. Notifications go
through D-Bus (`org.freedesktop.Notifications`), the clipboard through the
Wayland data-control protocol or X11 selections, and "Start at login" writes
`~/.config/autostart/goshareit.desktop`.

## Architecture: pure-Go core + thin OS shells

The core (`internal/core/...`) is **pure Go**. It builds and tests on any
platform with `CGO_ENABLED=0`, never uses cgo, and never imports a `platform/`
package. The only processes it spawns are its own sibling helper binaries
(editor, region overlay), the platform updater (`ditto`/`open` on macOS) and,
on Linux, the optional `tesseract` command for text recognition.
All other OS-specific behavior is expressed as interface seams that the core
depends on:

| Seam | Package | Responsibility |
|------|---------|----------------|
| `Capturer` / `Recorder` / `RegionRecorder` | `internal/core/capture` | screen/region/window capture, video/GIF recording |
| `Uploader` | `internal/core/upload` | upload + share (Nextcloud impl is portable) |
| `Clipboard` | `internal/core/clipboard` | read/write text + images |
| `Notifier` / `Confirmer` | `internal/core/notify` | desktop notifications, blocking confirm dialogs |
| `Tray` | `internal/core/tray` | menu-bar / system-tray |
| `hotkey.Manager` | `internal/core/hotkey` | global hotkeys |
| `ocr.Engine` | `internal/core/ocr` | on-device text recognition; `internal/core/ocr/engines` picks the platform engine |

Concrete OS implementations live under `platform/`:

| Package | Builds on | Backs |
|---------|-----------|-------|
| `platform/wailsapp` | darwin + windows + linux (cgo) | tray, global hotkeys, notifications, confirm dialogs (one Wails v3 application, one main loop) |
| `platform/darwin` | darwin (cgo) | screen capture, AVFoundation recording, clipboard, Screen Recording TCC preflight; `platform/darwin/vision`: Apple Vision text recognition |
| `platform/windows` | windows | screen capture, ffmpeg recording, clipboard, PrintScreen hotkey chords + registry tweak, Smart App Control notice; `platform/windows/winocr`: Windows.Media.Ocr text recognition (CGO off) |
| `platform/linux` | linux (pure Go) | screen capture (X11 direct / XDG Screenshot portal on Wayland), ffmpeg `x11grab` recording, clipboard; text recognition is the portable `internal/core/ocr/tesseract` engine |

`platform/windows/winocr/internal/winrt` is generated WinRT binding code
(winrt-go-gen output from github.com/saltosystems/winrt-go, MIT, notice in
`platform/windows/winocr/LICENSE.winrt-go`) committed in-repo, the same
pattern go-toast uses for Windows notifications; the generator is not a
module dependency. Regenerate with `sh scripts/winrt-gen.sh` (pinned
generator commit) and review the diff.

They are injected through `core.Providers` by per-GOOS
`cmd/goshareit/wire_<goos>.go` files. `main.go` is OS-agnostic and calls
`buildProviders(cfg)`. A CGO-off linux build (`wire_linux_nocgo.go`) returns
in-memory fakes so the portable core still builds and runs for CI.

The tray owns the process main loop: `Tray.Run` calls the Wails `app.Run()` on
the main goroutine (macOS pins AppKit to the first thread), and the hotkey,
notification and dialog seams marshal onto that same loop.

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

The macOS app is built on macOS (`GOOS=darwin`), Windows on Windows, Linux on
Linux with cgo (`make build-linux`; the CGO-off linux build above is the
portable core with fake desktop seams, for CI).

The host and settings binaries ship with `-tags production` (the Wails release
variant, plus `gtk3` on Linux: GTK 3 + WebKitGTK 4.1 rather than Wails'
default GTK 4 + WebKitGTK 6.0); the editor has no build tag. macOS and Linux
need cgo, Windows does not. Linux dev packages (Debian/Ubuntu):
`libgtk-3-dev libwebkit2gtk-4.1-dev libegl1-mesa-dev libgles2-mesa-dev
libwayland-dev libxkbcommon-dev libxkbcommon-x11-dev libx11-dev
libx11-xcb-dev libxcursor-dev libxfixes-dev libvulkan-dev libffi-dev`.

## Configuration

The easiest path is the **settings UI** (`goshareit-settings`, opened from the
tray or automatically on first run): every everyday option is editable there
(a few advanced ones, such as `editor.timeout_seconds`, `editor.helper_path`,
`update.repo`, secret file paths and the `*_env` secret sources, are
YAML-only), including "Sign in with browser" (Nextcloud Login Flow v2,
OIDC/SSO-compatible) which
sets up the server credentials without ever typing a password into a text
field. Saving restarts the host automatically.

For manual/headless setup, edit the YAML directly: copy `config.example.yaml`
to `config.yaml` in the app root (`~/.goshareit` on macOS/Linux,
`%USERPROFILE%\goshareit` on Windows) and edit. The config path can be
overridden with `GOSHAREIT_CONFIG_PATH`.

The Nextcloud app password is **never** stored inline. Set exactly one of:

- `nextcloud.password_file`: path to a `0600` file containing the password
  (read and whitespace-trimmed), or
- `nextcloud.password_env`: the name of an environment variable holding it.

Generate a Nextcloud app password under Settings -> Security -> Devices & sessions.

Set `upload.enabled: false` (or toggle "Upload captures" in settings, or the
tray "Uploads: On/Off" item, or the `upload_toggle` hotkey) for **local-only
mode**: nothing leaves the machine and the whole Nextcloud section becomes
optional. Captures still save locally / copy to clipboard / notify per your
after-capture settings.

### Public image, GIF and file hosts

No server of your own? Pick a host straight from the Destination list in
the settings UI (grouped under "Public hosts"), paste its API key where one
is needed, Save. In YAML that is `upload.destination: imgur` and the key in
`~/.goshareit/host-imgur.secret` (see `config.example.yaml`). Anything not
listed works too if it takes an HTTP upload: the "Custom HTTP" destination
is a generic uploader (method, URL, headers, multipart or raw body, JSON
path, regex or template extraction of the link) and can start from any of
the same presets.

| Preset | Accepts | Key | Notes |
|---|---|---|---|
| Imgur | images, GIFs, video | Client ID (free app registration) | deletion link kept in history; set file field to `video` for MP4 |
| ImgBB | images, GIFs (32 MB) | API key | deletion link kept in history |
| Imgchest | images, GIFs | API key | uploads become posts on your account |
| Lensdump | images, GIFs | API key | Chevereto host |
| Chevereto (any host) | images, GIFs | API key | edit the host in the URL |
| Catbox | any file (200 MB) | none (optional userhash) | permanent |
| Litterbox | any file (1 GB) | none | temporary, 1 h to 72 h |
| Uguu | any file (128 MB) | none | temporary, 3 h |
| 0x0.st | any file (512 MB) | none | 30 to 365 days by size |
| Pixeldrain | any file (20 GB) | API key | account retention rules apply |
| Gofile | any file | none (optional token) | guest uploads expire when unused |
| GIPHY | GIFs, short video (100 MB) | API key | lands in your GIPHY account |

Not available: Tenor has no public upload API, and Gfycat shut down.
Deletion links, when a host returns one, are written to `history.jsonl`
(`delete_url`) and logged at upload time.

## Editor

The post-capture editor opens after a capture when `editor.enabled` is on
(or from the `*_edit` hotkeys and "Capture Region and Edit").

| Toolbar part | Holds |
|---|---|
| Tool buttons | Select (always first), the drawing tools from `editor.tools`, Redact (always last); then the colour swatches |
| Controls | stroke `-` / `+` with the width, zoom readout, the text field (Text tool), Quick redact (Redact tool), Undo, Redo, Copy text, Redact selection (Select tool) |
| Actions | Copy, Save, Upload (greyed out while uploads are off), Cancel and the confirm button (its label follows your after-capture settings, e.g. "Copy & Upload") |
| Hint row | what the active tool does, why a greyed-out button is unavailable, what was just copied or redacted |

- Nothing scrolls or is cut off: the tool buttons and swatches wrap onto a
  second row when the window is too narrow, and the actions move to a row
  of their own, right-aligned, with Cancel and the confirm button always
  visible (down to the 640 dp minimum window width).
- Tool buttons show their key ("Arrow (A)") only when all of them fit on
  one row that way; otherwise the hint row names the active tool's key.
- **Select (`V`)** works like Live Text on macOS: over recognized text the
  cursor is an I-beam, a drag that starts on text selects it in reading
  order, a double-click selects a line, Ctrl/Cmd+C copies the selection
  (the editor stays open) and Ctrl/Cmd+A selects all of it. A drag that
  starts on a placed annotation moves the annotation (it wins over text
  under it); a click elsewhere deselects.
- **Copy text** copies the selected text, or all recognized text when
  nothing is selected, and the hint row says how many characters were
  copied. It reads "Reading text..." while recognition runs and is greyed
  out with the reason when recognition is unavailable. When recognition
  finishes, the hint row says how many lines of text it found.
- Words with a quarter or more of their box under a Redact box are never selected or
  copied; undoing or deleting the box brings them back.
- **Redact (`X`)** is always in the toolbar: drag an opaque box (current
  colour, black recommended). With Redact active, Quick redact hides the
  emails and phone numbers it found (`ocr.quick_redact`) in one undo step.
  In Select, Redact selection covers the selected text.
- `editor.tools` lists only the drawing tools; `select_text` and `redact`
  from older configs are accepted and ignored.

| Key (not while typing annotation text) | Does |
|---|---|
| `V` `C` `A` `R` `E` `T` `B` `P` `H` `N` `L` `F` `X` | Select, Crop, Arrow, Rectangle, Ellipse, Text, Blur, Pixelate, Highlight, Step, Line, Freehand, Redact |
| Select: click a shape, drag it | selects and moves a placed annotation (thin strokes have a few pixels of slack) |
| Select: drag over text / double-click a word | selects text in reading order / selects the line |
| Ctrl/Cmd+C | copies the selected text (the editor stays open); with no text selection it copies the image and closes, as before |
| Ctrl/Cmd+Shift+C | copies all recognized text |
| Ctrl/Cmd+A | selects all recognized text (Select) |
| Ctrl/Cmd+Shift+R | Quick redact (one undo step) |
| Delete / Backspace | removes the selected shape |
| Arrow keys / Shift+arrow keys | nudge the selected shape 1 px / 10 px (a run of nudges is one undo step) |
| Esc | abandons a drag in progress, then clears a text selection, then deselects, then cancels as before |
| Crop (`C`) handles | drag a corner or edge to resize, inside to move, outside for a new crop; Enter confirms the editor |
| `[` / `]` | stroke width -1 / +1 (also restyles the selected shape) |
| `1` to `7` | colour swatch (also recolours the selected shape) |
| Ctrl/Cmd+Z / Ctrl/Cmd+Shift+Z or Ctrl/Cmd+Y | undo / redo, including moves, deletes, restyles and crop changes |
| Ctrl/Cmd+0 / Ctrl/Cmd+1 | zoom to fit / 100% (the zoom readout in the toolbar toggles the same) |
| Ctrl/Cmd+= / Ctrl/Cmd+- | zoom in / out |

## Text recognition (OCR)

The editor can find text in a capture: select and copy it with the Select
tool, copy all of it with Copy text, or cover it with an opaque Redact box.
Recognition runs on this computer only (see [PRIVACY.md](PRIVACY.md)); when
no engine is available Copy text and Quick redact are greyed out with the
reason and the fix.

| Platform | Engine | Requirement |
|---|---|---|
| macOS 13+ | Apple Vision (on-device) | none |
| Windows 10/11 | Windows OCR (`Windows.Media.Ocr`) | an OCR language pack: Settings > Time & language > Language & region > your language > Language options > Optical character recognition; restart GoShareIt after installing |
| Linux (beta) | the `tesseract` command, run locally over stdin/stdout | `sudo apt install tesseract-ocr tesseract-ocr-eng` (Debian/Ubuntu) or `sudo dnf install tesseract tesseract-langpack-eng` (Fedora); Tesseract 4.0 or newer; picked up on the next capture, no restart |

- Settings > Text recognition shows the engine, its languages, or why it is
  not available, and holds the `ocr:` options (see `config.example.yaml`).
- Recognition starts in the background when the editor opens
  (`ocr.auto_run`); drawing, zooming and the action buttons never wait for it.
  With it off, recognition starts when the editor opens in Select, on
  switching to Select, or on the first Copy text.
- Quick redact hides the emails and phone numbers it found (configurable:
  `ocr.quick_redact`). It can miss text; check the result. Blur and Pixelate
  are not safe for text; use Redact.
- **Capture Text** (tray item, hotkey `hotkeys.text`): pick a region and its
  text goes straight to the clipboard, no editor; a notification says how
  many characters were copied. The tray item is greyed out with the reason
  while recognition is unavailable (on Linux it enables itself within 30 s
  of installing tesseract). Nothing is saved, uploaded or added to the
  history.

| Hotkey (`hotkeys.text`) | macOS | Windows | Linux |
|---|---|---|---|
| Capture Text (new installs; existing configs: set it in Settings > Hotkeys) | Cmd+Shift+8 | Ctrl+Shift+8 | Ctrl+Shift+8 |

Text selection, Copy text and the redaction tools are described under
[Editor](#editor).

## Validated upload flow

Applies when `upload.enabled: true` (the default when the key is absent; new
installs start with it off until a destination is set up; off = local-only
mode, see above).

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
   - `DirectURL`, copied to clipboard: `{base_url}/s/{token}/preview` for
     `image/png` and `image/jpeg` (a GIF's `/preview` is a single static
     frame, so it must not use this), otherwise `{base_url}/s/{token}/download`
     for raw bytes (GIFs, video, everything else)
   - `PublicURL = {base_url}/s/{token}`: viewer page, stored in history
   - `ShareToken = token`

## Releases and self-update

Releases are cut by CI (merge to `main` mints the next semver tag and builds
all three platforms in one run; see [docs/RELEASE.md](docs/RELEASE.md) for
the full flow): a macOS universal `.dmg`/`.zip`, a Windows Inno Setup
installer + `.zip`, and a Linux `.tar.gz` (beta), plus a
`checksums.txt`. The macOS `.app` is codesigned with a Developer ID
certificate and notarized in CI (menubar-only, `LSUIElement`); see
docs/RELEASE.md for the signing flow.

The app self-updates: `goshareit` polls the GitHub Releases API anonymously
shortly after launch and then on an interval, and installs a newer release
as soon as it finds one (set `update.auto_install: false` to be notified
instead). The tray "Check for Updates" item lets you check and install on
demand. Installing hands off to a small progress window (`goshareit-editor
--update`): the host quits, the release downloads and verifies, files are
swapped, and the new host starts again. Minor and major updates first show
a what's-new window with the release notes since your version (Update now
/ Later; `update.show_changelog: false` turns it off); patch updates go
straight through. No credentials or configuration are needed for updates. Microsoft Store installs are updated by the Store
instead.

- [docs/RELEASE.md](docs/RELEASE.md): the CI release path, signing secrets,
  and the local `make release` runbook.
- [docs/PERMISSIONS.md](docs/PERMISSIONS.md): the Screen Recording and
  Notifications permissions the app needs on macOS and how to grant them.

### Make targets

| Target         | What it does                                                     |
|----------------|-------------------------------------------------------------------|
| `test`         | build/test the pure-Go core (`CGO_ENABLED=0`)                    |
| `vet`          | `go vet ./...`                                                    |
| `fmt-check`    | fail if any file is not gofmt-clean                               |
| `build-darwin` | build the cgo host, editor, and settings binaries for the host arch into `dist/` |
| `build-linux`  | build the cgo host, editor, and settings binaries for linux into `dist/` |
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

MIT. See [LICENSE](LICENSE).
