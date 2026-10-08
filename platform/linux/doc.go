//go:build linux

// Package linux provides the Linux platform backends for the GoShareIt core
// seams: still capture, ffmpeg screen recording and the clipboard. The tray,
// global hotkeys, notifications and confirm dialogs come from platform/wailsapp
// like on the other desktops.
//
// SESSION TYPES: an X11 session is served directly (xinerama display bounds,
// XGetImage captures through kbinani/screenshot, the active window through
// _NET_ACTIVE_WINDOW, ffmpeg x11grab for video). A Wayland session has no
// client-side screen access, so stills go through the XDG desktop portal's
// Screenshot interface (org.freedesktop.portal.Screenshot: non-interactive
// for whole-screen grabs, the compositor's own picker for window captures)
// and video recording is not offered (that would need the ScreenCast portal
// and a PipeWire consumer, see BACKLOG.md).
//
// Everything here is pure Go (xgb, godbus, x/clipboard), so the package builds
// and its tests run in the CGO-off core job; cgo is only needed by the Wails
// and Gio halves of the app.
package linux
