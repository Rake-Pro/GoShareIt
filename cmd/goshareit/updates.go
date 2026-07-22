package main

import (
	"context"
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
	upd      *update.Updater
	app      *core.App
	interval time.Duration
	quit     func()

	mu      sync.Mutex
	pending *update.Release
	busy    bool
}

const updateItemID = "update"

func newUpdateController(upd *update.Updater, app *core.App, interval time.Duration, quit func()) *updateController {
	return &updateController{upd: upd, app: app, interval: interval, quit: quit}
}

func (c *updateController) menuItem(ctx context.Context) tray.MenuItem {
	return tray.MenuItem{
		ID:      updateItemID,
		Title:   "Check for Updates",
		OnClick: func() { go c.onClick(ctx) },
	}
}

// start runs the periodic background check. Dev builds never auto-check (a
// 0.0.0-dev binary would otherwise immediately "upgrade" to the last release).
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
// notification; background checks only surface a found update.
func (c *updateController) check(ctx context.Context, manual bool) {
	rel, err := c.upd.Check(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("update check failed")
		if manual {
			c.notify("Update check failed", err.Error())
		}
		return
	}
	if rel == nil {
		log.Debug().Msg("update: up to date")
		if manual {
			c.notify("Up to date", "You are running the latest version.")
		}
		return
	}
	c.mu.Lock()
	c.pending = rel
	c.mu.Unlock()
	c.setTitle("Install Update v" + rel.Version)
	log.Info().Str("version", rel.Version).Msg("update available")
	c.notify("Update available", "GoShareIt v"+rel.Version+" is ready - use the tray menu to install.")
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
