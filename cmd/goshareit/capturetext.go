package main

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// textItemID is the tray menu item for Capture Text.
const textItemID = "text"

// ocrRecheck is how often an unavailable engine is probed again on Linux,
// where tesseract can be installed while the host runs.
const ocrRecheck = 30 * time.Second

// textItemState is the Capture Text tray item for an OCR status: the title
// (label carries the hotkey; a greyed item names the reason instead, since
// Windows tray menus show no tooltips), whether it is enabled, and the
// reason with its fix for a tooltip.
func textItemState(st ocr.Status, label string) (title string, enabled bool, why string) {
	if st.Available {
		return label, true, ""
	}
	reason := strings.TrimSuffix(st.Reason, ".")
	if reason == "" {
		reason = "not available"
	}
	return "Capture Text (" + reason + ")", false, st.Explain()
}

// textItemChecking is the Capture Text tray item before the first probe
// has answered: greyed out, saying so.
func textItemChecking() (title string, enabled bool, why string) {
	return "Capture Text (checking...)", false, "Checking whether text recognition is available."
}

// textNotification is what a finished Capture Text tells the user: the
// count copied, or that nothing was found. A cancel or a dropped press says
// nothing (ok false).
func textNotification(out core.TextOutcome) (title, body string, ok bool) {
	switch {
	case out.Cancelled || out.Busy:
		return "", "", false
	case out.Chars == 0:
		return "No text found", "No text was recognized in the selected area; the clipboard is unchanged.", true
	case out.Chars == 1:
		return "Text copied", "Copied 1 character to the clipboard.", true
	}
	return "Text copied", "Copied " + strconv.Itoa(out.Chars) + " characters to the clipboard.", true
}

// watchOCR probes text recognition once at start (when it is turned on),
// reports the result through apply and closes first. On Linux it then
// probes again every ocrRecheck while recognition is unavailable, so the
// tray item enables itself once tesseract is installed.
func watchOCR(ctx context.Context, app *core.App, apply func(ocr.Status), first chan<- struct{}) {
	if !app.Config().OCREnabled() {
		close(first)
		apply(app.TextStatus())
		return
	}
	st := app.ProbeOCR(ctx)
	close(first)
	apply(st)
	if st.Available || runtime.GOOS != "linux" {
		return
	}
	t := time.NewTicker(ocrRecheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		// An editor launch may already have re-probed and found it.
		prev := st
		if st = app.OCRStatus(); !st.Available {
			st = app.ProbeOCR(ctx)
		}
		if st.Available || st.Reason != prev.Reason {
			apply(st)
		}
		if st.Available {
			return
		}
	}
}
