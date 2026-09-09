package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
	"github.com/Rake-Pro/GoShareIt/internal/core/update"
)

// updateController owns the "Check for Updates" tray item: periodic background
// checks, and click-to-check / click-to-install depending on state.
type updateController struct {
	upd         *update.Updater
	app         *core.App
	interval    time.Duration
	autoInstall bool
	quit        func()
	// handoff, when non-nil, runs the download/install/relaunch in a separate
	// process with its own progress window (goshareit-editor --update) after
	// this host quits. nil = in-process install (linux, or no helper found).
	handoff *update.Job
	helper  string
	// changelog gates a what's-new window before minor/major updates.
	changelog bool

	mu      sync.Mutex
	pending *update.Release
	busy    bool
}

const updateItemID = "update"

func newUpdateController(upd *update.Updater, app *core.App, interval time.Duration, autoInstall bool, quit func()) *updateController {
	return &updateController{upd: upd, app: app, interval: interval, autoInstall: autoInstall, quit: quit}
}

// enableHandoff makes install() delegate to the out-of-process updater
// (helper = goshareit-editor path; "" resolves the sibling binary). job
// carries what the updater needs besides the release (repo, relaunch path,
// args, theme); HostPID and Version are filled at install time.
func (c *updateController) enableHandoff(helper string, job update.Job) {
	if helper == "" {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		name := "goshareit-editor"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		helper = filepath.Join(filepath.Dir(exe), name)
	}
	if _, err := os.Stat(helper); err != nil {
		log.Debug().Str("helper", helper).Msg("update: no editor helper, installs stay in-process")
		return
	}
	c.helper = helper
	c.handoff = &job
}

func (c *updateController) menuItem(ctx context.Context) tray.MenuItem {
	return tray.MenuItem{
		ID:      updateItemID,
		Title:   "Check for Updates",
		OnClick: func() { go c.onClick(ctx) },
	}
}

// start runs the launch check (~30s in, so the tray and network are up) and
// the periodic background check. Dev builds never auto-check (a 0.0.0-dev
// binary would otherwise immediately "upgrade" to the last release).
func (c *updateController) start(ctx context.Context) {
	if c.upd.IsDev() {
		log.Debug().Msg("update: dev build, background checks disabled")
		return
	}
	go func() {
		startup := time.NewTimer(30 * time.Second)
		defer startup.Stop()
		select {
		case <-startup.C:
			c.check(ctx, false)
		case <-ctx.Done():
			return
		}
		tick := time.NewTicker(c.interval)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				c.check(ctx, false)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (c *updateController) onClick(ctx context.Context) {
	c.mu.Lock()
	if c.busy {
		c.mu.Unlock()
		return
	}
	pending := c.pending
	c.mu.Unlock()
	if pending != nil {
		c.install(ctx, pending)
		return
	}
	c.check(ctx, true)
}

// check queries the feed. Manual checks report "up to date" and errors via
// notification; background checks only surface a found update. It holds the
// busy flag across its whole span - confirm dialog included - so a second tray
// click or a background tick can neither run a parallel check nor take the
// pending-install path underneath a still-open dialog.
func (c *updateController) check(ctx context.Context, manual bool) {
	c.mu.Lock()
	if c.busy {
		c.mu.Unlock()
		return
	}
	c.busy = true
	c.mu.Unlock()
	rel := c.doCheck(ctx, manual)
	c.mu.Lock()
	c.busy = false
	c.mu.Unlock()
	// install() takes busy itself, so it must run after check releases it.
	if rel != nil {
		c.install(ctx, rel)
	}
}

// doCheck runs the feed query + user interaction and returns a non-nil release
// only when the user confirmed installing it now.
func (c *updateController) doCheck(ctx context.Context, manual bool) *update.Release {
	rel, err := c.upd.Check(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("update check failed")
		if manual {
			c.notify("Update check failed", err.Error())
		}
		return nil
	}
	if rel == nil {
		log.Debug().Msg("update: up to date")
		if manual {
			c.notify("Up to date", "You are running the latest version.")
		}
		return nil
	}
	c.mu.Lock()
	c.pending = rel
	c.mu.Unlock()
	c.setTitle("Install Update v" + rel.Version)
	log.Info().Str("version", rel.Version).Msg("update available")

	// Background checks (launch + periodic) install straight away when
	// auto-install is on, unless a recording is in progress (the relaunch
	// would kill it); the recording case falls through to the quiet path and
	// the next tick or a tray click picks it up. Manual clicks get a native
	// confirm dialog instead of the quiet notify+retitle fallback; background
	// checks with auto-install off always stay quiet. The pending state and
	// tray title above are set first regardless, so the tray-menu install path
	// still works if the dialog errors.
	if !manual && c.autoInstall {
		if c.app.Recording() {
			log.Info().Str("version", rel.Version).Msg("update: recording active, deferring auto-install")
		} else {
			if c.handoff == nil {
				c.notify("Updating GoShareIt", "Installing v"+rel.Version+" and restarting.")
			}
			return rel
		}
	}
	//
	// check() always runs off the tray's main loop: menuItem's OnClick wraps
	// onClick in `go`, and the periodic path in start() runs inside its own
	// goroutine. The Confirmer itself dispatches the modal onto the app's main
	// thread, so tray handling pauses only while the dialog is actually open
	// (bounded by its timeout) - standard modal behavior.
	if manual {
		if confirmer := c.app.Confirmer(); confirmer != nil {
			ok, err := confirmer.Confirm(
				"Update available",
				"GoShareIt v"+rel.Version+" is ready to install. Update now?",
				"Update Now", "Later",
			)
			if err != nil {
				log.Debug().Err(err).Msg("update confirm dialog failed")
				return nil
			}
			if ok {
				return rel
			}
			return nil
		}
	}
	c.notify("Update available", "GoShareIt v"+rel.Version+" is ready - use the tray menu to install.")
	return nil
}

func (c *updateController) install(ctx context.Context, rel *update.Release) {
	c.mu.Lock()
	if c.busy {
		c.mu.Unlock()
		return
	}
	c.busy = true
	c.mu.Unlock()
	c.setEnabled(false)
	c.setTitle("Installing Update v" + rel.Version + "...")
	defer func() {
		c.mu.Lock()
		c.busy = false
		c.mu.Unlock()
		c.setEnabled(true)
	}()

	if c.handoff != nil {
		job := *c.handoff
		job.Version = rel.Version
		job.HostPID = os.Getpid()
		path, err := update.WriteJob(job)
		if err != nil {
			log.Error().Err(err).Msg("update handoff failed")
			c.notify("Update failed", err.Error())
			c.setTitle("Install Update v" + rel.Version)
			return
		}
		if c.changelog && update.MinorBump(job.Current, rel.Version) {
			// What's-new window first; the host keeps running while it is up.
			// Exit 64 = Later: keep the pending install on the tray item.
			var exitErr *exec.ExitError
			err := exec.Command(c.helper, "--changelog", path).Run()
			switch {
			case err == nil:
			case errors.As(err, &exitErr) && exitErr.ExitCode() == 64:
				os.Remove(path)
				log.Info().Str("version", rel.Version).Msg("update deferred by user")
				c.setTitle("Install Update v" + rel.Version)
				return
			default:
				// Informational only: a broken window must not block the update.
				log.Warn().Err(err).Msg("changelog window failed; continuing with the update")
			}
		}
		cmd := exec.Command(c.helper, "--update", path)
		if err := cmd.Start(); err != nil {
			os.Remove(path)
			log.Error().Err(err).Msg("update handoff: start updater")
			c.notify("Update failed", err.Error())
			c.setTitle("Install Update v" + rel.Version)
			return
		}
		_ = cmd.Process.Release()
		log.Info().Str("version", rel.Version).Msg("update handed off to updater; quitting")
		c.quit()
		return
	}

	archive, err := c.upd.Download(ctx, rel)
	if err != nil {
		log.Error().Err(err).Msg("update download failed")
		c.notify("Update failed", err.Error())
		c.setTitle("Install Update v" + rel.Version)
		return
	}
	relaunch, err := update.Apply(archive)
	if err != nil {
		log.Error().Err(err).Msg("update apply failed")
		c.notify("Update failed", err.Error())
		c.setTitle("Install Update v" + rel.Version)
		return
	}
	c.notify("Update installed", "Restarting as v"+rel.Version+".")
	if err := update.Relaunch(relaunch); err != nil {
		log.Error().Err(err).Msg("update relaunch failed - start the app manually")
	}
	c.quit()
}

func (c *updateController) setTitle(title string) {
	if tr := c.app.Tray(); tr != nil {
		tr.SetItemTitle(updateItemID, title)
	}
}

func (c *updateController) setEnabled(enabled bool) {
	if tr := c.app.Tray(); tr != nil {
		tr.SetItemEnabled(updateItemID, enabled)
	}
}

func (c *updateController) notify(title, body string) {
	if n := c.app.Notifier(); n != nil {
		if err := n.Notify(notify.Notification{Title: title, Body: body}); err != nil {
			log.Debug().Err(err).Msg("update notification failed")
		}
	}
}
