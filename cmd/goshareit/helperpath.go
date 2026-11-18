package main

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// trustedHelperPath vets editor.helper_path. The helper runs as a child of the
// host and so shares its OS permissions (on macOS the Screen Recording grant),
// so in release builds an override is honoured only when it points at a file
// next to the host binary; anything else falls back to the bundled sibling.
// Development builds (no "production" tag) keep the override as is.
func trustedHelperPath(p string) string {
	if p == "" || !productionBuild {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, ok := helperInExeDir(p, exe); ok {
			return resolved // exec the vetted file, not a link that can be repointed later
		}
	}
	log.Warn().Str("helper_path", p).Msg("editor.helper_path ignored: release builds only run helpers from the application's own folder")
	return ""
}

// helperInExeDir resolves p (symlinks, relative paths) and reports whether
// the result is a file directly inside the directory of exe.
func helperInExeDir(p, exe string) (string, bool) {
	exe, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", false
	}
	return abs, filepath.Dir(abs) == filepath.Dir(exe)
}
