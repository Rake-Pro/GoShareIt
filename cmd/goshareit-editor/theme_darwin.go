//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

// detectSystemDark reports whether macOS is currently in Dark Mode by
// reading the AppleInterfaceStyle global default. The key only exists in
// Dark Mode, so a non-zero exit (key absent) or any output other than
// "Dark" means light.
func detectSystemDark() bool {
	out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "Dark"
}
