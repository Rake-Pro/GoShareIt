# Release Runbook (macOS)

Build, sign, notarize and staple a distributable `GoShareIt.app`. Run on macOS
with Xcode command line tools installed.

## Prerequisites

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

## Required environment variables

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

## Release flow

```
make release VERSION=1.2.3
```

`release` runs, in order:

1. `bundle` - builds the cgo binary (`make build-darwin`) and assembles
   `dist/GoShareIt.app` (`scripts/bundle.sh`).
2. `sign` - codesigns with the Hardened Runtime + `build/macos/entitlements.plist`
   (`scripts/sign.sh`).
3. `notarize` - zips, submits to Apple with `notarytool --wait`, then
   `stapler staple`s the ticket (`scripts/notarize.sh`).

Individual steps are also available: `make bundle`, `make sign`, `make notarize`.

## Verify the result

```
codesign --verify --deep --strict --verbose=2 dist/GoShareIt.app
spctl --assess --type execute --verbose=4 dist/GoShareIt.app   # -> accepted
xcrun stapler validate dist/GoShareIt.app                       # -> validated
```

A successful release shows `accepted` (source=Notarized Developer ID) from
`spctl` and `The validate action worked!` from `stapler`.

## Notes

- CI (`.github/workflows/ci.yml`) builds the cgo app for arm64 and amd64 to prove
  it compiles, but does **not** sign by default. Signing only runs when the
  signing secrets are present.
- For a universal binary, build both arches with `lipo` before bundling; the
  current Makefile ships a host-arch binary.
- Runtime permissions are separate from signing; see [PERMISSIONS.md](PERMISSIONS.md).
