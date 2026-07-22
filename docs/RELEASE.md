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
| `DEVELOPER_ID_APP` | codesign identity string, e.g. `Developer ID Application: Your Name (ABCDE12345)` |
| `AC_APPLE_ID` (optional) | Apple ID for notarization |
| `AC_PASSWORD` (optional) | app-specific password for notarization |
| `TEAM_ID` (optional) | 10-char Apple Team ID, required with the above two |

- `MACOS_CERT_P12` + `MACOS_CERT_P12_PASSWORD` + `DEVELOPER_ID_APP` present ->
  the bundle is codesigned with the Hardened Runtime + entitlements
  (`scripts/sign.sh`).
- `AC_APPLE_ID` + `AC_PASSWORD` + `TEAM_ID` also present -> the signed bundle
  is submitted to Apple's notary service and the ticket stapled.
- Signing secrets absent entirely -> the workflow falls back to ad-hoc signing
  the whole bundle (`codesign --force --deep -s -`) so it still has one
  consistent identity; the job stays green either way.

**Current state:** the signing secrets are populated with an interim
self-signed `RakePro-Dev` identity (`TeamIdentifier` not set). Release builds
are codesigned with a stable identity, which is enough for Screen
Recording/Accessibility TCC grants to persist across installs and updates on
a personal machine, but the bundle carries **no Gatekeeper credit** (not
notarized, not a real Developer ID). Replacing it with a real Developer ID
Application certificate, and wiring up notarization once that cert is in
place, are tracked in [BACKLOG.md](../BACKLOG.md) - do not duplicate that
item here.

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

For an unsigned local dev loop instead, use `make dev-run` (see
[docs/STATUS.md](STATUS.md#build--run-quick-reference)): it auto-discovers a
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
