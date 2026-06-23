#!/usr/bin/env bash
# Build GoShareIt and assemble the macOS .app bundle in one shot, optionally
# signing and launching it. Intended for the local dev loop - wire it as a
# GoLand "Shell Script" run configuration for one-click build + run.
#
# Usage:
#   scripts/dev-build.sh [--open] [--no-sign]
#     --open      launch the bundle after building (relaunches if already running)
#     --no-sign   skip code signing even if DEVELOPER_ID_APP is set
#
# Env:
#   VERSION           bundle version            (default 0.0.0-dev)
#   BUNDLE_ID         CFBundleIdentifier        (default pro.rake.goshareit)
#   DEVELOPER_ID_APP  codesign identity (name or 40-char SHA-1 hash). If unset,
#                     signing is skipped: the app still bundles, but TCC grants
#                     (e.g. global hotkeys) will not persist across rebuilds.
set -euo pipefail

OPEN=0
SIGN=1
for arg in "$@"; do
	case "$arg" in
	--open | -o) OPEN=1 ;;
	--no-sign) SIGN=0 ;;
	*)
		echo "dev-build: unknown argument: $arg" >&2
		exit 2
		;;
	esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

VERSION="${VERSION:-0.0.0-dev}"
BUNDLE_ID="${BUNDLE_ID:-pro.rake.goshareit}"
DIST="dist"
BIN="$DIST/goshareit"
APP="$DIST/GoShareIt.app"
HOST_ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"

echo "==> building cgo binary (darwin/$HOST_ARCH)"
mkdir -p "$DIST"
CGO_ENABLED=1 GOOS=darwin GOARCH="$HOST_ARCH" go build -o "$BIN" ./cmd/goshareit

echo "==> assembling $APP"
VERSION="$VERSION" BUNDLE_ID="$BUNDLE_ID" BIN="$BIN" APP="$APP" "$SCRIPT_DIR/bundle.sh"

if [ "$SIGN" -eq 1 ]; then
	if [ -n "${DEVELOPER_ID_APP:-}" ]; then
		echo "==> signing"
		APP="$APP" "$SCRIPT_DIR/sign.sh"
	else
		echo "dev-build: DEVELOPER_ID_APP not set - skipping signing." >&2
		echo "dev-build: unsigned bundle; TCC grants will not persist across rebuilds." >&2
	fi
fi

echo "==> done: $APP (version $VERSION)"

if [ "$OPEN" -eq 1 ]; then
	echo "==> launching"
	pkill -f "$APP" 2>/dev/null || true
	open "$APP"
fi
