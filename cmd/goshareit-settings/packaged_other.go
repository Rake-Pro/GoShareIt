//go:build darwin || (linux && cgo)

package main

import "github.com/Rake-Pro/GoShareIt/platform/wailsapp"

// isPackaged is always false outside Windows: there is no MSIX equivalent, and
// no bundle owns the login item (the host registers it itself).
func isPackaged() bool { return false }

// raiseWindow is a no-op outside Windows, where Focus brings the window
// forward on its own.
func raiseWindow() {}

// checkChord validates one hotkey chord the way the host's Wails shortcut
// manager will bind it and returns its canonical accelerator.
func checkChord(chord string) (string, error) {
	return wailsapp.ValidateChord(chord)
}
