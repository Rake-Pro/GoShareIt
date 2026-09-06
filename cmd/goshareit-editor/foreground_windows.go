//go:build windows

package main

import (
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procSetForeground     = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop  = user32.NewProc("BringWindowToTop")
	procSetFocus          = user32.NewProc("SetFocus")
	procAttachThreadInput = user32.NewProc("AttachThreadInput")
	procShowWindow        = user32.NewProc("ShowWindow")
)

const swShow = 5

// bringToFront pushes this process's first visible top-level window to the
// foreground once it exists. Windows refuses SetForegroundWindow to a process
// the user did not interact with, and this helper is always started by the
// tray host in the background, so a plain show leaves the editor behind the
// current app and the user has to click it. Attaching to the foreground
// window's input thread is the documented way around that rule for a window
// the user actually asked for (a capture they just took).
func bringToFront() {
	go func() {
		for i := 0; i < 60; i++ { // ~3 s of polling for the window to appear
			if hwnd := ownWindow(); hwnd != 0 {
				forceForeground(hwnd)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

// ownWindow returns the first visible top-level window owned by this process.
func ownWindow() windows.HWND {
	var found windows.HWND
	pid := windows.GetCurrentProcessId()
	cb := syscall.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var wpid uint32
		windows.GetWindowThreadProcessId(hwnd, &wpid)
		if wpid == pid && windows.IsWindowVisible(hwnd) {
			found = hwnd
			return 0 // stop
		}
		return 1
	})
	_ = windows.EnumWindows(cb, nil)
	return found
}

func forceForeground(hwnd windows.HWND) {
	fg := windows.GetForegroundWindow()
	self := windows.GetCurrentThreadId()
	var fgThread uint32
	if fg != 0 && fg != hwnd {
		fgThread, _ = windows.GetWindowThreadProcessId(fg, nil)
	}
	attached := false
	if fgThread != 0 && fgThread != self {
		r, _, _ := procAttachThreadInput.Call(uintptr(fgThread), uintptr(self), 1)
		attached = r != 0
	}
	procShowWindow.Call(uintptr(hwnd), swShow)
	procBringWindowToTop.Call(uintptr(hwnd))
	procSetForeground.Call(uintptr(hwnd))
	procSetFocus.Call(uintptr(hwnd))
	if attached {
		procAttachThreadInput.Call(uintptr(fgThread), uintptr(self), 0)
	}
}
