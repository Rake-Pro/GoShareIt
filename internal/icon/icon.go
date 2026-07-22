// Package icon embeds the tray glyph assets. Regenerate with
// scripts/gen-tray-icon.py (capture-corner brackets + center dot).
package icon

import _ "embed"

// TrayDarwin is a black+alpha template PNG; macOS renders it adaptively for
// light/dark menu bars via systray.SetTemplateIcon.
//
//go:embed tray_darwin.png
var TrayDarwin []byte

// TrayWindows is a multi-size ICO (16/24/32/48), white glyph with a subtle
// outline so it reads on both taskbar themes.
//
//go:embed tray_windows.ico
var TrayWindows []byte
