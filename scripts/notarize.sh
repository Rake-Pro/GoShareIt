#!/usr/bin/env bash
# Notarize dist/GoShareIt.app and staple the ticket.
#
# Inputs (env):
#   AC_NOTARY_PROFILE  required. Name of the notarytool keychain profile.
#   APP                .app bundle path (default dist/GoShareIt.app)
#
# One-time setup: create the keychain profile so this script never sees raw
# credentials. Generate an app-specific password at https://appleid.apple.com,
# then run:
#
#   xcrun notarytool store-credentials "$AC_NOTARY_PROFILE" \
#       --apple-id "you@example.com" \
#       --team-id "$TEAM_ID" \
#       --password "app-specific-password"
#
# (Alternatively use --key/--key-id/--issuer for an App Store Connect API key.)
set -euo pipefail

APP="${APP:-dist/GoShareIt.app}"

if [ -z "${AC_NOTARY_PROFILE:-}" ]; then
	echo "notarize: AC_NOTARY_PROFILE is not set" >&2
	exit 1
fi
if [ ! -d "$APP" ]; then
	echo "notarize: bundle not found: $APP (run 'make bundle && make sign' first)" >&2
	exit 1
fi

# notarytool requires a zip/dmg/pkg container, not a raw .app directory.
ZIP="${APP%.app}.zip"
rm -f "$ZIP"
# ditto preserves the bundle structure and symlinks for notarization.
/usr/bin/ditto -c -k --keepParent "$APP" "$ZIP"

# Submit and block until Apple returns a verdict; non-zero exit on rejection.
xcrun notarytool submit "$ZIP" \
	--keychain-profile "$AC_NOTARY_PROFILE" \
	--wait

# Staple the ticket onto the .app so it validates offline.
xcrun stapler staple "$APP"

# Confirm the staple and Gatekeeper now accept it.
xcrun stapler validate "$APP"
spctl --assess --type execute --verbose=4 "$APP"

rm -f "$ZIP"
echo "notarized + stapled $APP"
