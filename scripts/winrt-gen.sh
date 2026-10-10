#!/bin/sh
# scripts/winrt-gen.sh - regenerate platform/windows/winocr/internal/winrt.
#
# Pinned to the winrt-go commit the 2026-10-10 probe used. The output is
# committed; the generator is NOT a module dependency (it runs with go run in
# a scratch directory). Review the diff before committing a regeneration.
#
# Known trait of the generated code: static wrappers call
# RoGetActivationFactory and never Release the factory. Factories are
# per-process singletons, so only their reference count grows (see the
# Engine doc comment in platform/windows/winocr/engine.go). Keep the output
# as generated; check whether a newer generator releases them before
# regenerating.
#
# Usage, from the repo root or via go generate in platform/windows/winocr:
#   sh scripts/winrt-gen.sh
set -eu

GEN=github.com/saltosystems/winrt-go/cmd/winrt-go-gen@v0.0.0-20260513072510-45f10383b2b8
SRC_MOD=github.com/saltosystems/winrt-go
ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=platform/windows/winocr/internal/winrt
PKG=github.com/Rake-Pro/GoShareIt/$OUT

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cd "$work"
run() { go run "$GEN" "$@"; }
run -class Windows.Media.Ocr.OcrEngine
run -class Windows.Media.Ocr.OcrResult
run -class Windows.Media.Ocr.OcrLine
run -class Windows.Media.Ocr.OcrWord
run -class Windows.Graphics.Imaging.SoftwareBitmap \
    -method-filter CreateCopyFromBuffer -method-filter Close \
    -method-filter PixelWidth -method-filter PixelHeight
run -class Windows.Graphics.Imaging.BitmapPixelFormat
run -class Windows.Graphics.Imaging.BitmapAlphaMode
run -class Windows.Globalization.Language -method-filter LanguageTag -method-filter DisplayName
run -class Windows.Security.Cryptography.CryptographicBuffer -method-filter CreateFromByteArray
run -class Windows.Foundation.Rect

# The generator writes windows/<namespace>/... relative to the working
# directory. Copy only the files the engine uses, plus the hand-written
# support packages they import (internal/delegate, internal/kernel32) from
# the generator's own module.
files="
media/ocr/ocrengine.go media/ocr/ocrline.go media/ocr/ocrresult.go media/ocr/ocrword.go
graphics/imaging/softwarebitmap.go graphics/imaging/bitmappixelformat.go graphics/imaging/bitmapalphamode.go
globalization/language.go
security/cryptography/cryptographicbuffer.go
foundation/rect.go foundation/iasyncinfo.go foundation/iasyncoperation.go foundation/asyncstatus.go
foundation/asyncoperationcompletedhandler.go foundation/iclosable.go foundation/hresult.go foundation/ireference.go
foundation/collections/ivectorview.go
storage/streams/ibuffer.go
"
rewrite() {
    sed -e "s#\"$SRC_MOD/windows/#\"$PKG/#" -e "s#\"$SRC_MOD/internal/#\"$PKG/internal/#" "$1" > "$2"
}
for f in $files; do
    mkdir -p "$ROOT/$OUT/$(dirname "$f")"
    rewrite "windows/$f" "$ROOT/$OUT/$f"
done

modsrc=$(go mod download -json "${GEN%%/cmd/*}@${GEN##*@}" | sed -n 's/.*"Dir": "\(.*\)".*/\1/p')
for d in delegate kernel32; do
    echo "support package internal/$d: review $modsrc/internal/$d against $OUT/internal/$d by hand;"
    echo "  the committed copies carry a provenance comment and (kernel32) one vet fix."
done
cp "$modsrc/LICENSE" "$ROOT/platform/windows/winocr/LICENSE.winrt-go"

cd "$ROOT"
gofmt -w "$OUT"
GOOS=windows CGO_ENABLED=0 go vet ./platform/windows/...
echo "regenerated $OUT; review git diff before committing"
