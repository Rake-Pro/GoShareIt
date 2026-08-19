//go:build darwin

package darwin

/*
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>

int gsi_confirm(const char *title, const char *body, const char *ok, const char *cancel, int giveUpSeconds);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Confirmer shows blocking native NSAlert dialogs (confirm.m).
type Confirmer struct{}

// NewConfirmer returns a macOS confirm-dialog provider.
func NewConfirmer() *Confirmer { return &Confirmer{} }

// Confirm shows a two-button alert on the app's main run loop and blocks until
// the user answers, Escape is hit, or the alert times out (120s) - the latter
// two are a normal "no" answer, not an error.
//
// The alert occupies the main thread while open (tray menu handling resumes
// when it closes), which is standard modal behavior and bounded by the timeout.
func (c *Confirmer) Confirm(title, body, okLabel, cancelLabel string) (bool, error) {
	ctitle, cbody := C.CString(title), C.CString(body)
	cok, ccancel := C.CString(okLabel), C.CString(cancelLabel)
	defer C.free(unsafe.Pointer(ctitle))
	defer C.free(unsafe.Pointer(cbody))
	defer C.free(unsafe.Pointer(cok))
	defer C.free(unsafe.Pointer(ccancel))
	switch rc := C.gsi_confirm(ctitle, cbody, cok, ccancel, 120); rc {
	case 1:
		return true, nil
	case 0:
		return false, nil // cancel button, Escape, or timeout
	case -2:
		return false, fmt.Errorf("darwin confirm: another dialog is already open")
	default:
		return false, fmt.Errorf("darwin confirm: cannot host the dialog (code %d)", int(rc))
	}
}
