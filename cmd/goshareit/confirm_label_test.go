package main

import (
	"errors"
	"strings"
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
			if got := composeConfirmLabel(cfg, cfg.UploadEnabled()); got != tt.want {
				t.Errorf("composeConfirmLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFriendlyError(t *testing.T) {
	for _, tc := range []struct {
		err         error
		title, body string
	}{
		{errors.New("upload: Catbox needs your userhash: open Settings > Upload and enter it"), "Upload failed", "Catbox needs your userhash: open Settings > Upload and enter it"},
		{errors.New("capture: screen recording permission denied"), "Capture failed", "Screen recording permission denied"},
		{errors.New("editor: the capture was discarded: editor: run helper: signal: killed"), "Capture discarded", ""},
		{errors.New("stop recording: ffmpeg exited"), "Recording failed", "Ffmpeg exited"},
	} {
		title, body := friendlyError(tc.err)
		if title != tc.title || (tc.body != "" && body != tc.body) || body == "" {
			t.Errorf("friendlyError(%q) = %q, %q; want %q, %q", tc.err, title, body, tc.title, tc.body)
		}
	}
	long := errors.New("upload: " + strings.Repeat("x", 500))
	if _, body := friendlyError(long); len(body) > 260 || !strings.Contains(body, "goshareit.log") {
		t.Errorf("long error body not shortened: %d chars", len(body))
	}
}
