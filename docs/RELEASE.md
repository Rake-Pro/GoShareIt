# Release Runbook

## Release path (CI, primary)

Releases are cut by merging the automated promotion PR, not by tagging or
running anything by hand:

1. `sync-prod.yml` opens a bot PR from `main` into `prod` whenever `main`
   moves.
2. A maintainer merges that PR into `prod` with a **merge commit** (squash/
   rebase merges do not trigger the release; `prod` is a protected branch
   requiring the `ci.yml` aggregate `build` check).
3. The merge triggers `.github/workflows/release.yml`, which:
   - mints the next semver tag from the highest existing `v*` tag (patch by
     default; a `release:minor` or `release:major` label on the merged PR
     bumps accordingly). Versioning restarted at `v0.0.1` after the 2026-07-22
     history squash; `1.0.0` is reserved for the public release.
   - builds and packages all three OS targets in the same run, from that tag:
     - **macOS**: universal (`lipo` arm64+amd64) `GoShareIt.app`, packaged as
       `GoShareIt_<ver>_darwin_universal.dmg` (human install) and
       `GoShareIt_<ver>_darwin_universal.zip` (the updater feed, whole `.app`).
     - **Windows**: `GoShareIt_<ver>_windows_amd64_setup.exe` (Inno Setup,
       per-user install, no admin) and `GoShareIt_<ver>_windows_amd64.zip`
       (the updater feed, loose exes).
     - **Linux**: `GoShareIt_<ver>_linux_amd64.tar.gz` - **EXPERIMENTAL**,
       host binary only; `wire_linux.go` still wires in-memory fakes, so
       there is no real capture backend.
   - writes `checksums.txt` (sha256 of every asset; the in-app updater fails
     closed if it is missing).
   - publishes everything to a GitHub Release on that tag. That release feed
     is exactly what `internal/core/update` polls; asset names must stay in
     sync with `update.AssetName()`.

### CI signing secrets (macOS)

Set as repository secrets; the macOS job checks for them and adapts:

| Secret | Purpose |
|---|---|
| `MACOS_CERT_P12` | base64-encoded `.p12` codesigning certificate |
| `MACOS_CERT_P12_PASSWORD` | password for the `.p12` |
| `DEVELOPER_ID_APP` | informational only for CI signing (see below); still read by local `scripts/sign.sh` |
| `AC_APPLE_ID` (optional) | Apple ID for notarization |
| `AC_PASSWORD` (optional) | app-specific password for notarization |
| `TEAM_ID` (optional) | 10-char Apple Team ID (bare, no parentheses), required with the above two |

- `MACOS_CERT_P12` + `MACOS_CERT_P12_PASSWORD` present -> the p12 is imported
  into the build keychain, followed by Apple's Developer ID CA intermediates
  (G1+G2) - a Keychain-exported p12 carries only the leaf cert + key, so
  without the intermediates the identity imports but is never valid on the
  bare runner keychain. The workflow then derives the identity's SHA-1 hash
  via `find-identity` and signs by that hash (not by the `DEVELOPER_ID_APP`
  name string - `codesign -s <name>` matches the exact certificate CN, and
  the build keychain only ever holds the one imported identity, so the hash
  is exact and immune to name/secret formatting drift). The bundle is signed
  with the Hardened Runtime + entitlements (`scripts/sign.sh`).
- `AC_APPLE_ID` + `AC_PASSWORD` + `TEAM_ID` also present -> the signed bundle
  is submitted to Apple's notary service and the ticket stapled.
- Signing secrets absent entirely -> the workflow falls back to ad-hoc signing
  the whole bundle (`codesign --force --deep -s -`) so it still has one
  consistent identity; the job stays green either way.

**Current state:** as of v0.0.8, the signing secrets carry a real Developer
ID Application certificate (team-anchored identity). Release builds are
codesigned and **notarized** - `notarytool` reports `Accepted` and the ticket
is stapled - so the bundle carries full Gatekeeper credit, and (being a
team-anchored identity) keeps Screen Recording/Accessibility TCC grants
persistent across installs, updates, and certificate renewals. Switching from
the old self-signed identity required a one-time full remove-and-re-add of
those TCC permissions on any existing install (a stale grant under the old
identity shadows the new one); see [PERMISSIONS.md](PERMISSIONS.md).
`DEVELOPER_ID_APP` is no longer load-bearing for CI signing (the identity is
resolved by hash); it is still used by `scripts/sign.sh` for local builds
(see below), where it must be the exact identity string.

### CI signing (Windows, SignPath Foundation)

Windows binaries are Authenticode-signed through [SignPath Foundation](https://signpath.org)'s
free open-source program. Without it Smart App Control on Windows 11 blocks
the unsigned exes ("Part of this app has been blocked") and SmartScreen
warns on first run.

| Setting | Kind | Purpose |
|---|---|---|
| `SIGNPATH_API_TOKEN` | secret | SignPath API token for the CI user (submit signing requests only). Absent -> Windows job ships unsigned and stays green. |
| `SIGNPATH_ORGANIZATION_ID` | variable | organization id from the SignPath UI |
| `SIGNPATH_PROJECT_SLUG` | variable (optional) | defaults to `GoShareIt` |

SignPath-side setup (one-time, in the SignPath UI):

1. Apply at <https://signpath.org/apply.html> (OSI license, public repo,
   2FA on GitHub and SignPath for every maintainer, builds from GitHub-hosted
   runners).
2. Project `GoShareIt` with the GitHub repository connected as its trusted
   build system (origin verification; keep `disallow_reruns`).
3. Two artifact configurations, pasted from `build/windows/signpath/`:
   `windows-binaries` (zip of the three exes) and `windows-installer` (the
   Inno Setup exe).
4. Signing policy `release-signing` with manual approval (the Foundation
   requires it for OSS projects).

Per release the Windows job submits two signing requests - first the loose
exes, then the installer built from the signed exes - and blocks until each
is approved in the SignPath UI (timeout 2 h; a lapsed request fails the job,
re-run it after approving). A `Verify signatures` step fails the job if any
shipped exe is not `Valid`. The signer shows as "SignPath Foundation", not
Rake-Pro; the private key never leaves SignPath's HSM, and the GitHub
connector only verifies that the artifact came from this repository's
workflow run.

## Local build (`make release`), secondary

For a manual local macOS build (testing signing, building outside CI):

### Prerequisites

1. **Apple Developer account** with a **Developer ID Application** certificate.
2. The certificate + private key installed in your login keychain. Verify:

   ```
   security find-identity -v -p codesigning
   ```

   Copy the full identity string (e.g. `Developer ID Application: Your Name (ABCDE12345)`).

3. A **notarytool keychain profile** holding your notarization credentials. Create
   it once with an app-specific password from https://appleid.apple.com:

   ```
   xcrun notarytool store-credentials "goshareit-notary" \
       --apple-id "you@example.com" \
       --team-id "ABCDE12345" \
       --password "app-specific-password"
   ```

### Required environment variables

| Variable            | Purpose                                                        |
|---------------------|----------------------------------------------------------------|
| `DEVELOPER_ID_APP`  | codesign identity, e.g. `Developer ID Application: Your Name (ABCDE12345)` |
| `TEAM_ID`           | 10-char Apple Team ID (used when creating the notary profile)  |
| `AC_NOTARY_PROFILE` | name of the notarytool keychain profile (e.g. `goshareit-notary`) |
| `BUNDLE_ID`         | bundle identifier (default `pro.rake.goshareit`)               |
| `VERSION`           | release version, passed to `make` (default `0.0.0-dev`)        |

Example:

```
export DEVELOPER_ID_APP="Developer ID Application: Your Name (ABCDE12345)"
export TEAM_ID="ABCDE12345"
export AC_NOTARY_PROFILE="goshareit-notary"
export BUNDLE_ID="pro.rake.goshareit"
```

### Release flow

```
make release VERSION=1.2.3
```

`release` runs, in order:

1. `bundle` - builds the cgo host, editor, and settings binaries for the host
   arch (`make build-darwin`) and assembles `dist/GoShareIt.app`
   (`scripts/bundle.sh`). This produces a **host-arch-only** bundle, unlike
   the CI build which lipos arm64+amd64 into a universal binary.
2. `sign` - codesigns with the Hardened Runtime + `build/macos/entitlements.plist`
   (`scripts/sign.sh`).
3. `notarize` - zips, submits to Apple with `notarytool --wait`, then
   `stapler staple`s the ticket (`scripts/notarize.sh`).

Individual steps are also available: `make bundle`, `make sign`, `make notarize`.

For an unsigned local dev loop instead, use `make dev-run`: it auto-discovers a
local dev signing identity so TCC grants persist across rebuilds, without
needing a real Developer ID cert.

### Verify the result

```
codesign --verify --deep --strict --verbose=2 dist/GoShareIt.app
spctl --assess --type execute --verbose=4 dist/GoShareIt.app   # -> accepted
xcrun stapler validate dist/GoShareIt.app                       # -> validated
```

A successful release shows `accepted` (source=Notarized Developer ID) from
`spctl` and `The validate action worked!` from `stapler`.

### Notes

- For a universal binary, build both arches with `lipo` before bundling (this
  is what the CI macOS job does); the local Makefile ships a host-arch binary
  only.
- Runtime permissions are separate from signing; see [PERMISSIONS.md](PERMISSIONS.md).
