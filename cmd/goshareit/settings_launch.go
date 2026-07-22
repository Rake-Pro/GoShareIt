package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
	"github.com/Rake-Pro/GoShareIt/internal/core/update"
)

// settingsLauncher opens the out-of-process goshareit-settings UI (sibling
// binary, like the editor helper). When the settings process exits and the
// config file changed, the host restarts itself so every subsystem (hotkeys,
// uploader, tray labels) picks up the new config cleanly.
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
	before := mtime(s.configPath)

	cmd := exec.CommandContext(ctx, helper, "--config", s.configPath)
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		log.Error().Err(err).Msg("settings: helper failed")
		return
	}
	if ctx.Err() != nil {
		return
	}
	if after := mtime(s.configPath); !after.After(before) {
		return // nothing saved
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

// runSettingsBlocking launches the settings UI and waits for it to close.
// Used at startup (first run or invalid config) as onboarding, before any
// tray exists - the alternative is a silent exit, which on windowsgui builds
// looks like the app simply not working.
func runSettingsBlocking(ctx context.Context, cfgPath string) error {
	helper, err := resolveSettingsHelper()
	if err != nil {
		return err
	}
	return exec.CommandContext(ctx, helper, "--config", cfgPath).Run()
}

func (s *settingsLauncher) notify(title, body string) {
	if n := s.app.Notifier(); n != nil {
		if err := n.Notify(notify.Notification{Title: title, Body: body}); err != nil {
			log.Debug().Err(err).Msg("settings notification failed")
		}
	}
}

func mtime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
