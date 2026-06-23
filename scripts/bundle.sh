#!/usr/bin/env bash
# Assemble dist/GoShareIt.app from a pre-built binary and Info.plist.
#
# Inputs (env, with defaults):
#   VERSION    marketing/build version written into Info.plist   (default 0.0.0-dev)
#   BUNDLE_ID  CFBundleIdentifier                                 (default pro.rake.goshareit)
#   BIN        path to the built goshareit binary                (default dist/goshareit)
#   APP        output .app bundle path                           (default dist/GoShareIt.app)
#
# Produces:
#   $APP/Contents/MacOS/goshareit
#   $APP/Contents/Info.plist        (placeholders substituted)
#   $APP/Contents/Resources/        (icon goes here; see note below)
set -euo pipefail

VERSION="${VERSION:-0.0.0-dev}"
BUNDLE_ID="${BUNDLE_ID:-pro.rake.goshareit}"
BIN="${BIN:-dist/goshareit}"
APP="${APP:-dist/GoShareIt.app}"

# Resolve repo root from this script's location so the run dir does not matter.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLIST_SRC="$REPO_ROOT/build/macos/Info.plist"

if [ ! -f "$BIN" ]; then
	echo "bundle: binary not found: $BIN (run 'make build-darwin' first)" >&2
	exit 1
fi
if [ ! -f "$PLIST_SRC" ]; then
	echo "bundle: missing $PLIST_SRC" >&2
	exit 1
fi

CONTENTS="$APP/Contents"
MACOS_DIR="$CONTENTS/MacOS"
RES_DIR="$CONTENTS/Resources"

# Start clean so stale files never linger in the bundle.
rm -rf "$APP"
mkdir -p "$MACOS_DIR" "$RES_DIR"

# Host binary.
cp "$BIN" "$MACOS_DIR/goshareit"
chmod +x "$MACOS_DIR/goshareit"

# Editor helper binary, placed beside the host so the launcher finds it via
# os.Executable()'s directory. Optional: skipped if not built.
EDITOR_BIN="${EDITOR_BIN:-dist/goshareit-editor}"
if [ -f "$EDITOR_BIN" ]; then
	cp "$EDITOR_BIN" "$MACOS_DIR/goshareit-editor"
	chmod +x "$MACOS_DIR/goshareit-editor"
	echo "  + bundled editor helper: goshareit-editor"
else
	echo "  ! editor helper not found ($EDITOR_BIN); annotation editor will be unavailable" >&2
fi

# Info.plist with placeholders substituted. Use a temp file then move.
sed -e "s|__VERSION__|$VERSION|g" \
    -e "s|__BUNDLE_ID__|$BUNDLE_ID|g" \
    "$PLIST_SRC" > "$CONTENTS/Info.plist"

# Icon placeholder. Drop an AppIcon.icns into build/macos/ and uncomment the copy
# below (and add a CFBundleIconFile=AppIcon key to Info.plist) to ship an icon.
# A menubar (LSUIElement) app has no Dock icon, so this is optional.
#   cp "$REPO_ROOT/build/macos/AppIcon.icns" "$RES_DIR/AppIcon.icns"
: > "$RES_DIR/.keep"

echo "bundled $APP (version $VERSION, id $BUNDLE_ID)"
