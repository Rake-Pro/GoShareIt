# DESIGN: Phase 3 - Screen Recording + GIF

Status: proposed
Scope: P3a (darwin video) -> P3b (GIF) -> P3c (windows, deferred)
Touches: `internal/core/capture`, `internal/core/app.go`, `internal/core/pipeline.go`, `cmd/goshareit/main.go`, `platform/darwin`

## 1. The core problem: recording is stateful

Every existing capture mode is **one-shot**: `Capturer.Capture(ctx, Request) (Result, error)` blocks, produces bytes, returns. The whole pipeline (`runPipeline`) assumes this - it calls capture, gets a `Result`, then names/uploads it synchronously.

Recording is **stateful**: `start -> (time passes, user does work) -> stop -> finalize`. The duration is open-ended and driven by a *second* user action (a hotkey press or a tray click), not by the call site. There is no single blocking call that maps cleanly onto `Capture`.

Three ways to model this:

1. **Streaming** - `Capture` returns a channel/io.Reader the core drains while recording runs. Rejected: it forces the upload step to either stream-while-recording (Nextcloud WebDAV PUT wants a known length and a finalized file) or buffer the whole stream anyway. It also leaks the "still running" state into the pipeline, which is built to run start->finish in one call. High complexity, no payoff for a screen recorder whose output is a finalized mp4.

2. **Block until stopped** - `Capture` blocks for the entire recording and some out-of-band signal tells it to stop. Rejected: the trigger that stops a recording (hotkey, tray) lives in `main.go`, far from the blocked `Capture` goroutine. You would need a shared stop channel threaded through `Request`, which pollutes the one-shot contract for every backend.

3. **Explicit start/stop (chosen)** - a separate `Recorder` seam with `Start` and `Stop`. `Start` kicks off the OS recorder and returns immediately; the process keeps running in the background (exactly how `_prototype/capture/record.go` does it: `cmd.Run()` inside a goroutine, stop via `os.Interrupt`). `Stop` interrupts, waits for the file to finalize, reads the bytes, and returns a normal `capture.Result`. That `Result` then flows through the **existing** pipeline unchanged.

Start/stop wins because it matches the OS reality (a long-lived `ffmpeg`/ScreenCaptureKit session you signal to stop), keeps the one-shot `Capturer` contract clean, and reuses the entire after-capture/name/upload path by handing back a `capture.Result` at the end.

## 2. The `Recorder` seam and `Providers` change

New interface, alongside `Capturer`, in `internal/core/capture/capture.go` (same package - it shares `Mode`/`Result`/`Kind`):

```go
// Recorder is the stateful capture seam for screen recording and GIF. Unlike
// Capturer (one-shot), a Recorder is started, runs in the background, and is
// later stopped to finalize the media. Implementations are platform-specific
// and must be safe for concurrent Start/Stop/Recording calls.
type Recorder interface {
	// Start begins recording for the given mode (VideoRegion, VideoFull, GIF).
	// It returns once the OS recorder is running; it does not block for the
	// duration. Calling Start while already recording returns ErrAlreadyRecording.
	Start(ctx context.Context, mode Mode) error

	// Stop ends the active recording, finalizes the container, and returns the
	// encoded media as a Result (Kind=KindVideo, Mime video/mp4 or image/gif).
	// Calling Stop with no active recording returns ErrNotRecording.
	Stop(ctx context.Context) (Result, error)

	// Recording reports whether a recording is currently in progress.
	Recording() bool

	// Capabilities advertises which video/GIF modes this Recorder supports.
	Capabilities() Caps
}
```

Sentinel errors (same file):

```go
var (
	ErrAlreadyRecording = errors.New("capture: recording already in progress")
	ErrNotRecording     = errors.New("capture: no recording in progress")
)
```

`core.Providers` gains an **optional** `Recorder` field (`internal/core/app.go`):

```go
type Providers struct {
	Capturer  capture.Capturer
	Recorder  capture.Recorder // optional: nil means recording unsupported
	Uploader  upload.Uploader
	Clipboard clipboard.Clipboard
	Notifier  notify.Notifier
	Tray      tray.Tray
	Hotkeys   hotkey.Manager
}
```

`core.App` carries it through:

```go
type App struct {
	// ... existing fields ...
	recorder capture.Recorder // may be nil
}
```

`New` does **not** require it (unlike Capturer/Uploader/Clipboard). It is wired straight through:

```go
return &App{
	// ... existing ...
	recorder: p.Recorder,
}, nil
```

Plus an accessor and a capability check:

```go
// Recorder exposes the recorder seam (may be nil if unsupported).
func (a *App) Recorder() capture.Recorder { return a.recorder }

// RecordingSupported reports whether this build can record.
func (a *App) RecordingSupported() bool { return a.recorder != nil }
```

`nil` is the contract for "recording unsupported": the Windows build (P3c) and any build that fails to locate ffmpeg can ship with `Recorder: nil`, and the macOS/linux wiring tolerates it because every consumer guards on `RecordingSupported()` / a nil check before use. No interface is forced onto a platform before it can implement it.

## 3. Pipeline integration

The video `Result` returned by `Stop` is a normal `capture.Result` with `Kind=KindVideo` and `Mime="video/mp4"` (or `"image/gif"` for GIF). It must flow through the **same** after-capture/name/upload/after-upload path. The good news: `pipeline.go` already mostly handles this.

What already works:
- `extFor` already maps `video/mp4`, `video/webm`, `image/gif`, and falls back to `mp4` for `KindVideo`. No change.
- The clipboard image-copy step is already gated on `res.Kind == capture.KindImage`, so video skips it. No change.
- `name.Render`, local save, upload, history, notify are all media-agnostic.
- The uploader handles non-image mime and uses the `/download` link form for non-images (per the existing upload contract).

What needs to change - **factor the post-capture half of `runPipeline` into a reusable method** so both the one-shot `Capture` path and the `Stop` path share it:

```go
// runPipeline stays the entry point for one-shot captures.
func (a *App) runPipeline(ctx context.Context, req capture.Request) (upload.UploadResult, error) {
	res, err := a.capturer.Capture(ctx, req)
	if err != nil {
		return upload.UploadResult{}, fmt.Errorf("capture: %w", err)
	}
	return a.processResult(ctx, req, res)
}

// processResult runs after-capture -> name -> upload -> after-upload on an
// already-captured Result. Shared by one-shot capture and recording stop.
func (a *App) processResult(ctx context.Context, req capture.Request, res capture.Result) (upload.UploadResult, error) {
	if len(res.Bytes) == 0 {
		return upload.UploadResult{}, fmt.Errorf("capture: empty result")
	}
	// ... existing steps 2-5 verbatim (save local, clipboard image copy,
	//     name, upload, history, notify) ...
}
```

New `App` methods drive the recorder and reuse `processResult`:

```go
// StartRecording begins a recording for the given mode.
func (a *App) StartRecording(ctx context.Context, mode capture.Mode) error {
	if a.recorder == nil {
		return fmt.Errorf("recording not supported on this build")
	}
	return a.recorder.Start(ctx, mode)
}

// StopRecording finalizes the active recording and runs it through the
// upload pipeline, returning the upload result.
func (a *App) StopRecording(ctx context.Context) (upload.UploadResult, error) {
	if a.recorder == nil {
		return upload.UploadResult{}, fmt.Errorf("recording not supported on this build")
	}
	res, err := a.recorder.Stop(ctx)
	if err != nil {
		return upload.UploadResult{}, fmt.Errorf("stop recording: %w", err)
	}
	req := capture.Request{
		CopyToClipboard: a.cfg.AfterCapture.CopyImageToClipboard,
		SaveLocal:       a.cfg.AfterCapture.SaveLocal,
		SaveDir:         a.cfg.AfterCapture.SaveDir,
	}
	return a.processResult(ctx, req, res)
}

// Recording reports whether a recording is active.
func (a *App) Recording() bool {
	return a.recorder != nil && a.recorder.Recording()
}
```

This is the only pipeline change: a pure refactor (extract `processResult`) plus three thin App methods. No behavioral change to existing capture flows.

One edge case: the macOS recorder may already have written the mp4 to a temp path during `Stop`. `processResult` reads `res.Bytes`, so the Recorder must read the finalized file into `Bytes` (and set `Path` only when `SaveLocal`). For large recordings this means the whole file is buffered in memory before upload - acceptable for P3 (short clips); a streaming upload variant is a future optimization, not in scope.

## 4. UX / trigger and state machine

A recording has two stable states and the toggle action moves between them:

```
        Start (hotkey or tray click)
 IDLE ---------------------------------> RECORDING
   ^                                         |
   |     Stop (hotkey or tray click)         |
   +-----------------------------------------+
              (finalize + upload)
```

- **IDLE -> RECORDING**: call `app.StartRecording(ctx, mode)`. Update tray label to "Stop Recording". Optionally change the tray icon to a "recording" variant.
- **RECORDING -> IDLE**: call `app.StopRecording(ctx)` (finalizes + uploads in the background). Reset tray label to "Start Recording".

The mode (VideoRegion / VideoFull / GIF) is chosen at **Start** time. For P3a a single default ("Start Recording" = `VideoFull`, or a sub-menu for region vs full) is enough; the toggle only needs to remember "am I recording" not "which mode" because `Stop` takes no mode.

A single `recordToggle` closure in `main.go`'s `run()` owns the transition. The authoritative state lives in the Recorder (`Recording()`); the closure reads it to decide which branch to take, so a hotkey and a tray click can never disagree:

```go
func run(ctx context.Context, app *core.App, quit func()) error {
	cfg := app.Config()

	capture := func(mode capture.Mode) func() { /* unchanged */ }

	// recordToggle starts or stops recording based on current state.
	// trayItem is nil when there is no tray (hotkey-only path).
	recordToggle := func(mode capture.Mode, relabel func(string)) func() {
		return func() {
			if !app.RecordingSupported() {
				log.Warn().Msg("recording not supported on this build")
				return
			}
			if app.Recording() {
				if relabel != nil {
					relabel("Start Recording")
				}
				if _, err := app.StopRecording(ctx); err != nil {
					log.Error().Err(err).Msg("stop recording failed")
				}
				return
			}
			if err := app.StartRecording(ctx, mode); err != nil {
				log.Error().Err(err).Msg("start recording failed")
				return
			}
			if relabel != nil {
				relabel("Stop Recording")
			}
		}
	}
	// ... hotkey + tray wiring below ...
}
```

Hotkey wiring (add to the existing `bindings` slice, only when supported):

```go
if app.RecordingSupported() && cfg.Hotkeys.Record != "" {
	bindings = append(bindings, struct {
		id, keys string
		fn       func()
	}{"record", cfg.Hotkeys.Record, recordToggle(capture.VideoFull, nil)})
}
```

(That implies one new config field, `Hotkeys.Record`, mirroring the existing `Region`/`FullScreen`/`Window`/`Quit` keys.)

Tray wiring (add a menu item; the tray seam must expose a way to relabel an item - if `tray.MenuItem` has no SetTitle today, P3a adds a minimal `tray.Tray.SetItemTitle(id, title)` or the item is rebuilt). The item is only added when `app.RecordingSupported()`:

```go
items := []tray.MenuItem{
	{ID: "region", Title: "Capture Region", OnClick: capture(captureMode("region"))},
	{ID: "fullscreen", Title: "Capture Full Screen", OnClick: capture(captureMode("fullscreen"))},
}
if app.RecordingSupported() {
	relabel := func(title string) { /* tr.SetItemTitle("record", title) */ }
	items = append(items,
		tray.MenuItem{Separator: true},
		tray.MenuItem{ID: "record", Title: "Start Recording",
			OnClick: recordToggle(capture.VideoFull, relabel)},
	)
}
items = append(items, tray.MenuItem{Separator: true},
	tray.MenuItem{ID: "quit", Title: "Quit", OnClick: func() { quit() }})
```

Graceful shutdown: if the app receives quit while `Recording()`, `run` should call `app.StopRecording` (best-effort) before returning so the in-flight ffmpeg child is interrupted and the partial file is finalized rather than orphaned.

## 5. macOS implementation plan

Two viable backends:

| | bundled ffmpeg (`-f avfoundation`) | ScreenCaptureKit (SCK) |
|---|---|---|
| Proven | Yes - `_prototype` already records with it | No - needs new cgo/Obj-C bridge |
| Binary to ship | ~40-80 MB ffmpeg in the `.app` | none (system framework, macOS 12.3+) |
| Region capture | no native rect; needs pre-measure | native `SCContentFilter` rect |
| Encoding | libx264 / VideoToolbox via ffmpeg | VideoToolbox via `AVAssetWriter` |
| Effort | low | high (cgo, framework linking, entitlements) |
| Licensing | ffmpeg GPL/LGPL concerns (see 8) | none |

**Recommendation for P3a: bundled ffmpeg.** It is the path the prototype already validated, it lands video capture fastest, and it keeps the Recorder a thin `exec.Cmd` wrapper that mirrors known-good code. SCK is the better long-term answer (no bundled binary, native region, better perf) and is the recommended follow-up once video works, but it is not the way to *introduce* recording.

Concrete darwin Recorder (`platform/darwin/recorder.go`, `//go:build darwin`), porting `_prototype/capture/record.go`:

- **Start**:
  - Resolve ffmpeg path (next to the executable / inside the `.app` `Resources`, with a config override). If not found, the wiring should leave `Recorder: nil` rather than fail at start.
  - `VideoFull`: `ffmpeg -f avfoundation -framerate 30 -i "1:none" -an -y <tmp>.mp4` (input index `1` = main display per the prototype; should be discovered via `ffmpeg -f avfoundation -list_devices true` rather than hardcoded, since index varies by machine).
  - `VideoRegion`: there is **no rect from `screencapture -i`**. Port the prototype trick: run `screencapture -i -x /tmp/region.png`, then `sips -g pixelWidth -g pixelHeight` to read the selected size, and pass `-video_size WxH`. This gives size but **not offset** - avfoundation cannot crop to an arbitrary x/y origin from the device input alone, so P3a region recording captures a WxH area anchored at the display origin (a known limitation; a true cropped region needs a `-vf crop=w:h:x:y` filter with coordinates the overlay does not give us). Document this clearly; the proper fix rides with the SCK migration or a custom selection overlay that yields a full rect.
  - Store the `*exec.Cmd`, output path, and `recording=true` under a mutex. Run `cmd.Run()` in a goroutine (background), exactly like the prototype.
- **Audio**: **none in P3** (`-an`). Matches the prototype and avoids mic-permission prompts.
- **Output container**: **mp4 / H.264**. Widely playable, what the prototype produced, and what Nextcloud will serve via `/download`.
- **Stop = graceful interrupt + finalize**:
  - `cmd.Process.Signal(os.Interrupt)`. ffmpeg traps SIGINT, flushes, and writes the mp4 moov atom - a hard `Kill` would leave an unplayable file, so interrupt is mandatory.
  - Wait for the goroutine's `cmd.Wait()` to return (use a done channel, not the prototype's fixed `time.Sleep(1s)` - sleep races on slow finalize). Bound the wait with a timeout from `ctx`.
  - Read the finalized mp4 into `Result.Bytes`, set `Mime="video/mp4"`, `Kind=KindVideo`, set `Path` only if `SaveLocal`. Clear `recording=false`, delete the temp file unless kept.
- **Permissions**: screen recording requires the Screen Recording entitlement / TCC approval; the first run will prompt. Note in release docs.
- **Capabilities**: advertise `VideoRegion`, `VideoFull` in P3a; add `GIF` in P3b.

## 6. GIF plan

Two approaches:

- **Record-then-transcode (chosen)**: record to mp4/H.264 as in section 5, then on `Stop` run a two-pass ffmpeg palette transcode:
  - Pass 1: `ffmpeg -i in.mp4 -vf "fps=15,scale=640:-1:flags=lanczos,palettegen" palette.png`
  - Pass 2: `ffmpeg -i in.mp4 -i palette.png -lavfi "fps=15,scale=640:-1:flags=lanczos[x];[x][1:v]paletteuse" out.gif`
  - Return `Result{Mime:"image/gif", Kind:KindVideo}` (Kind stays Video; `extFor` already yields `gif` from the mime).
- **Native Go encoding** (`image/gif` + frame grabs): rejected. We would have to capture frames ourselves at a fixed fps, and Go's `image/gif` quantizer produces visibly worse output than ffmpeg's palettegen/paletteuse for screen content. More code, worse result.

**Size/fps tradeoffs** (GIF is uncompressed-ish and balloons fast):
- fps: cap at **10-15 fps**. GIF size scales roughly linearly with frame count; 30 fps GIFs are huge for no perceptual gain on UI captures.
- scale: downscale width to ~640px (`scale=640:-1`). Halving dimensions roughly quarters size.
- palette: the two-pass palettegen keeps it to 256 well-chosen colors, far smaller and cleaner than single-pass.
- duration guard: GIFs over ~15-20 s are impractical; consider a soft warning, not a hard cap, in P3b.

P3b adds `GIF` to the darwin Recorder's `Capabilities` and branches `Stop`: when the active mode is `GIF`, run the transcode before reading bytes. The mode is remembered from `Start` (the Recorder already stores per-session state).

## 7. Windows implementation plan (P3c, deferred)

High level, not built in P3a/P3b:

- Two candidate backends, mirroring macOS:
  - **`Windows.Graphics.Capture`** (WinRT, Windows 10 1803+) - the modern, no-bundle path; needs a cgo/WinRT bridge (or a helper exe), analogous to SCK on macOS.
  - **ffmpeg `gdigrab`** (`ffmpeg -f gdigrab -framerate 30 -i desktop ...`) or `ddagrab` (Desktop Duplication) - reuses the same bundled-ffmpeg Recorder shape as darwin, so most of the `exec.Cmd` logic ports directly. Region via `-offset_x/-offset_y/-video_size` on gdigrab (gdigrab *does* support offset, unlike avfoundation, so Windows region recording can be a true rect).
- Stop is the same model: signal the child to finalize. Windows `os.Interrupt` delivery to a child differs from Unix (no real SIGINT to a non-console child); the gdigrab approach typically writes `q` to ffmpeg stdin to stop gracefully instead. Note this divergence.
- Until P3c lands, the Windows build wires `Recorder: nil`. Because all consumers guard on `RecordingSupported()`, the Windows tray/hotkeys simply omit the record controls and everything else works.

## 8. Bundling concerns

The `_prototype` checked a `bin/ffmpeg` into the repo and referenced it by relative path (`filepath.Join("bin","ffmpeg")`). For a shipped `.app` that is a problem on three axes:

- **Size**: a static ffmpeg is ~40-80 MB, dwarfing the Go binary. It roughly defines the download size.
- **Licensing**: prebuilt ffmpeg binaries are commonly GPL (if built with `--enable-gpl`, e.g. x264) or LGPL otherwise. Bundling GPL ffmpeg in a distributed `.app` imposes GPL obligations (source offer, license text). An LGPL build (dynamic, no GPL-only encoders) is less restrictive but still requires attribution and the ability to relink. Either way the license text must ship and the build flavor must be a deliberate, documented choice. **Do not** vendor a random `bin/ffmpeg` blindly as the prototype did.
- **Path resolution**: relative `bin/ffmpeg` breaks when the `.app` is launched with cwd `/` (the same reason `firstExistingConfig` already probes absolute locations). The Recorder must resolve ffmpeg relative to the executable (`os.Executable()` -> `Contents/Resources/ffmpeg`), with a config override, and treat "not found" as `Recorder: nil`.

**Recommendation**: ship bundled ffmpeg for P3a to get video working, with an explicit LGPL-or-GPL decision recorded and license text in the `.app`. Treat **migrating darwin to ScreenCaptureKit** (and Windows to `Windows.Graphics.Capture`) as the strategic follow-up that removes the binary entirely - no size, no licensing, native region. The Recorder seam makes that swap invisible to the core: only `platform/darwin` changes.

## 9. Risks and phased breakdown

Risks:
- **Region offset gap (macOS)**: avfoundation gives size but not origin; P3a region recording is anchored at the display origin. Mitigation: document; full fix with SCK/overlay rect. (Windows gdigrab does not share this limit.)
- **Orphaned ffmpeg child**: a crash or ungraceful quit between Start and Stop leaves a running ffmpeg and a half-written file. Mitigation: stop-on-quit in `run`, and the moov-atom finalize requires interrupt-not-kill.
- **Stop race**: prototype's `time.Sleep(1s)` can read the file before finalize completes. Mitigation: wait on `cmd.Wait()` via a done channel with a ctx-bounded timeout.
- **Memory for large clips**: whole mp4/gif buffered into `Result.Bytes` before upload. Mitigation: fine for short P3 clips; streaming upload is a later optimization.
- **ffmpeg device index drift**: avfoundation input index is machine-specific; hardcoding `1` (as the prototype did) can grab the wrong display. Mitigation: discover via `-list_devices`.
- **Bundling/licensing**: see section 8; an undocumented GPL ffmpeg in a shipped `.app` is a legal risk.
- **Permissions**: macOS Screen Recording TCC prompt on first record; surface clearly.

Phased breakdown:

- **P3a - darwin video** (MVP): `Recorder` interface + sentinel errors; `Providers.Recorder` + `App` methods; `processResult` refactor in `pipeline.go`; `platform/darwin/recorder.go` (bundled ffmpeg, VideoFull + VideoRegion, mp4/H.264, no audio, interrupt+finalize); tray toggle + `Hotkeys.Record`; ffmpeg bundling + license text + executable-relative path resolution; stop-on-quit.
- **P3b - GIF**: add `GIF` to darwin `Capabilities`; `Stop` runs palettegen/paletteuse transcode at 10-15 fps / ~640px when mode is `GIF`; tray sub-item or mode for GIF. No core changes (`extFor`/mime already handle gif).
- **P3c - windows** (deferred): Windows Recorder via `Windows.Graphics.Capture` or ffmpeg `gdigrab` (stop via stdin `q`); wire `Recorder` non-nil on Windows; true rect region. Until then Windows ships `Recorder: nil` and the record controls are absent.

A strategic, non-blocking follow-up after P3a: migrate darwin from bundled ffmpeg to **ScreenCaptureKit** to drop the binary, gain native region, and remove the licensing burden - invisible to the core thanks to the Recorder seam.
