//go:build windows

package windows

import (
	"os/exec"
	"syscall"

	syswin "golang.org/x/sys/windows"
)

// noConsole keeps a console-subsystem child (powershell.exe, ffmpeg.exe) from
// opening its own console window. The host is a windowsgui binary with no
// console of its own, so without this every toast, dialog and recording
// flashes a black console on screen. -WindowStyle Hidden alone is not enough:
// PowerShell creates the window before it reads that switch.
func noConsole(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: syswin.CREATE_NO_WINDOW}
	return cmd
}
