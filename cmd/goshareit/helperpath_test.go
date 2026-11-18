package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelperInExeDir(t *testing.T) {
	appDir := t.TempDir()
	exe := filepath.Join(appDir, "goshareit")
	sibling := filepath.Join(appDir, "goshareit-editor")
	elsewhere := filepath.Join(t.TempDir(), "evil")
	link := filepath.Join(appDir, "link-to-evil")
	for _, p := range []string{exe, sibling, elsewhere} {
		if err := os.WriteFile(p, nil, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		sibling:                          true,
		elsewhere:                        false,
		link:                             false, // a symlink in the app folder pointing out of it
		filepath.Join(appDir, "missing"): false,
	}
	for p, want := range cases {
		if _, got := helperInExeDir(p, exe); got != want {
			t.Errorf("helperInExeDir(%s) = %v, want %v", p, got, want)
		}
	}
}

func TestTrustedHelperPathDevBuildKeepsOverride(t *testing.T) {
	if productionBuild {
		t.Skip("production build")
	}
	if got := trustedHelperPath("/some/dev/goshareit-editor"); got != "/some/dev/goshareit-editor" {
		t.Errorf("dev build changed helper_path to %q", got)
	}
}
