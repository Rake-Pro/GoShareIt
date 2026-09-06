//go:build darwin

package main

// bringToFront is a no-op outside Windows: `open`-launched and Gio-created
// windows on macOS come to the front on their own.
func bringToFront() {}
