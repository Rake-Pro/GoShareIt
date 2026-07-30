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

# Settings UI helper (Wails), same sibling-binary pattern. Optional.
SETTINGS_BIN="${SETTINGS_BIN:-dist/goshareit-settings}"
if [ -f "$SETTINGS_BIN" ]; then
	cp "$SETTINGS_BIN" "$MACOS_DIR/goshareit-settings"
	chmod +x "$MACOS_DIR/goshareit-settings"
	echo "  + bundled settings helper: goshareit-settings"
else
	echo "  ! settings helper not found ($SETTINGS_BIN); Settings menu will be unavailable" >&2
fi

# Info.plist with placeholders substituted. Use a temp file then move.
sed -e "s|__VERSION__|$VERSION|g" \
    -e "s|__BUNDLE_ID__|$BUNDLE_ID|g" \
    "$PLIST_SRC" > "$CONTENTS/Info.plist"

# App icon: build/icons/goshareit_icon.png (1024x1024 source) is rendered into
# an .iconset and compiled to AppIcon.icns via sips/iconutil (macOS-only tools;
# bundle.sh only ever runs on macOS - dev-build.sh cross-compiles with cgo, and
# release.yml's macos job is the only CI caller). Referenced by CFBundleIconFile
# in Info.plist. The app also shows in the Finder/Dock icon picker even though
# LSUIElement hides the Dock icon at runtime (e.g. Finder, "About", Force Quit).
ICON_SRC="${ICON_SRC:-$REPO_ROOT/build/icons/goshareit_icon.png}"
if [ -f "$ICON_SRC" ]; then
	if command -v sips >/dev/null 2>&1 && command -v iconutil >/dev/null 2>&1; then
		ICONSET_TMP="$(mktemp -d)"
		ICONSET="$ICONSET_TMP/AppIcon.iconset"
		mkdir -p "$ICONSET"
		for size in 16 32 128 256 512; do
			sips -z "$size" "$size" "$ICON_SRC" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
			double=$((size * 2))
			sips -z "$double" "$double" "$ICON_SRC" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
		done
		iconutil -c icns "$ICONSET" -o "$RES_DIR/AppIcon.icns"
		rm -rf "$ICONSET_TMP"
		echo "  + generated app icon: AppIcon.icns"
	else
		echo "  ! sips/iconutil not found; app icon will be unavailable" >&2
	fi
else
	echo "  ! icon source not found ($ICON_SRC); app icon will be unavailable" >&2
fi

echo "bundled $APP (version $VERSION, id $BUNDLE_ID)"
