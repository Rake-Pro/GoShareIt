//go:build windows

package windows

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// FreePrintScreen turns off Windows 11's "Use the Print screen key to open
// screen capture" accessibility setting (HKCU, no elevation needed). While it
// is on, Snipping Tool owns VK_SNAPSHOT and RegisterHotKey fails for every
// chord built on PrintScreen. Returns changed=true when the value was flipped;
// a missing key (Windows 10) is not an error. Windows may only pick up the
// change at the next sign-in.
func FreePrintScreen() (changed bool, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\Keyboard`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("open HKCU\\Control Panel\\Keyboard: %w", err)
	}
	defer k.Close()
	cur, _, err := k.GetIntegerValue("PrintScreenKeyForSnippingEnabled")
	if errors.Is(err, registry.ErrNotExist) || (err == nil && cur == 0) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read PrintScreenKeyForSnippingEnabled: %w", err)
	}
	if err := k.SetDWordValue("PrintScreenKeyForSnippingEnabled", 0); err != nil {
		return false, fmt.Errorf("set PrintScreenKeyForSnippingEnabled: %w", err)
	}
	return true, nil
}
