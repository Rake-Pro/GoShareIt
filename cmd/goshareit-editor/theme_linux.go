//go:build linux && cgo

package main

import (
	"os/exec"
	"strings"

	"github.com/godbus/dbus/v5"
)

// detectSystemDark reports whether the desktop prefers a dark theme: first via
// the XDG settings portal (org.freedesktop.appearance color-scheme, honored by
// GNOME, KDE and most portal backends; 1 = prefer dark), then via gsettings
// for desktops without a portal. Anything unreadable falls back to dark.
func detectSystemDark() bool {
	if dark, ok := portalColorScheme(); ok {
		return dark
	}
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err != nil {
		return true
	}
	return strings.Contains(string(out), "dark")
}

func portalColorScheme() (dark, ok bool) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false, false
	}
	defer conn.Close()
	var v dbus.Variant
	err = conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop").
		Call("org.freedesktop.portal.Settings.Read", 0, "org.freedesktop.appearance", "color-scheme").
		Store(&v)
	if err != nil {
		return false, false
	}
	// Read wraps the value in a second variant.
	if inner, isVariant := v.Value().(dbus.Variant); isVariant {
		v = inner
	}
	n, isUint := v.Value().(uint32)
	if !isUint {
		return false, false
	}
	return n == 1, true
}
