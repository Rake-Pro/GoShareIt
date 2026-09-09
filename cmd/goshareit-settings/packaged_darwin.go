//go:build darwin

package main

// isPackaged is always false on macOS: there is no MSIX equivalent, and the
// .app bundle does not own the login item (the host registers it itself).
func isPackaged() bool { return false }
