#!/usr/bin/env bash
# Codesign dist/GoShareIt.app with the Hardened Runtime + entitlements.
#
# Inputs (env):
#   DEVELOPER_ID_APP  required. Codesign identity, e.g.
#                     "Developer ID Application: Your Name (ABCDE12345)"
#   APP               .app bundle path (default dist/GoShareIt.app)
set -euo pipefail

APP="${APP:-dist/GoShareIt.app}"

if [ -z "${DEVELOPER_ID_APP:-}" ]; then
	echo "sign: DEVELOPER_ID_APP is not set" >&2
	exit 1
fi
if [ ! -d "$APP" ]; then
	echo "sign: bundle not found: $APP (run 'make bundle' first)" >&2
	exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ENTITLEMENTS="$REPO_ROOT/build/macos/entitlements.plist"

if [ ! -f "$ENTITLEMENTS" ]; then
	echo "sign: missing $ENTITLEMENTS" >&2
	exit 1
fi

# Sign the bundle. --options runtime enables the Hardened Runtime (required for
# notarization). --timestamp obtains a secure timestamp. --deep signs nested
# code; for a single-binary app it just covers the one Mach-O.
codesign --force --deep \
	--options runtime \
	--timestamp \
	--entitlements "$ENTITLEMENTS" \
	--sign "$DEVELOPER_ID_APP" \
	"$APP"

# Verify the signature is valid and strict.
codesign --verify --deep --strict --verbose=2 "$APP"

# Gatekeeper assessment. Pre-notarization this prints "rejected" for the notary
# requirement but confirms the signature is otherwise sound; it passes after the
# staple step. Do not fail the build on it here.
spctl --assess --type execute --verbose=4 "$APP" || \
	echo "sign: spctl not yet accepted (expected until notarized+stapled)"

echo "signed $APP with: $DEVELOPER_ID_APP"
