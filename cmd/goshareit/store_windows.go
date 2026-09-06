//go:build windows

package main

import "github.com/Rake-Pro/GoShareIt/platform/windows"

// distributedByStore is true for the Microsoft Store (MSIX) build, which the
// Store keeps updated itself.
func distributedByStore() bool { return windows.IsPackaged() }
