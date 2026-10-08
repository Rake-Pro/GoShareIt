//go:build linux && cgo

package main

import "github.com/Rake-Pro/GoShareIt/platform/linux"

// regionFreeze supplies the frozen-screen backdrop for the record-region
// overlay on Linux (the Gio overlay is opaque there too).
var regionFreeze = linux.FreezePrimaryDisplay
