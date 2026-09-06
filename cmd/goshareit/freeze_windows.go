//go:build windows

package main

import "github.com/Rake-Pro/GoShareIt/platform/windows"

// regionFreeze supplies the frozen-screen backdrop for the record-region
// overlay on Windows.
var regionFreeze = windows.FreezePrimaryDisplay
