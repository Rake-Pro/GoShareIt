//go:build windows

package main

import (
	"github.com/Rake-Pro/GoShareIt/platform/wailsapp"
	"github.com/Rake-Pro/GoShareIt/platform/windows"
)

// isPackaged reports whether this is a Microsoft Store (MSIX) install, using
// the same helper the host uses so both binaries agree.
func isPackaged() bool { return windows.IsPackaged() }

// raiseWindow brings the open settings window to the foreground when the
// tray asks for it; Windows refuses a plain Focus from a background process.
func raiseWindow() { windows.RaiseOwnWindow() }

// checkChord validates one hotkey chord the way the host will bind it
// (PrintScreen chords through the direct RegisterHotKey path, the rest
// through the Wails shortcut manager) and returns its canonical form.
func checkChord(chord string) (string, error) {
	if windows.IsPrintScreenChord(chord) {
		return windows.ValidatePrintScreenChord(chord)
	}
	return wailsapp.ValidateChord(chord)
}
