package main

import (
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
)

func TestComposeConfirmLabel(t *testing.T) {
	disabled := false
	tests := []struct {
		name   string
		copy   bool
		save   bool
		upload *bool // nil -> UploadEnabled() default (true)
		want   string
	}{
		{"none", false, false, &disabled, "Done"},
		{"copy only", true, false, &disabled, "Copy"},
		{"save only", false, true, &disabled, "Save"},
		{"upload only", false, false, nil, "Upload"},
		{"copy and upload", true, false, nil, "Copy & Upload"},
		{"save and upload", false, true, nil, "Save & Upload"},
		{"copy save and upload", true, true, nil, "Copy, Save & Upload"},
		{"copy and save, no upload", true, true, &disabled, "Copy & Save"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.AfterCapture.CopyImageToClipboard = tt.copy
			cfg.AfterCapture.SaveLocal = tt.save
			cfg.Upload.Enabled = tt.upload
			if got := composeConfirmLabel(cfg); got != tt.want {
				t.Errorf("composeConfirmLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
