package main

import (
	"runtime"

	"github.com/Rake-Pro/GoShareIt/internal/icon"
)

// trayIcon returns the platform-appropriate tray icon bytes (nil on platforms
// without a real tray, where the fake ignores it anyway).
func trayIcon() []byte {
	switch runtime.GOOS {
	case "darwin":
		return icon.TrayDarwin
	case "windows":
		return icon.TrayWindows
	}
	return nil
}
