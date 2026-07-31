package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
	"github.com/Rake-Pro/GoShareIt/internal/core/update"
	"github.com/Rake-Pro/GoShareIt/internal/settings"
)

// settingsLauncher opens the out-of-process goshareit-settings UI (sibling
// binary, like the editor helper). The helper signals via settings.ExitSaved
// that the user actually hit Save; only then - and only if the config content
// really changed - does the host restart itself so every subsystem (hotkeys,
// uploader, tray labels) picks up the new config cleanly. Closing the window
// without saving discards and never restarts.
type settingsLauncher struct {
	configPath string
	app        *core.App
	quit       func()

	mu      sync.Mutex
	running bool
}

func (s *settingsLauncher) open(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	helper, err := s.resolveHelper()
	if err != nil {
		log.Error().Err(err).Msg("settings: helper not found")
		s.notify("Settings unavailable", "goshareit-settings binary not found next to the app")
		return
	}
	before, _ := os.ReadFile(s.configPath)

	saved, err := runSettingsHelper(exec.CommandContext(ctx, helper, "--config", s.configPath))
	if err != nil && ctx.Err() == nil {
		log.Error().Err(err).Msg("settings: helper failed")
		return
	}
	if ctx.Err() != nil {
		return
	}
	if !saved {
		return // closed without saving - discard, nothing to apply
	}
	// Saved, but byte-identical config (e.g. Save with no edits): a restart
	// would apply nothing, so skip it.
	if after, _ := os.ReadFile(s.configPath); bytes.Equal(before, after) {
		log.Debug().Msg("settings: saved without changes - no restart needed")
		return
	}

	log.Info().Msg("settings changed - restarting to apply")
	s.notify("Settings saved", "Restarting GoShareIt to apply changes.")
	path, err := update.SelfLaunchPath()
	if err != nil {
		log.Error().Err(err).Msg("settings: locate self for restart - restart manually")
		return
	}
	// Forward the exact config file that was edited: the relaunched process has
	// a different cwd and no memory of an original --config flag.
	absCfg, err := filepath.Abs(s.configPath)
	if err != nil {
		absCfg = s.configPath
	}
	if err := update.Relaunch(path, "--config", absCfg); err != nil {
		log.Error().Err(err).Msg("settings: relaunch failed - restart manually")
		return
	}
	s.quit()
}

func (s *settingsLauncher) resolveHelper() (string, error) { return resolveSettingsHelper() }

func resolveSettingsHelper() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "goshareit-settings"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

// runSettingsHelper runs the settings binary and reports whether the user
// saved at least once (signalled by settings.ExitSaved, which is not an
// error). A plain close exits 0 -> saved=false.
func runSettingsHelper(cmd *exec.Cmd) (saved bool, err error) {
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == settings.ExitSaved {
		return true, nil
	}
	return false, err
}

// runSettingsBlocking launches the settings UI and waits for it to close.
// Used at startup (first run or invalid config) as onboarding, before any
// tray exists - the alternative is a silent exit, which on windowsgui builds
// looks like the app simply not working.
func runSettingsBlocking(ctx context.Context, cfgPath string) error {
	helper, err := resolveSettingsHelper()
	if err != nil {
		return err
	}
	_, err = runSettingsHelper(exec.CommandContext(ctx, helper, "--config", cfgPath))
	return err
}

func (s *settingsLauncher) notify(title, body string) {
	if n := s.app.Notifier(); n != nil {
		if err := n.Notify(notify.Notification{Title: title, Body: body}); err != nil {
			log.Debug().Err(err).Msg("settings notification failed")
		}
	}
}
