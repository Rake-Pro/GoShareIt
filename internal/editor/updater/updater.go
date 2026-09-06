//go:build darwin || windows

// Package updater is the out-of-process update window: a small Gio dialog
// that waits for the host to exit, downloads and verifies the release with a
// progress bar, installs it, starts the new host and closes. It runs inside
// goshareit-editor (--update <job>) so it needs no binary of its own, and it
// keeps working while the editor file underneath it is replaced: Windows
// allows renaming a running executable, macOS keeps the old inode alive.
package updater

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core/update"
)

// hostExitWait bounds how long we wait for the host to release its files
// before installing anyway (the rename-aside strategy tolerates a live host,
// but a half-closed one could still be holding the tray).
const hostExitWait = 20 * time.Second

type state struct {
	mu       sync.Mutex
	phase    string  // headline, e.g. "Downloading GoShareIt v1.4.0"
	detail   string  // second line, e.g. "3.2 MB of 12.0 MB"
	progress float32 // 0..1, or -1 for indeterminate
	err      error
	done     bool
}

func (s *state) set(phase, detail string, progress float32) {
	s.mu.Lock()
	s.phase, s.detail, s.progress = phase, detail, progress
	s.mu.Unlock()
}

// Run performs the update described by job while showing progress, and
// returns when the window closes. dark selects the palette. It returns the
// error that stopped the update, if any (the window stays open until the
// user closes it in that case).
func Run(job update.Job, dark bool) error {
	st := &state{progress: -1}
	workerDone := make(chan struct{})
	w := new(app.Window)
	w.Option(
		app.Title("GoShareIt Update"),
		app.Size(unit.Dp(440), unit.Dp(150)),
		app.MinSize(unit.Dp(440), unit.Dp(150)),
		app.MaxSize(unit.Dp(440), unit.Dp(150)),
	)

	go func() {
		defer close(workerDone)
		err := perform(job, st, w)
		st.mu.Lock()
		st.err = err
		st.done = true
		st.mu.Unlock()
		w.Invalidate()
		if err == nil {
			// Give the last frame a moment to paint, then close.
			time.Sleep(600 * time.Millisecond)
			w.Perform(system.ActionClose)
		}
	}()

	err := loop(w, st, dark)
	// The window can be closed at any time; the install itself must not be
	// interrupted half-way through a file swap, so wait for the worker.
	<-workerDone
	st.mu.Lock()
	if err == nil {
		err = st.err
	}
	st.mu.Unlock()
	return err
}

// perform is the update itself; UI state flows through st. The host has
// already quit by the time this runs, so any failure before the swap brings
// the old host back before reporting.
func perform(job update.Job, st *state, w *app.Window) (err error) {
	ctx := context.Background()

	st.set("Waiting for GoShareIt to close", "", -1)
	w.Invalidate()
	if !update.WaitExit(job.HostPID, hostExitWait) {
		log.Warn().Int("pid", job.HostPID).Msg("host still running after wait; continuing")
	}
	installed := false
	defer func() {
		if err == nil || installed {
			return
		}
		if rerr := update.Relaunch(job.Relaunch, job.Args...); rerr != nil {
			err = fmt.Errorf("%w (and GoShareIt could not be restarted: %v; start it manually)", err, rerr)
		} else {
			err = fmt.Errorf("%w (GoShareIt has been restarted unchanged)", err)
		}
	}()

	upd, err := update.New(update.Config{Repo: job.Repo, Current: job.Current, APIBaseURL: job.APIBaseURL})
	if err != nil {
		return err
	}
	st.set("Checking release", "", -1)
	w.Invalidate()
	rel, err := upd.Check(ctx)
	if err != nil {
		return err
	}
	if rel == nil {
		// Already current (raced with another install); just bring the host back.
		st.set("Already up to date", "Starting GoShareIt", -1)
		w.Invalidate()
		installed = true
		return update.Relaunch(job.Relaunch, job.Args...)
	}

	st.set("Downloading GoShareIt v"+rel.Version, "", 0)
	w.Invalidate()
	archive, err := upd.DownloadProgress(ctx, rel, func(done, total int64) {
		p := float32(-1)
		detail := humanBytes(done)
		if total > 0 {
			p = float32(done) / float32(total)
			detail += " of " + humanBytes(total)
		}
		st.set("Downloading GoShareIt v"+rel.Version, detail, p)
		w.Invalidate()
	})
	if err != nil {
		return err
	}
	defer os.Remove(archive)

	st.set("Installing v"+rel.Version, "", -1)
	w.Invalidate()
	target := job.HostExe
	if target == "" {
		target = job.Relaunch
	}
	if _, err := update.ApplyFor(archive, target); err != nil {
		return err
	}
	installed = true

	st.set("Starting GoShareIt", "v"+rel.Version+" installed", 1)
	w.Invalidate()
	if err := update.Relaunch(job.Relaunch, job.Args...); err != nil {
		return fmt.Errorf("installed v%s but could not start it: %w (start GoShareIt manually)", rel.Version, err)
	}
	return nil
}

func loop(w *app.Window, st *state, dark bool) error {
	th := newTheme(dark)
	bg, fg := th.Palette.Bg, th.Palette.Fg
	var closeBtn widget.Clickable
	var ops op.Ops
	for {
		switch ev := w.Event().(type) {
		case app.DestroyEvent:
			st.mu.Lock()
			err := st.err
			st.mu.Unlock()
			if ev.Err != nil {
				return ev.Err
			}
			return err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, ev)
			paint.Fill(gtx.Ops, bg)
			if closeBtn.Clicked(gtx) {
				w.Perform(system.ActionClose)
			}
			st.mu.Lock()
			phase, detail, progress, err, done := st.phase, st.detail, st.progress, st.err, st.done
			st.mu.Unlock()
			layout.UniformInset(unit.Dp(18)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						title := phase
						if err != nil {
							title = "Update failed"
						}
						l := material.Body1(th, title)
						l.Font.Weight = 600
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						text := detail
						if err != nil {
							text = err.Error()
						}
						l := material.Body2(th, text)
						l.Color = fg
						l.Color.A = 0xb0
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if err != nil {
							return layout.E.Layout(gtx, material.Button(th, &closeBtn, "Close").Layout)
						}
						p := progress
						if p < 0 {
							// Indeterminate: sweep a marker back and forth.
							t := float32(time.Now().UnixMilli()%1600) / 1600
							if t > 0.5 {
								t = 1 - t
							}
							p = t * 2
							if !done {
								gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(40 * time.Millisecond)})
							}
						}
						return material.ProgressBar(th, p).Layout(gtx)
					}),
				)
			})
			ev.Frame(gtx.Ops)
		}
	}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
