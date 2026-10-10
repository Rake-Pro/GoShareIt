//go:build darwin || windows || (linux && cgo)

package ui

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"slices"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Rake-Pro/GoShareIt/internal/editor/annotate"
)

func (e *editor) layout(gtx layout.Context) layout.Dimensions {
	e.hoverWhy = ""
	e.handleWidgets(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return e.layoutToolbar(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return e.layoutCanvas(gtx)
		}),
	)
}

// handleWidgets reacts to toolbar button clicks.
func (e *editor) handleWidgets(gtx layout.Context) {
	for t, b := range e.toolBtns {
		// A greyed-out tool is laid out with gtx.Disabled, so it never
		// reports a click; selectTool also refuses it with the reason.
		if b.Clicked(gtx) {
			e.selectTool(gtx, t)
		}
	}
	if e.copyTextB.Clicked(gtx) {
		e.copySelection(gtx)
	}
	if e.copyAllB.Clicked(gtx) {
		e.copyAllText(gtx)
	}
	if e.redactSelB.Clicked(gtx) {
		e.redactSelection(gtx)
	}
	if e.quickRedactB.Clicked(gtx) {
		e.quickRedact(gtx)
	}
	if e.zoomBtn.Clicked(gtx) {
		// The readout toggles between fit and 100%.
		if e.scalePercent() == 100 {
			e.zoomFit()
		} else {
			e.zoom100()
		}
	}
	// Colour and stroke also restyle a selected shape (Select tool).
	for i, b := range e.swatchBtn {
		if b.Clicked(gtx) {
			e.setColor(e.palette[i])
		}
	}
	if e.strokeInc.Clicked(gtx) && e.stroke < maxStroke {
		e.setStroke(e.stroke + 1)
	}
	if e.strokeDec.Clicked(gtx) && e.stroke > 1 {
		e.setStroke(e.stroke - 1)
	}
	if e.undoBtn.Clicked(gtx) {
		e.undo()
	}
	if e.redoBtn.Clicked(gtx) {
		e.redoOne()
	}
	if e.cancelB.Clicked(gtx) {
		e.requestCancel()
	}
	if e.confirm.Clicked(gtx) {
		e.confirmNow(ActionConfirm)
	}
	if e.actions {
		if e.copyB.Clicked(gtx) {
			e.confirmNow(ActionCopy)
		}
		if e.saveB.Clicked(gtx) {
			e.confirmNow(ActionSave)
		}
		if e.canUpload && e.uploadB.Clicked(gtx) {
			e.confirmNow(ActionUpload)
		}
	}
}

// layoutToolbar renders two rows: a horizontally scrollable row of tool
// buttons + color swatches (whose width varies with the configured tool set),
// and a fixed action row (stroke, text field, undo/redo/cancel/confirm). The
// split guarantees Confirm/Cancel stay visible at any window width - a single
// flex row used to push them out of view on narrower screens. The whole row
// gets an explicit themed background fill spanning the full window width, so
// the window's default (light) surface never shows through.
func (e *editor) layoutToolbar(gtx layout.Context) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := e.layoutToolbarContent(gtx)
	call := macro.Stop()
	bgSize := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
	paint.FillShape(gtx.Ops, e.theme.toolbarBg, clip.Rect{Max: bgSize}.Op())
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: bgSize}
}

func (e *editor) layoutToolbarContent(gtx layout.Context) layout.Dimensions {
	th := e.th
	inset := layout.UniformInset(unit.Dp(8))
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Row 1: tools + swatches, scrollable. Wide windows show each
		// tool's shortcut key in its label; narrow ones show it in the hint
		// row for the active tool.
		wide := gtx.Constraints.Max.X >= gtx.Dp(unit.Dp(900))
		items := make([]layout.Widget, 0, len(e.tools)+len(e.palette))
		for _, t := range e.tools {
			t := t
			items = append(items, func(gtx layout.Context) layout.Dimensions {
				return e.layoutToolButton(gtx, t, wide)
			})
		}
		for i := range e.palette {
			i := i
			items = append(items, func(gtx layout.Context) layout.Dimensions {
				return e.layoutSwatch(gtx, i)
			})
		}
		if !slices.Contains(e.palette, e.col) {
			// The configured color is not one of the swatches: show it as an
			// extra, already-selected chip so the current color is visible.
			items = append(items, e.layoutCurrentColor)
		}

		// Row 2: stroke controls | text input | undo/redo | [copy/save/upload]
		// | cancel/confirm, visually grouped with spacers.
		dec := e.subtleButton(&e.strokeDec, "-")
		inc := e.subtleButton(&e.strokeInc, "+")
		cancel := e.subtleButton(&e.cancelB, "Cancel")
		ok := e.accentButton(&e.confirm, e.confirmLabel)

		row2Children := []layout.FlexChild{
			rigidBtn(dec),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(th, itoa(e.stroke)+" px")
				lbl.Color = e.theme.fg
				return layout.UniformInset(unit.Dp(6)).Layout(gtx, lbl.Layout)
			}),
			rigidBtn(inc),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(e.layoutZoomReadout),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, e.layoutRow2Middle),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return e.layoutMaybeButton(gtx, &e.undoBtn, "Undo", e.hist.CanUndo())
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return e.layoutMaybeButton(gtx, &e.redoBtn, "Redo", e.hist.CanRedo())
			}),
		}
		if e.actions {
			copyBtn := e.subtleButton(&e.copyB, "Copy")
			saveBtn := e.subtleButton(&e.saveB, "Save")
			row2Children = append(row2Children,
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				rigidBtn(copyBtn),
				rigidBtn(saveBtn),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return e.layoutUploadButton(gtx)
				}),
			)
		}
		row2Children = append(row2Children,
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			rigidBtn(cancel),
			rigidBtn(ok),
		)

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return e.toolRow.Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
					return items[i](gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, row2Children...)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return e.layoutHintRow(gtx, wide)
			}),
		)
	})
}

// layoutToolButton renders one row-1 tool button: accent when active,
// subtle otherwise, and greyed out (with its reason in the hint row on
// hover) when the tool cannot be used now.
func (e *editor) layoutToolButton(gtx layout.Context, t Tool, wide bool) layout.Dimensions {
	label := toolLabel(t)
	if k := toolKeyName(t); wide && k != "" {
		label += " (" + k + ")"
	}
	btn := e.toolBtns[t]
	if ok, why := e.toolEnabled(t); !ok {
		return e.layoutGreyed(gtx, btn, label, e.toolHover[t], why, e.tool == t)
	}
	style := e.subtleButton(btn, label)
	if e.tool == t {
		style = e.accentButton(btn, label)
	}
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, style.Layout)
}

// layoutGreyed renders an inert, dimmed button (gtx.Disabled) with a hover
// tracker on top, so the hint row can say why it is greyed out.
func (e *editor) layoutGreyed(gtx layout.Context, btn *widget.Clickable, label string, hov *gesture.Hover, why string, active bool) layout.Dimensions {
	if hov.Update(gtx.Source) {
		e.hoverWhy = why
	}
	bg := mutedColor(e.theme.surfaceBg)
	if active {
		bg = mutedColor(e.theme.accent)
	}
	b := e.styledButton(btn, label, bg, mutedColor(e.theme.fg))
	dims := layout.UniformInset(unit.Dp(4)).Layout(gtx.Disabled(), b.Layout)
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	hov.Add(gtx.Ops)
	area.Pop()
	return dims
}

// layoutZoomReadout is the clickable zoom percentage; a click toggles
// between fit and 100%. A fixed minimum width keeps the row from jittering.
func (e *editor) layoutZoomReadout(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(unit.Dp(56))
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return e.subtleButton(&e.zoomBtn, itoa(e.scalePercent())+"%").Layout(gtx)
	})
}

// layoutRow2Middle fills the flexible middle of toolbar row 2: the text
// actions in Select text, Quick redact with the Redact tool, else the
// annotation text field.
func (e *editor) layoutRow2Middle(gtx layout.Context) layout.Dimensions {
	quick := func(gtx layout.Context) layout.Dimensions {
		if ok, why := e.quickRedactEnabled(); !ok {
			return e.layoutGreyed(gtx, &e.quickRedactB, "Quick redact", &e.quickHover, why, false)
		}
		return layout.UniformInset(unit.Dp(4)).Layout(gtx, e.subtleButton(&e.quickRedactB, "Quick redact").Layout)
	}
	switch {
	case e.tool == ToolSelectText:
		hasResult := e.ocr.result != nil && e.ocr.result.WordCount() > 0
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return e.layoutMaybeButton(gtx, &e.copyTextB, "Copy text", e.hasTextSelection())
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return e.layoutMaybeButton(gtx, &e.copyAllB, "Copy all", hasResult)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return e.layoutMaybeButton(gtx, &e.redactSelB, "Redact selection", e.hasTextSelection())
			}),
			layout.Rigid(quick),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(e.th, e.selectionStatus())
				lbl.Color = mutedColor(e.theme.fg)
				lbl.MaxLines = 1
				return layout.UniformInset(unit.Dp(6)).Layout(gtx, lbl.Layout)
			}),
		)
	case e.tool == ToolRedact && e.ocr.mode != OCRHidden:
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, layout.Rigid(quick))
	}
	return e.layoutTextField(gtx)
}

// selectionStatus is the short status label next to the text actions.
func (e *editor) selectionStatus() string {
	switch {
	case e.ocr.running:
		return "Recognizing..."
	case e.ocr.result == nil:
		return ""
	case e.ocr.result.WordCount() == 0:
		return "No text found"
	case e.hasTextSelection():
		return countLabel(len(e.sel.refs), "word selected", "words selected")
	}
	return countLabel(e.ocr.result.WordCount(), "word", "words")
}

// hintRowHeight keeps the canvas from jumping when a hint appears.
const hintRowHeight = 22

// layoutHintRow renders row 3: one line of guidance, by priority (see
// hintText).
func (e *editor) layoutHintRow(gtx layout.Context, wide bool) layout.Dimensions {
	msg, strong := e.hintText(gtx, wide)
	h := gtx.Dp(unit.Dp(hintRowHeight))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = h, h
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Body2(e.th, msg)
		lbl.MaxLines = 1
		lbl.Color = mutedColor(e.theme.fg)
		if strong {
			lbl.Color = e.theme.fg
		}
		return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, lbl.Layout)
	})
}

// hintText picks the hint row message: the discard prompt, then a
// transient message ("Copied 12 characters"), then the reason of a greyed
// button under the pointer, then the recognition status, then the active
// tool's hint. strong marks messages that need attention.
func (e *editor) hintText(gtx layout.Context, wide bool) (string, bool) {
	if e.discardArmed && len(e.shapes) > 0 {
		if len(e.shapes) == 1 {
			return "Discard 1 annotation? Press Esc or Cancel again to discard this capture.", true
		}
		return "Discard " + itoa(len(e.shapes)) + " annotations? Press Esc or Cancel again to discard this capture.", true
	}
	if e.hint.text != "" && gtx.Now.Before(e.hint.until) {
		return e.hint.text, true
	}
	if e.hoverWhy != "" {
		return e.hoverWhy, false
	}
	if e.tool == ToolSelectText {
		if ok, why := e.ocrAvailable(); !ok {
			return why, false
		}
	}
	msg := e.toolHint()
	if k := toolKeyName(e.tool); !wide && k != "" {
		msg = toolLabel(e.tool) + " (" + k + "): " + msg
	}
	if wide {
		msg += ". Hold Space or middle-drag to pan, scroll to zoom."
	}
	return msg, false
}

// shortcutMod is the shortcut modifier's name on this OS.
func shortcutMod() string {
	if runtime.GOOS == "darwin" {
		return "Cmd"
	}
	return "Ctrl"
}

// toolHint is the static one-line guidance for the active tool.
func (e *editor) toolHint() string {
	switch e.tool {
	case ToolSelect:
		if e.selected >= 0 {
			return "Drag to move, arrows nudge (Shift: 10 px), Delete removes, Esc deselects"
		}
		return "Click a shape to select it; colour and stroke then restyle it"
	case ToolCrop:
		if e.crop != nil {
			return "Drag a handle to resize, inside to move, outside for a new crop; Enter applies"
		}
		return "Drag to crop; Enter applies"
	case ToolArrow:
		return "Drag from tail to tip"
	case ToolRect, ToolEllip, ToolHighlight, ToolBlur, ToolPixelate:
		return "Drag a box"
	case ToolRedact:
		return "Drag over text to hide it. Black is recommended"
	case ToolText:
		return "Type in the field, then click the image"
	case ToolStep:
		return "Click to place step " + itoa(e.nextStep())
	case ToolLine:
		return "Drag"
	case ToolFreehand:
		return "Draw"
	case ToolSelectText:
		if e.ocr.result != nil && e.ocr.result.WordCount() == 0 {
			return "No text was found in this image"
		}
		return "Drag over words to select, " + shortcutMod() + "+C copies, double-click selects a line"
	}
	return ""
}

// cursorFor picks the canvas cursor: grab while panning, an I-beam for text,
// a crosshair for drawing.
func (e *editor) cursorFor() pointer.Cursor {
	switch {
	case e.panning:
		return pointer.CursorGrabbing
	case e.spaceDown:
		return pointer.CursorGrab
	case e.tool == ToolText:
		return pointer.CursorText
	case e.tool == ToolSelectText && e.hovering && e.overWord(e.hover):
		return pointer.CursorText
	case e.tool == ToolSelect:
		return e.selectCursor()
	case e.cropDrag.active:
		return handleCursor(e.cropDrag.handle)
	case e.tool == ToolCrop && e.hovering && !e.dragging:
		return handleCursor(e.cropHandleAt(e.hoverPos))
	}
	return pointer.CursorCrosshair
}

// layoutMaybeButton renders a subtle button that is greyed out and inert when
// enabled is false (Undo/Redo with nothing to undo or redo).
func (e *editor) layoutMaybeButton(gtx layout.Context, btn *widget.Clickable, label string, enabled bool) layout.Dimensions {
	b := e.subtleButton(btn, label)
	if !enabled {
		gtx = gtx.Disabled()
		b = e.styledButton(btn, label, mutedColor(e.theme.surfaceBg), mutedColor(e.theme.fg))
	}
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, b.Layout)
}

// layoutCurrentColor draws a non-clickable chip of the current color with the
// selection ring, for a configured color that has no palette swatch.
func (e *editor) layoutCurrentColor(gtx layout.Context) layout.Dimensions {
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		sz := gtx.Dp(unit.Dp(22))
		d := image.Pt(sz, sz)
		rr := clip.RRect{Rect: image.Rectangle{Max: d}, SE: 4, SW: 4, NE: 4, NW: 4}
		paint.FillShape(gtx.Ops, e.col, rr.Op(gtx.Ops))
		paint.FillShape(gtx.Ops, e.theme.fg, clip.Stroke{Path: rr.Path(gtx.Ops), Width: 2}.Op())
		return layout.Dimensions{Size: d}
	})
}

// layoutUploadButton renders the Upload action button. When the editor was
// launched with uploads unavailable (CanUpload=false), it is disabled and
// visibly dimmed rather than left clickable-then-erroring: per house rule,
// grey out unavailable options instead of accept-then-error.
func (e *editor) layoutUploadButton(gtx layout.Context) layout.Dimensions {
	if !e.canUpload {
		gtx = gtx.Disabled()
		btn := e.styledButton(&e.uploadB, "Upload", mutedColor(e.theme.surfaceBg), mutedColor(e.theme.fg))
		return layout.UniformInset(unit.Dp(4)).Layout(gtx, btn.Layout)
	}
	btn := e.subtleButton(&e.uploadB, "Upload")
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, btn.Layout)
}

// styledButton applies uniform geometry (corner radius, text size) plus the
// given background/foreground on top of material.Button's defaults.
func (e *editor) styledButton(btn *widget.Clickable, label string, bg, fg color.NRGBA) material.ButtonStyle {
	b := material.Button(e.th, btn, label)
	b.Background = bg
	b.Color = fg
	b.CornerRadius = unit.Dp(6)
	b.TextSize = unit.Sp(13)
	return b
}

// accentButton is the primary style: accent-filled with contrast text. Used
// for the selected tool and Confirm.
func (e *editor) accentButton(btn *widget.Clickable, label string) material.ButtonStyle {
	return e.styledButton(btn, label, e.theme.accent, e.theme.contrastFg)
}

// subtleButton is the secondary style: a flat themed surface with regular
// text. Used for unselected tools, undo/redo, and Cancel.
func (e *editor) subtleButton(btn *widget.Clickable, label string) material.ButtonStyle {
	return e.styledButton(btn, label, e.theme.surfaceBg, e.theme.fg)
}

func rigidBtn(b material.ButtonStyle) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(4)).Layout(gtx, b.Layout)
	})
}

// layoutTextField renders the annotation text input with a subtle themed
// background pill so it reads as an input rather than bare text.
func (e *editor) layoutTextField(gtx layout.Context) layout.Dimensions {
	ed := material.Editor(e.th, &e.textIn, "Type text, then click the image")
	ed.Color = e.theme.fg
	ed.HintColor = mutedColor(e.theme.fg)
	return layoutPill(gtx, e.theme.surfaceBg, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(6)).Layout(gtx, ed.Layout)
	})
}

// layoutPill paints a rounded background behind w, sized to w's own
// dimensions (stretched to at least the incoming width).
func layoutPill(gtx layout.Context, bg color.NRGBA, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	rr := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: 6, SW: 6, NE: 6, NW: 6}
	paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// mutedColor returns c with reduced alpha, for secondary text like hints.
func mutedColor(c color.NRGBA) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 0xa0}
}

func (e *editor) layoutSwatch(gtx layout.Context, i int) layout.Dimensions {
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		sz := gtx.Dp(unit.Dp(22))
		return e.swatchBtn[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			d := image.Pt(sz, sz)
			rr := clip.RRect{Rect: image.Rectangle{Max: d}, SE: 4, SW: 4, NE: 4, NW: 4}
			paint.FillShape(gtx.Ops, e.palette[i], rr.Op(gtx.Ops))
			// A pure-white swatch is invisible against a light toolbar/canvas
			// background; give it a hairline border in light mode regardless
			// of selection.
			if e.theme == lightTheme && e.palette[i] == (color.NRGBA{0xff, 0xff, 0xff, 0xff}) {
				hairline := clip.Stroke{Path: rr.Path(gtx.Ops), Width: 1}.Op()
				paint.FillShape(gtx.Ops, color.NRGBA{0, 0, 0, 0x40}, hairline)
			}
			if colorsEqual(e.palette[i], e.col) {
				// Ring uses the theme fg color so it stays visible against
				// both the dark palette swatch colors and the theme bg.
				border := clip.Stroke{Path: rr.Path(gtx.Ops), Width: 2}.Op()
				paint.FillShape(gtx.Ops, e.theme.fg, border)
			}
			return layout.Dimensions{Size: d}
		})
	})
}

func (e *editor) layoutCanvas(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	// Background.
	paint.FillShape(gtx.Ops, e.theme.canvasBg, clip.Rect{Max: size}.Op())

	// Fit-to-window scale, modified by user zoom.
	bw := float32(e.bounds.Dx())
	bh := float32(e.bounds.Dy())
	if bw == 0 || bh == 0 {
		return layout.Dimensions{Size: size}
	}
	fit := math.Min(float64(size.X)/float64(bw), float64(size.Y)/float64(bh))
	e.lastFit = fit
	scale := float32(fit * e.zoom)
	dispW := bw * scale
	dispH := bh * scale
	origin := f32.Pt(
		(float32(size.X)-dispW)/2+e.panX,
		(float32(size.Y)-dispH)/2+e.panY,
	)
	e.lastScale = scale
	e.lastOrigin = origin
	e.lastSize = size
	// Hit areas in dp, so they keep their physical size on HiDPI screens.
	e.hitTolPx = float32(gtx.Dp(unit.Dp(4)))
	e.handlePx = gtx.Dp(unit.Dp(6))

	// Draw the image.
	{
		stack := op.Affine(f32.Affine2D{}.
			Scale(f32.Pt(0, 0), f32.Pt(scale, scale)).
			Offset(origin)).Push(gtx.Ops)
		e.imgOp.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}

	// Draw committed shapes and the in-progress drag in screen space. Crop
	// shapes are not drawn one by one: only the active crop applies, shown by
	// dimming everything outside it.
	for _, s := range e.shapes {
		if s.kind != kCrop {
			e.drawShape(gtx.Ops, s)
		}
	}
	if e.crop != nil {
		min, max := e.screen(e.crop.Min), e.screen(e.crop.Max)
		r := image.Rect(int(min.X), int(min.Y), int(max.X), int(max.Y))
		dim := color.NRGBA{0x00, 0x00, 0x00, 0xa0}
		fillRect(gtx.Ops, f32.Pt(0, 0), f32.Pt(float32(size.X), float32(r.Min.Y)), dim)
		fillRect(gtx.Ops, f32.Pt(0, float32(r.Max.Y)), f32.Pt(float32(size.X), float32(size.Y)), dim)
		fillRect(gtx.Ops, f32.Pt(0, float32(r.Min.Y)), f32.Pt(float32(r.Min.X), float32(r.Max.Y)), dim)
		fillRect(gtx.Ops, f32.Pt(float32(r.Max.X), float32(r.Min.Y)), f32.Pt(float32(size.X), float32(r.Max.Y)), dim)
		strokeRect(gtx.Ops, min, max, 2, color.NRGBA{0xff, 0xff, 0xff, 0xff})
		if e.tool == ToolCrop && !e.dragging {
			e.drawCropHandles(gtx.Ops, gtx.Dp(unit.Dp(8)))
		}
	}
	if e.tool == ToolSelect {
		e.drawSelection(gtx.Ops, gtx.Dp(unit.Dp(6)))
	}
	if e.tool == ToolSelectText {
		e.drawTextOverlay(gtx.Ops)
	}
	if e.dragging {
		e.drawShape(gtx.Ops, e.previewShape())
	}
	// Text tool: show the typed text at the pointer, at its real size, before
	// the click places it.
	if txt := e.textIn.Text(); e.tool == ToolText && e.hovering && txt != "" && !e.dragging {
		if e.ghost.text != txt || e.ghost.col != e.col || e.ghost.stroke != e.stroke {
			e.ghost = shape{kind: kText, text: txt, col: e.col, stroke: e.stroke}
			e.ghost.textOp, e.ghost.textSize = textImage(txt, e.col, e.stroke)
		}
		e.ghost.p0 = e.hover
		e.drawShape(gtx.Ops, e.ghost)
	}

	// Register the canvas input area for the next frame.
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, event.Tag(canvasTag))
	e.cursorFor().Add(gtx.Ops)
	area.Pop()

	return layout.Dimensions{Size: size}
}

func (e *editor) previewShape() shape {
	kind := kArrow
	switch e.tool {
	case ToolRect:
		kind = kRect
	case ToolEllip:
		kind = kEllipse
	case ToolCrop:
		kind = kCrop
	case ToolLine:
		kind = kLine
	case ToolBlur:
		kind = kBlur
	case ToolPixelate:
		kind = kPixelate
	case ToolHighlight:
		kind = kHighlight
	case ToolFreehand:
		return shape{kind: kFreehand, pts: e.freehandPts, col: e.col, stroke: e.stroke}
	case ToolRedact:
		col := e.col
		col.A = 0xff
		return shape{kind: kRedact, rects: []image.Rectangle{rectOf(e.dragFrom, e.dragTo)}, col: col}
	}
	return shape{kind: kind, p0: e.dragFrom, p1: e.dragTo, col: e.col, stroke: e.stroke}
}

func (e *editor) screen(p image.Point) f32.Point {
	return f32.Pt(
		e.lastOrigin.X+float32(p.X)*e.lastScale,
		e.lastOrigin.Y+float32(p.Y)*e.lastScale,
	)
}

func (e *editor) drawShape(ops *op.Ops, s shape) {
	w := float32(s.stroke) * e.lastScale
	if w < 1 {
		w = 1
	}
	switch s.kind {
	case kArrow:
		drawArrow(ops, e.screen(s.p0), e.screen(s.p1), w, s.col)
	case kRect:
		r := rectOf(s.p0, s.p1)
		strokeRect(ops, e.screen(r.Min), e.screen(r.Max), w, s.col)
	case kEllipse:
		r := rectOf(s.p0, s.p1)
		min, max := e.screen(r.Min), e.screen(r.Max)
		el := clip.Ellipse{Min: image.Pt(int(min.X), int(min.Y)), Max: image.Pt(int(max.X), int(max.Y))}
		paint.FillShape(ops, s.col, clip.Stroke{Path: el.Path(ops), Width: w}.Op())
	case kText:
		// The cached rendering is annotate's own output, so position and size
		// match the final image.
		if s.textSize == (image.Point{}) {
			return
		}
		st := op.Affine(f32.Affine2D{}.
			Scale(f32.Pt(0, 0), f32.Pt(e.lastScale, e.lastScale)).
			Offset(e.screen(s.p0))).Push(ops)
		s.textOp.Add(ops)
		paint.PaintOp{}.Add(ops)
		st.Pop()
	case kLine:
		strokeLine(ops, e.screen(s.p0), e.screen(s.p1), w, s.col)
	case kFreehand:
		if len(s.pts) >= 2 {
			var p clip.Path
			p.Begin(ops)
			p.MoveTo(e.screen(s.pts[0]))
			for _, pt := range s.pts[1:] {
				p.LineTo(e.screen(pt))
			}
			paint.FillShape(ops, s.col, clip.Stroke{Path: p.End(), Width: w}.Op())
		} else if len(s.pts) == 1 {
			c := e.screen(s.pts[0])
			dot := clip.Rect{Min: image.Pt(int(c.X), int(c.Y)), Max: image.Pt(int(c.X)+int(w)+1, int(c.Y)+int(w)+1)}
			paint.FillShape(ops, s.col, dot.Op())
		}
	case kBlur, kPixelate:
		// Approximate preview: translucent fill plus outline. The real pixel
		// effect (box-blur / mosaic) is applied by annotate on Confirm.
		r := rectOf(s.p0, s.p1)
		min, max := e.screen(r.Min), e.screen(r.Max)
		fillRect(ops, min, max, color.NRGBA{0x80, 0x80, 0x80, 0x60})
		strokeRect(ops, min, max, 2, color.NRGBA{0xff, 0xff, 0xff, 0xc0})
	case kHighlight:
		// Approximate preview: translucent tint in the chosen color; annotate
		// composites the same color over the region on Confirm.
		r := rectOf(s.p0, s.p1)
		min, max := e.screen(r.Min), e.screen(r.Max)
		tint := color.NRGBA{s.col.R, s.col.G, s.col.B, 0x60}
		fillRect(ops, min, max, tint)
	case kStep:
		// Filled disc preview; the centered number is rasterized by annotate.
		c := e.screen(s.p0)
		rad := float32(badgeRadius(s.stroke)) * e.lastScale
		if rad < 3 {
			rad = 3
		}
		el := clip.Ellipse{
			Min: image.Pt(int(c.X-rad), int(c.Y-rad)),
			Max: image.Pt(int(c.X+rad), int(c.Y+rad)),
		}
		paint.FillShape(ops, s.col, el.Op(ops))
	case kRedact:
		// The on-canvas look is the final look: an opaque box.
		for _, r := range s.rects {
			fillRect(ops, e.screen(r.Min), e.screen(r.Max), s.col)
		}
	case kCrop:
		r := rectOf(s.p0, s.p1)
		strokeRect(ops, e.screen(r.Min), e.screen(r.Max), 2, color.NRGBA{0xff, 0xff, 0xff, 0xff})
	}
}

func strokeLine(ops *op.Ops, a, b f32.Point, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(a)
	p.LineTo(b)
	spec := p.End()
	paint.FillShape(ops, col, clip.Stroke{Path: spec, Width: width}.Op())
}

func fillRect(ops *op.Ops, min, max f32.Point, col color.NRGBA) {
	r := image.Rect(int(min.X), int(min.Y), int(max.X), int(max.Y)).Canon()
	paint.FillShape(ops, col, clip.Rect(r).Op())
}

func strokeRect(ops *op.Ops, min, max f32.Point, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(min)
	p.LineTo(f32.Pt(max.X, min.Y))
	p.LineTo(max)
	p.LineTo(f32.Pt(min.X, max.Y))
	p.LineTo(min)
	spec := p.End()
	paint.FillShape(ops, col, clip.Stroke{Path: spec, Width: width}.Op())
}

// drawArrow draws the preview arrow the way annotate renders it: a shaft
// that stops at the head's base and a filled triangular head.
func drawArrow(ops *op.Ops, from, to f32.Point, width float32, col color.NRGBA) {
	dx := float64(to.X - from.X)
	dy := float64(to.Y - from.Y)
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l, dy/l
	head := annotate.ArrowHeadLength(float64(width), l)
	cosA, sinA := math.Cos(annotate.ArrowHeadAngle), math.Sin(annotate.ArrowHeadAngle)
	rx1, ry1 := -(ux*cosA - uy*sinA), -(ux*sinA + uy*cosA)
	rx2, ry2 := -(ux*cosA + uy*sinA), -(-ux*sinA + uy*cosA)
	p1 := f32.Pt(to.X+float32(rx1*head), to.Y+float32(ry1*head))
	p2 := f32.Pt(to.X+float32(rx2*head), to.Y+float32(ry2*head))
	if back := head * cosA; back < l {
		base := f32.Pt(to.X-float32(ux*back), to.Y-float32(uy*back))
		strokeLine(ops, from, base, width, col)
	}
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(to)
	p.LineTo(p1)
	p.LineTo(p2)
	p.Close()
	paint.FillShape(ops, col, clip.Outline{Path: p.End()}.Op())
}

func toolLabel(t Tool) string {
	switch t {
	case ToolSelect:
		return "Select"
	case ToolCrop:
		return "Crop"
	case ToolArrow:
		return "Arrow"
	case ToolRect:
		return "Rectangle"
	case ToolEllip:
		return "Ellipse"
	case ToolText:
		return "Text"
	case ToolBlur:
		return "Blur"
	case ToolPixelate:
		return "Pixelate"
	case ToolHighlight:
		return "Highlight"
	case ToolStep:
		return "Step"
	case ToolLine:
		return "Line"
	case ToolFreehand:
		return "Freehand"
	case ToolRedact:
		return "Redact"
	case ToolSelectText:
		return "Select text"
	}
	return "" // unknown tool
}

func colorsEqual(a, b color.NRGBA) bool { return a == b }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
