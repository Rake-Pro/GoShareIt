//go:build windows

package windows

import (
	"errors"
	"os/exec"
	"syscall"
	"unsafe"

	syswin "golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// MSStoreURL is where the dialogs send users for the Store build. Search until
// the listing exists; swap for "ms-windows-store://pdp/?ProductId=<id>" once
// Partner Center assigns one.
const MSStoreURL = "ms-windows-store://search/?query=GoShareIt"

const appmodelErrorNoPackage syscall.Errno = 15700

// IsPackaged reports whether this process runs from an MSIX package (the
// Microsoft Store build). Store installs are updated by the Store and run with
// Smart App Control on, so the self-updater and the SAC warning both stand down.
func IsPackaged() bool {
	proc := syswin.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentPackageFullName")
	if err := proc.Find(); err != nil {
		return false // pre-1809 Windows: no packaging API, so not packaged
	}
	var n uint32
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&n)), 0)
	return syscall.Errno(r) != appmodelErrorNoPackage
}

// OpenMSStore opens the Microsoft Store at MSStoreURL.
func OpenMSStore() error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", MSStoreURL).Start()
}

// Smart App Control (Windows 11 22H2+, clean installs only) states as stored
// in HKLM\SYSTEM\CurrentControlSet\Control\CI\Policy\VerifiedAndReputablePolicyState.
const (
	SACOff        = 0
	SACOn         = 1
	SACEvaluation = 2
)

// SmartAppControlState reads the Smart App Control mode. ok is false when the
// value is absent (Windows 10, or Windows 11 upgraded in place), which means
// the feature is not present and nothing needs to be said about it.
func SmartAppControlState() (state int, ok bool, err error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\CI\Policy`, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("VerifiedAndReputablePolicyState")
	if errors.Is(err, registry.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return int(v), true, nil
}

// OpenAppBrowserControl opens Windows Security on the "App & browser control"
// page, which holds the Smart App Control settings link.
func OpenAppBrowserControl() error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", "windowsdefender://appbrowser").Start()
}
