// Package icon embeds the tray icon assets. Regenerate with
// scripts/gen-tray-icon.py.
package icon

import _ "embed"

// TrayDarwin is the black+alpha TEMPLATE glyph (capture corners + dot);
// macOS renders templates black/white adaptively, where the simple glyph
// reads better than a silhouette of the full logo.
//
//go:embed tray_darwin.png
var TrayDarwin []byte

// TrayWindows is the product logo (shutter aperture + G mark, master at
// build/icons/goshareit_icon.png) as a full-color multi-size ICO
// (16/24/32/48/64).
//
//go:embed tray_windows.ico
var TrayWindows []byte
