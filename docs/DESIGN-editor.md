# DESIGN: Phase 4 - Post-Capture Annotation Editor

Status: proposed
Scope: design only. No code in this phase.
Author: GoShareIt core
Audience: maintainers implementing P4.

## 1. Goal

Insert an OPTIONAL annotation/editing step between capture and upload:

```
capture -> [edit] -> after-capture (save/clipboard) -> name -> upload -> after-upload
```

The user captures a region/screen, an editor window appears, they annotate
(crop, arrow, rectangle, text, blur, ...), then confirm. The (possibly modified)
image flows on through the existing pipeline unchanged. If the user skips or
cancels, the original capture flows through untouched. The editor must run on
**both macOS and Windows from one codebase**, alongside the existing menu-bar
(`LSUIElement`) host that uses `fyne.io/systray` + `golang.design/x/hotkey`.

Reference feature target (ShareX / Greenshot-derived editor): crop, arrow, line,
rectangle/ellipse, freehand, text, highlight, blur/pixelate, step numbers, plus
copy/save/upload actions from inside the editor.

## 2. The central decision: which GUI approach

This is the load-bearing decision for the whole phase. The hard constraint that
dominates it is **main-thread ownership**: on macOS exactly one component may own
the `NSApplication` main run loop, and `systray.Run` already owns it (see
`platform/darwin/tray.go`, which documents this explicitly: systray installs and
owns the Cocoa main run loop on the process's first thread, and the hotkey
backend attaches to `CFRunLoopGetMain()` so it can share that one loop). Any
in-process GUI toolkit that also wants to spin up an `NSApplication` / main run
loop will fight systray for the main thread. You cannot run two `NSApplication`
instances in one process.

### 2.1 Candidates evaluated

| Approach | X-plat parity | Canvas/drawing fit | Image ops (blur/pixelate) | Packaging impact | cgo burden | Coexists with systray main loop | Dev effort |
|---|---|---|---|---|---|---|---|
| **Fyne** (`fyne.io`) | Good (one codebase) | Mediocre: retained `CanvasObject` tree; per-frame redraw of a drag-interactive annotation canvas is awkward and slow; `canvas.Raster` is static-image oriented | Done in pure Go on `image.Image` before handing to Fyne; Fyne itself does not help | +large (GL driver, fonts) | cgo on both OSes (GL) | **Conflict**: Fyne's driver owns its own `NSApp`/event loop on the main goroutine | Medium |
| **Gio** (`gioui.org`) | Good (one codebase) | **Best**: immediate-mode GPU surface, you own pointer/clip/paint each frame, maps directly to a live drawing canvas with drag handles and live previews | Pure Go on `image.Image`; upload the result as a Gio image op for preview | +medium (no heavy widget toolkit) | cgo on macOS; effectively no-cgo path on Windows (d3d11/syscall) | **Conflict in-process** (wants the main goroutine via `app.Main`), but cleanly **solved by running it out-of-process** | Medium-high |
| **Native per-OS** (AppKit/Cocoa + Win32/Direct2D via cgo/syscall) | Poor: two separate UI implementations to build and maintain | Excellent (native canvases) | Native or pure Go | +small | Heavy cgo on mac, heavy syscall on win; doubles UI surface | Same main-loop conflict on mac unless out-of-process | **Highest** |
| **Web/HTML in a webview** (wails / `webview_go`) | Good UI parity, but split language | HTML `<canvas>` is genuinely excellent for annotation UIs (lots of prior art, `ctx.filter='blur()'`, easy hit-testing) | Done in JS canvas or pre-processed in Go | +large on Windows (**WebView2 runtime dependency**), WKWebView on mac | cgo (WKWebView); WebView2 dep on Windows | webview also wants the main thread; same conflict | Medium, but splits codebase into Go + JS/HTML/CSS |
| **Shell to external editor** | N/A (delegates) | N/A | N/A | +0 | none | No conflict (separate process) | Low, but **no control, no parity, no consistent UX**, depends on user having an editor installed | Lowest |

### 2.2 Recommendation: Gio, run as an out-of-process editor helper

**Pick Gio (`gioui.org`), invoked out-of-process by the `Editor` seam.**

Two coupled decisions:

1. **Toolkit = Gio.** For an annotation editor the work is a custom drawing
   canvas, not a form of stock widgets. Gio's immediate-mode GPU model gives
   direct control of the paint surface, pointer events, clip stacks and per-frame
   redraw, which is exactly what live drag-to-draw shapes, resize handles and a
   live blur/pixelate preview need. Fyne's retained-object canvas fights you for
   this; the "build it yourself" cost of Gio is not a real penalty here because an
   annotation canvas is a from-scratch widget in any toolkit. Gio keeps the whole
   thing in Go (no JS/HTML split), produces a single static binary, and has no
   external runtime dependency (unlike WebView2 on Windows). cgo on macOS is
   already a fact of this project (capture and hotkeys are cgo); on Windows Gio
   stays on the syscall/d3d11 path, matching the project's "pure syscall on
   Windows" posture.

2. **Run it out-of-process.** This is the decisive architectural move and it is
   independent of toolkit choice: *any* in-process GUI toolkit collides with
   systray over the single macOS main run loop. Rather than try to interleave two
   main loops in one process (fragile, OS-specific, and directly contradicts the
   contract documented in `platform/darwin/tray.go`), the editor lives in a
   **separate helper that owns its own main loop in its own process**. The
   menu-bar host process keeps systray + hotkeys on its main thread, untouched.
   The host spawns the helper, hands it the captured PNG, blocks on it, and reads
   back the edited PNG. The helper is either a second entry point of the same
   binary (re-exec `os.Args[0] --editor`) or a sibling binary bundled next to it.

This sidesteps the hardest integration problem entirely, keeps the editor in Go,
ships as one (or two) static binaries with no runtime dependency, and gives the
best canvas substrate for the actual annotation work.

Rejected:
- **Fyne**: poor canvas fit for interactive annotation; still has the main-loop
  conflict; no upside over Gio for this use case.
- **Native per-OS**: doubles the UI codebase across Cocoa and Win32, the highest
  effort, contradicting the one-codebase requirement.
- **Webview/HTML**: splits the codebase into Go + web, and the WebView2 runtime
  dependency on Windows undercuts the project's single-static-binary packaging.
  Kept as a fallback only if Gio's text-input/IME ergonomics prove too painful.
- **External editor**: no control over UX or feature parity, and depends on the
  user's environment; unacceptable as the primary path.

## 3. The `Editor` seam

A new portable interface in `internal/core`, mirroring the existing seam style
(`capture.Capturer`, `upload.Uploader`, `tray.Tray`). It operates purely on
`capture.Result`, so the core stays oblivious to the GUI.

New package `internal/core/edit`:

```go
// Package edit defines the portable annotation-editor seam. The core depends
// only on the Editor interface; the concrete GUI lives out-of-process and is
// invoked by a host-side implementation.
package edit

import (
    "context"

    "github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// Editor presents the captured image for annotation and returns the result.
//
// Contract:
//   - Returns the edited capture.Result (Bytes re-encoded, Mime/Kind updated,
//     Path cleared since bytes no longer match any on-disk file).
//   - If the user SKIPS or CANCELS, returns the input Result unchanged and
//     ok=false. Callers MUST treat ok=false as "proceed with the original".
//   - Returns an error only for genuine failures (helper crash, decode error).
//     A failure is non-fatal to the pipeline: the caller logs and continues
//     with the original image (fail-open), matching the app's "never fatal on
//     a non-core failure" posture.
//   - Only KindImage is editable. KindVideo is returned unchanged, ok=false.
Edit(ctx context.Context, in capture.Result) (out capture.Result, ok bool, err error)
}

// NoopEditor is the default when editing is disabled. It always passes through.
type NoopEditor struct{}

func (NoopEditor) Edit(_ context.Context, in capture.Result) (capture.Result, bool, error) {
    return in, false, nil
}
```

Wire it into the existing provider bundle in `internal/core/app.go`
(`core.Providers`), defaulting to `NoopEditor` when unset or disabled so existing
wiring keeps working:

```go
type Providers struct {
    Capturer  capture.Capturer
    Uploader  upload.Uploader
    Clipboard clipboard.Clipboard
    Notifier  notify.Notifier
    Tray      tray.Tray
    Hotkeys   hotkey.Manager
    Editor    edit.Editor // optional; nil -> NoopEditor
}
```

`core.New` substitutes `edit.NoopEditor{}` when `p.Editor == nil`, exactly as it
already tolerates nil `Notifier`/`Tray`.

## 4. Pipeline integration

The editor is a single optional step inserted **immediately after capture and
before after-capture side effects**, so that the saved-local file and the
clipboard image reflect the *edited* result, and the upload uploads the edited
bytes. Change to `runPipeline` in `internal/core/pipeline.go`, between step 1
(Capture) and step 2 (After-capture):

```go
// 1. Capture.
res, err := a.capturer.Capture(ctx, req)
// ... existing empty-result guard ...

// 1b. Optional edit (gated by config + per-request flag).
if req.Edit && res.Kind == capture.KindImage {
    edited, ok, eerr := a.editor.Edit(ctx, res)
    switch {
    case eerr != nil:
        a.log.Warn().Err(eerr).Msg("editor failed; using original capture")
    case ok:
        res = edited // edited.Path is "", edited.Bytes/Mime are authoritative
    default:
        // skipped or cancelled: keep original res unchanged.
    }
}

// 2. After-capture: save + clipboard now see the edited bytes.
```

Notes:
- **Gating**: a new `Edit bool` on `capture.Request`, set from config in
  `App.RunCapture` (`internal/core/app.go`), the same way `CopyToClipboard`,
  `SaveLocal`, `SaveDir` are populated today. A per-mode override is possible
  later but not required for v1.
- **Cancel vs skip vs confirm** are all collapsed into `(out, ok, err)`: confirm
  -> `(edited, true, nil)`; cancel/skip -> `(in, false, nil)`; failure ->
  `(in, false, err)`. The pipeline only ever advances with valid bytes.
- **Fail-open**: an editor error never aborts the capture; the original image
  uploads. This matches the existing pattern where clipboard/notify/history
  failures are logged at warn and the pipeline continues.
- **Path hygiene**: when edited, `Path` is cleared because the bytes no longer
  match any file on disk. `saveLocal` re-derives a filename and writes the edited
  bytes; `extFor` already maps `Mime` to extension, so a re-encode to PNG/JPEG is
  handled.

## 5. Main-thread coexistence (the hard part)

### 5.1 The problem

`systray.Run` owns the macOS main run loop on the process's first OS thread for
the lifetime of the app (documented in `platform/darwin/tray.go`). The hotkey
backend deliberately shares that same loop via `CFRunLoopGetMain()`. A Gio (or
any) editor window needs *its own* main run loop on *its own* main thread. In one
process on macOS that is a direct, unresolvable conflict: two `NSApplication`
owners cannot coexist.

### 5.2 The solution: out-of-process editor helper

The `Editor` implementation that lives in the menu-bar process is a thin
**launcher**. It does not draw anything. It:

1. Writes the input PNG to a temp file (or a pipe).
2. Spawns the editor helper process and **blocks** (`exec.CommandContext`),
   honoring `ctx` (cancel -> kill the child -> treated as cancel/skip).
3. On exit code 0, reads the edited PNG back (temp file / stdout); on non-zero or
   a sentinel "cancelled" code, returns `ok=false`.

The helper process owns its own Gio main loop on its own main thread with zero
interaction with systray. The two processes never share a run loop, so the
hardest constraint disappears by construction.

```go
// internal/core/edit/proc/launcher.go  (host side, real Editor impl)
type Launcher struct {
    HelperPath string        // path to the editor helper binary/entrypoint
    Timeout    time.Duration // safety cap; 0 = no cap
    Tools      []string      // enabled tools, passed as flags/args
}

func (l Launcher) Edit(ctx context.Context, in capture.Result) (capture.Result, bool, error) {
    if in.Kind != capture.KindImage {
        return in, false, nil
    }
    // 1. write in.Bytes to a temp .png
    // 2. exec helper with --in <tmpIn> --out <tmpOut> [tool flags]; wait on ctx
    // 3. inspect exit code:
    //      0  -> read tmpOut, return (edited, true, nil)
    //      64 -> cancelled/skipped, return (in, false, nil)   // sentinel
    //      _  -> return (in, false, fmt.Errorf("editor exit %d", code))
}
```

Helper entry point: re-exec the same binary with a hidden flag
(`goshareit --editor --in ... --out ...`). `cmd/goshareit/main.go` checks the
flag *before* config/tray wiring and dispatches into the Gio editor `main`
instead of the menu-bar `run`. On macOS the helper does NOT set `LSUIElement`
behavior for its own window (it is a normal foreground window that comes to front
and is dismissed on confirm/cancel); the parent stays the menu-bar agent.

### 5.3 Why not in-process with cooperative scheduling

Rejected: trying to hand the main thread between systray and Gio (e.g. tearing
down systray, running Gio, restoring systray) is fragile, loses the menu bar
while editing, and is OS-specific glue that directly contradicts the single-owner
contract the codebase already documents. Process separation is simpler, robust,
and identical on both OSes.

### 5.4 Windows note

On Windows there is no `NSApplication` singleton, but the same out-of-process
design is used for symmetry: one architecture, one code path, no per-OS branching
in the pipeline. The helper owns its own message loop; the host's systray
(pure-syscall path) is unaffected.

## 6. Feature phasing

### P4a - Viewer + core annotations (MVP)
- Window shows the captured image at native pixels, zoom-to-fit, pan.
- Tools: **crop**, **arrow**, **rectangle/ellipse**, **text**.
- Color + stroke-width picker; undo/redo stack.
- Actions: **Confirm** (return edited), **Cancel/Skip** (return original), Esc to
  cancel. Output re-encoded as PNG.
- This alone delivers the 80% use case and exercises the full seam + IPC.

### P4b - Obscure + callouts
- **Blur** and **pixelate** over a selected region (pure Go: box/gaussian blur
  and block-average on the sub-image, composited back).
- **Highlight** (multiply/alpha rectangle).
- **Step numbers** (auto-incrementing numbered badges).
- **Freehand** / line.

### P4c - Full ShareX/Greenshot parity
- In-editor actions toolbar: **copy to clipboard**, **save as**, **upload now**
  (these reuse the existing downstream seams; see section 7).
- Speech-bubble callouts, obfuscation modes, magnifier, sticker/emoji, image
  resize, configurable defaults, remembered last tool, more shape options.

Image manipulation throughout is pure Go on `image.Image`
(`image`, `image/draw`, `golang.org/x/image/draw`, plus a small blur/pixelate
helper). Gio only renders the working surface and captures pointer input; the
pixel work is toolkit-independent and unit-testable without a GUI.

## 7. Config surface and after-edit actions

New config block (in `internal/core/config/config.go`, additive, all optional):

```yaml
editor:
  enabled: false          # master switch; false -> NoopEditor, current behavior
  on_modes: [region]      # which capture modes open the editor (region|fullscreen|window)
  helper_path: ""         # "" -> re-exec self with --editor
  timeout_seconds: 0      # 0 -> no timeout
  default_tool: arrow
  stroke_width: 3
  color: "#ff3b30"
  tools:                  # allow trimming the toolbar
    - crop
    - arrow
    - rect
    - text
    - blur
    - highlight
    - step
```

Go shape:

```go
type EditorConfig struct {
    Enabled        bool     `yaml:"enabled"`
    OnModes        []string `yaml:"on_modes"`
    HelperPath     string   `yaml:"helper_path"`
    TimeoutSeconds int      `yaml:"timeout_seconds"`
    DefaultTool    string   `yaml:"default_tool"`
    StrokeWidth    int      `yaml:"stroke_width"`
    Color          string   `yaml:"color"`
    Tools          []string `yaml:"tools"`
}
```

Add `Editor EditorConfig` to `config.Config`. `App.RunCapture` sets
`req.Edit = cfg.Editor.Enabled && modeInList(mode, cfg.Editor.OnModes)`.

**After-edit actions.** The pipeline's existing after-capture / after-upload
behavior (copy image to clipboard, save local, upload, copy URL, notify, history)
is the canonical path and is **unchanged** - the editor just feeds it edited
bytes. The editor's own in-window buttons in P4c (copy / save / upload now) are a
convenience that drive the *same* outcomes:
- "Copy" exits with the edited bytes and the pipeline's clipboard step handles it
  (no clipboard seam needed inside the helper).
- "Save as" can prompt in-helper, but the default save path is the existing
  `save_local`.
- "Upload now" simply confirms; the normal pipeline uploads. No duplicate upload
  logic in the editor.

This keeps a single source of truth for side effects and avoids re-implementing
clipboard/upload inside the out-of-process helper.

## 8. Effort estimate and biggest risk

**Effort** (one engineer, familiar with Go; Gio learning curve included):
- Seam + pipeline gate + config + NoopEditor + tests: ~2-3 days.
- Out-of-process launcher + helper entrypoint + IPC (temp-file PNG round-trip,
  exit-code protocol, ctx-kill): ~2-3 days.
- P4a Gio editor (surface, zoom/pan, crop/arrow/rect/text, undo, confirm/cancel):
  ~1.5-2.5 weeks.
- P4b (blur/pixelate/highlight/step/freehand, all pure-Go pixel ops + UI): ~1-1.5
  weeks.
- P4c parity + in-editor action buttons + polish: ~2-3 weeks.

P4a (shippable MVP) lands in roughly **2.5-3 weeks**; full parity in **6-9
weeks** total.

**Single biggest risk: Gio text input and IME.** Interactive on-canvas **text
annotation** (caret, selection, multi-line, and especially CJK/IME composition)
is the least mature, most fiddly part of Gio and the most likely to consume
unplanned time and produce platform-divergent behavior between macOS and Windows.
Mitigation: in P4a, scope text to single-line, click-to-place, Latin input via
Gio's `widget.Editor`, defer rich/IME text to P4b/P4c, and keep the webview/HTML
editor as a documented fallback if Gio text ergonomics prove unworkable - the
out-of-process architecture means swapping the helper's toolkit does not touch
the core seam or pipeline at all.
