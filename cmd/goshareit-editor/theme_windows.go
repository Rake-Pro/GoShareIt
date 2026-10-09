//go:build windows

package main

import "golang.org/x/sys/windows/registry"

// detectSystemDark reports whether Windows apps are using a dark theme by
// reading AppsUseLightTheme from the personalization registry key (0 =
// dark). A missing key or read error falls back to light, Windows' own
// default, the same fallback as macOS and Linux.
func detectSystemDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return false
	}
	return v == 0
}
