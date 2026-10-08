//go:build darwin || (linux && cgo)

package main

// isPackaged is always false outside Windows: there is no MSIX equivalent, and
// no bundle owns the login item (the host registers it itself).
func isPackaged() bool { return false }
