package main

import (
	"strings"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
)

// composeConfirmLabel builds the editor confirm button's label from the
// after-capture pipeline so the button always says what will happen on
// click. Enabled parts are collected in order Copy, Save, Upload and joined:
// one part as-is, two as "A & B", three as "A, B & C"; none enabled yields
// "Done".
func composeConfirmLabel(cfg *config.Config) string {
	var parts []string
	if cfg.AfterCapture.CopyImageToClipboard {
		parts = append(parts, "Copy")
	}
	if cfg.AfterCapture.SaveLocal {
		parts = append(parts, "Save")
	}
	if cfg.UploadEnabled() {
		parts = append(parts, "Upload")
	}
	switch len(parts) {
	case 0:
		return "Done"
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " & " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " & " + parts[len(parts)-1]
	}
}
