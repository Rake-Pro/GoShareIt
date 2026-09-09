//go:build windows

package main

import "github.com/Rake-Pro/GoShareIt/platform/windows"

// isPackaged reports whether this is a Microsoft Store (MSIX) install, using
// the same helper the host uses so both binaries agree.
func isPackaged() bool { return windows.IsPackaged() }
