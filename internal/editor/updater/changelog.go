//go:build darwin || windows

package updater

import (
	"context"
	"image/color"
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

	"github.com/Rake-Pro/GoShareIt/internal/core/update"
)

// RunChangelog shows what changed between the running version and the
// release in job (every published release in between, oldest first) and
// asks whether to update now. It returns proceed=true for "Update now",
// false for "Later" or a closed window. A notes fetch failure is shown in
// place; the buttons still work, since the notes are informational.
func RunChangelog(job update.Job, dark bool) (proceed bool, err error) {
	w := new(app.Window)
	w.Option(
		app.Title("What's new in GoShareIt v"+job.Version),
		app.Size(unit.Dp(560), unit.Dp(480)),
		app.MinSize(unit.Dp(420), unit.Dp(300)),
	)

	var (
		mu      sync.Mutex
		lines   []update.NoteLine
		loadErr error
		loaded  bool
	)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var notes []update.ReleaseNotes
		upd, e := update.New(update.Config{Repo: job.Repo, Current: job.Current, APIBaseURL: job.APIBaseURL})
		if e == nil {
			notes, e = upd.Notes(ctx, job.Current, job.Version)
		}
		mu.Lock()
		if e != nil {
			loadErr = e
		} else if len(notes) == 0 {
			lines = []update.NoteLine{{Text: "No release notes were published for this span."}}
		} else {
			lines = update.RenderNotes(notes)
		}
		loaded = true
		mu.Unlock()
		w.Invalidate()
	}()

	th := newTheme(dark)
	var (
		list    widget.List
		updBtn  widget.Clickable
		lateBtn widget.Clickable
		ops     op.Ops
		choice  = false
	)
	list.Axis = layout.Vertical
	for {
		switch ev := w.Event().(type) {
		case app.DestroyEvent:
			return choice, ev.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, ev)
			paint.Fill(gtx.Ops, th.Palette.Bg)
			if updBtn.Clicked(gtx) {
				choice = true
				w.Perform(system.ActionClose)
			}
			if lateBtn.Clicked(gtx) {
				choice = false
				w.Perform(system.ActionClose)
			}
			mu.Lock()
			cur, e, ok := lines, loadErr, loaded
			mu.Unlock()
			layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Body1(th, "GoShareIt v"+job.Version+" is available (you have v"+job.Current+").")
						l.Font.Weight = 600
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						switch {
						case !ok:
							return material.Body2(th, "Loading release notes...").Layout(gtx)
						case e != nil:
							return material.Body2(th, "Release notes unavailable: "+e.Error()).Layout(gtx)
						}
						return material.List(th, &list).Layout(gtx, len(cur), func(gtx layout.Context, i int) layout.Dimensions {
							line := cur[i]
							if line.Text == "" {
								return layout.Spacer{Height: unit.Dp(8)}.Layout(gtx)
							}
							l := material.Body2(th, line.Text)
							if line.Heading {
								l = material.Body1(th, line.Text)
								l.Font.Weight = 600
							}
							return layout.Inset{Bottom: unit.Dp(3)}.Layout(gtx, l.Layout)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								b := material.Button(th, &lateBtn, "Later")
								b.Background = th.Palette.Bg
								b.Color = th.Palette.Fg
								return b.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(material.Button(th, &updBtn, "Update now").Layout),
						)
					}),
				)
			})
			ev.Frame(gtx.Ops)
		}
	}
}

// newTheme is the shared light/dark palette of the updater windows.
func newTheme(dark bool) *material.Theme {
	th := material.NewTheme()
	bg := color.NRGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff}
	fg := color.NRGBA{R: 0xe8, G: 0xe8, B: 0xe8, A: 0xff}
	if !dark {
		bg = color.NRGBA{R: 0xf2, G: 0xf2, B: 0xf4, A: 0xff}
		fg = color.NRGBA{R: 0x1c, G: 0x1c, B: 0x1e, A: 0xff}
	}
	th.Palette = material.Palette{
		Bg:         bg,
		Fg:         fg,
		ContrastBg: color.NRGBA{R: 0x0a, G: 0x84, B: 0xff, A: 0xff},
		ContrastFg: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	}
	return th
}
