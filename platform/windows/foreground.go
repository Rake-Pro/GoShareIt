//go:build windows

package windows

import (
	"syscall"

	syswin "golang.org/x/sys/windows"
)

var (
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procSetFocus            = user32.NewProc("SetFocus")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")
	procShowWindow          = user32.NewProc("ShowWindow")
)

// RaiseOwnWindow brings this process's first visible top-level window to the
// foreground. Windows refuses SetForegroundWindow to a process the user is
// not interacting with, so it attaches to the foreground window's input
// thread first: the same approach as the editor helper's forceForeground
// (cmd/goshareit-editor/foreground_windows.go). The settings helper uses it
// when the tray asks an already open window to come forward.
func RaiseOwnWindow() {
	var hwnd syswin.HWND
	pid := syswin.GetCurrentProcessId()
	cb := syscall.NewCallback(func(h syswin.HWND, _ uintptr) uintptr {
		var wpid uint32
		syswin.GetWindowThreadProcessId(h, &wpid)
		if wpid == pid && syswin.IsWindowVisible(h) {
			hwnd = h
			return 0 // stop
		}
		return 1
	})
	_ = syswin.EnumWindows(cb, nil)
	if hwnd == 0 {
		return
	}
	fg := syswin.GetForegroundWindow()
	self := syswin.GetCurrentThreadId()
	var fgThread uint32
	if fg != 0 && fg != hwnd {
		fgThread, _ = syswin.GetWindowThreadProcessId(fg, nil)
	}
	attached := false
	if fgThread != 0 && fgThread != self {
		r, _, _ := procAttachThreadInput.Call(uintptr(fgThread), uintptr(self), 1)
		attached = r != 0
	}
	const swShow = 5
	procShowWindow.Call(uintptr(hwnd), swShow)
	procBringWindowToTop.Call(uintptr(hwnd))
	procSetForegroundWindow.Call(uintptr(hwnd))
	procSetFocus.Call(uintptr(hwnd))
	if attached {
		procAttachThreadInput.Call(uintptr(fgThread), uintptr(self), 0)
	}
}
