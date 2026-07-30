// Command goshareit is the OS-agnostic entry point. Per-GOOS wire_<goos>.go
// files supply buildProviders, which constructs the OS-specific seams.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/edit"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
	"github.com/Rake-Pro/GoShareIt/internal/core/region"
	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
	"github.com/Rake-Pro/GoShareIt/internal/core/update"
	"github.com/Rake-Pro/GoShareIt/internal/core/version"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file (overridden by GOSHAREIT_CONFIG_PATH)")
	flag.Parse()

	setupFileLog()

	cfgFile, didSetup, secretPath, err := acquireConfig(*cfgPath)
	if err != nil {
		log.Fatal().Err(err).Msg("resolve config")
	}
	if didSetup {
		log.Info().Str("config", cfgFile).Str("secret", secretPath).Msg("first run - scaffolded config")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		// Onboarding instead of a silent death: a fresh install always lands
		// here (empty password file), and on windowsgui builds stderr is
		// invisible, so a fatal would look like the app simply not starting.
		log.Warn().Err(err).Str("config", cfgFile).Msg("config not usable - opening settings UI")
		if serr := runSettingsBlocking(context.Background(), cfgFile); serr != nil {
			log.Fatal().Err(serr).Str("config", cfgFile).Msg("config invalid and settings UI unavailable - edit the config manually")
		}
		if cfg, err = config.Load(cfgFile); err != nil {
			log.Fatal().Err(err).Str("config", cfgFile).Msg("config still invalid after setup - exiting")
		}
	}

	level, _ := zerolog.ParseLevel(cfg.Logging.Level)
	if level == zerolog.NoLevel {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	logger := log.Logger

	hist, err := history.New(historyPath())
	if err != nil {
		logger.Fatal().Err(err).Msg("open history")
	}

	providers, err := buildProviders(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("build providers")
	}
	// The uploader is portable; wire it from config here regardless of GOOS.
	uploader, err := buildUploader(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("build uploader")
	}
	providers.Uploader = uploader

	// The editor launcher is portable (CGO-free); the GUI it spawns lives in a
	// separate goshareit-editor binary. It is only ever invoked when
	// cfg.Editor.Enabled, so a missing editor binary is harmless when disabled.
	providers.Editor = edit.Launcher{
		HelperPath:   cfg.Editor.HelperPath,
		Timeout:      time.Duration(cfg.Editor.TimeoutSeconds) * time.Second,
		Tool:         cfg.Editor.DefaultTool,
		Color:        cfg.Editor.Color,
		StrokeWidth:  cfg.Editor.StrokeWidth,
		Tools:        cfg.Editor.Tools,
		Theme:        cfg.Theme,
		ConfirmLabel: composeConfirmLabel(cfg),
	}

	app, err := core.New(cfg, providers, logger, hist)
	if err != nil {
		logger.Fatal().Err(err).Msg("init app")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var updates *updateController
	if cfg.UpdateEnabled() {
		upd, err := update.New(update.Config{
			Repo:    cfg.Update.Repo,
			Token:   cfg.UpdateToken(),
			Current: version.Version,
		})
		if err != nil {
			logger.Warn().Err(err).Msg("updater disabled")
		} else {
			updates = newUpdateController(upd, app, time.Duration(cfg.Update.IntervalHours)*time.Hour, cancel)
		}
	}

	settingsL := &settingsLauncher{configPath: cfgFile, app: app, quit: cancel}

	if err := run(ctx, app, updates, settingsL, cancel); err != nil {
		logger.Fatal().Err(err).Msg("run")
	}
}

// run wires hotkeys and the tray, then blocks until ctx is cancelled. It is
// portable: the tray runs on the main goroutine (required by some OSes) while
// the hotkey manager runs alongside.
func run(ctx context.Context, app *core.App, updates *updateController, settingsL *settingsLauncher, quit func()) error {
	cfg := app.Config()
	tr := app.Tray()

	runShot := func(mode capture.Mode) func() {
		return func() {
			if _, err := app.RunCapture(ctx, mode); err != nil {
				log.Error().Err(err).Msg("capture failed")
			}
		}
	}

	// label appends the configured hotkey to a menu title, e.g.
	// "Capture Region  (Cmd+Shift+1)", so the menu documents its own shortcuts.
	label := func(title, keys string) string {
		if keys == "" {
			return title
		}
		return title + "  (" + keys + ")"
	}

	notifyUser := func(title, body string) {
		if n := app.Notifier(); n != nil {
			if err := n.Notify(notify.Notification{Title: title, Body: body}); err != nil {
				log.Debug().Err(err).Msg("notification failed")
			}
		}
	}

	// Recording uses two separate menu items (Start / Stop); the inapplicable one
	// is greyed out. updateRecordItems reflects the current state onto the tray
	// (no-op when there is no tray). Start and Stop guard on app.Recording() so a
	// hotkey and a tray click can never disagree.
	updateRecordItems := func(recording bool) {
		if tr != nil {
			// While recording, all Start items grey out and Stop enables; the
			// Stop title gains a record marker so the active state is visible
			// at a glance.
			tr.SetItemEnabled("record-start", !recording)
			tr.SetItemEnabled("record-region", !recording)
			tr.SetItemEnabled("record-gif", !recording)
			tr.SetItemEnabled("record-stop", recording)
			stopTitle := label("Stop Recording", cfg.Hotkeys.Record)
			if recording {
				stopTitle = "● " + stopTitle
			}
			tr.SetItemTitle("record-stop", stopTitle)
		}
	}
	recDesc := func(mode capture.Mode) string {
		switch mode {
		case capture.VideoRegion:
			return "region recording"
		case capture.GIF:
			return "GIF recording"
		default:
			return "full-screen recording"
		}
	}
	recordStopHint := func() string {
		if alts := splitHotkeys(cfg.Hotkeys.Record); len(alts) > 0 {
			return "Press " + alts[0] + " or use the tray menu to stop."
		}
		return "Use the tray menu to stop."
	}
	// recordingStarted is the one place recording start becomes observable:
	// an info log carrying the trigger (hotkey vs tray) so sessions are
	// attributable from the log, plus a desktop notification - a screen
	// recording must never start silently.
	recordingStarted := func(mode capture.Mode, trigger string) {
		log.Info().Str("mode", mode.String()).Str("trigger", trigger).Msg("recording started")
		notifyUser("Recording started", "Started "+recDesc(mode)+". "+recordStopHint())
		updateRecordItems(true)
	}
	startRec := func(mode capture.Mode, trigger string) func() {
		return func() {
			if !app.RecordingSupported() || app.Recording() {
				return
			}
			if err := app.StartRecording(ctx, mode, image.Rectangle{}); err != nil {
				log.Error().Err(err).Str("mode", mode.String()).Str("trigger", trigger).Msg("start recording failed")
				return
			}
			recordingStarted(mode, trigger)
		}
	}
	// regionSel runs the out-of-process overlay to pick a screen rectangle. It
	// reuses the goshareit-editor helper (same binary, --region flag).
	regionSel := region.Launcher{HelperPath: cfg.Editor.HelperPath}
	startRegionRec := func() {
		if !app.RecordingSupported() || app.Recording() {
			return
		}
		// The overlay blocks on its own process, so run it off the tray callback
		// goroutine; recording only starts after the user confirms a rectangle.
		go func() {
			rect, ok, err := regionSel.Select(ctx)
			if err != nil {
				log.Error().Err(err).Msg("region select failed")
				return
			}
			if !ok || app.Recording() {
				return
			}
			if err := app.StartRecording(ctx, capture.VideoRegion, rect); err != nil {
				log.Error().Err(err).Msg("start region recording failed")
				return
			}
			recordingStarted(capture.VideoRegion, "tray")
		}()
	}
	stopRec := func(trigger string) func() {
		return func() {
			if !app.Recording() {
				return
			}
			if _, err := app.StopRecording(ctx); err != nil {
				log.Error().Err(err).Str("trigger", trigger).Msg("stop recording failed")
			} else {
				log.Info().Str("trigger", trigger).Msg("recording stopped")
			}
			updateRecordItems(false)
		}
	}
	recordToggle := func() {
		if !app.RecordingSupported() {
			log.Warn().Msg("recording not supported on this build")
			return
		}
		if app.Recording() {
			stopRec("hotkey")()
		} else {
			startRec(capture.VideoFull, "hotkey")()
		}
	}

	runShotEdit := func(mode capture.Mode) func() {
		return func() {
			if _, err := app.RunCaptureEdit(ctx, mode); err != nil {
				log.Error().Err(err).Msg("capture (edit) failed")
			}
		}
	}

	uploadItemTitle := func() string {
		state := "Off"
		if app.UploadEnabled() {
			state = "On"
		}
		return label("Uploads: "+state, cfg.Hotkeys.UploadToggle)
	}
	// uploadToggle flips the live switch, refuses to enable without a usable
	// server config, and persists the state so it survives restart.
	uploadToggle := func() {
		enable := !app.UploadEnabled()
		if enable && !app.UploadConfigured() {
			notifyUser("Uploads unavailable", "No server configured - set it up in Settings first.")
			return
		}
		app.SetUploadEnabled(enable)
		if tr != nil {
			tr.SetItemTitle("upload-toggle", uploadItemTitle())
		}
		state := "disabled - captures stay on this machine"
		if enable {
			state = "enabled"
		}
		notifyUser("Uploads "+map[bool]string{true: "on", false: "off"}[enable], "Uploads "+state+".")
		if settingsL != nil {
			go func() {
				if err := config.SetUploadEnabledFile(settingsL.configPath, enable); err != nil {
					log.Warn().Err(err).Msg("persist upload toggle failed (state applies until restart)")
				}
			}()
		}
	}

	hk := app.Hotkeys()
	if hk != nil {
		bindings := []struct {
			id, keys string
			fn       func()
		}{
			{"region", cfg.Hotkeys.Region, runShot(captureMode("region"))},
			{"fullscreen", cfg.Hotkeys.FullScreen, runShot(captureMode("fullscreen"))},
			{"window", cfg.Hotkeys.Window, runShot(captureMode("window"))},
			{"region-edit", cfg.Hotkeys.RegionEdit, runShotEdit(captureMode("region"))},
			{"fullscreen-edit", cfg.Hotkeys.FullScreenEdit, runShotEdit(captureMode("fullscreen"))},
			{"window-edit", cfg.Hotkeys.WindowEdit, runShotEdit(captureMode("window"))},
			{"upload-toggle", cfg.Hotkeys.UploadToggle, uploadToggle},
			{"quit", cfg.Hotkeys.Quit, quit},
		}
		if app.RecordingSupported() && cfg.Hotkeys.Record != "" {
			bindings = append(bindings, struct {
				id, keys string
				fn       func()
			}{"record", cfg.Hotkeys.Record, recordToggle})
		}
		for _, b := range bindings {
			// Each value may hold comma-separated alternatives; register each
			// under a suffixed id so they bind independently.
			for i, keys := range splitHotkeys(b.keys) {
				id := b.id
				if i > 0 {
					id = fmt.Sprintf("%s#%d", b.id, i+1)
				}
				if err := hk.Register(id, keys, b.fn); err != nil {
					log.Warn().Err(err).Str("id", id).Str("keys", keys).Msg("register hotkey")
				}
			}
		}
		go func() {
			if err := hk.Run(ctx); err != nil && ctx.Err() == nil {
				log.Error().Err(err).Msg("hotkey manager stopped")
			}
		}()
	}

	if tr == nil {
		<-ctx.Done()
		return nil
	}
	items := []tray.MenuItem{
		{ID: "region", Title: label("Capture Region", cfg.Hotkeys.Region), OnClick: runShot(captureMode("region"))},
		{ID: "fullscreen", Title: label("Capture Full Screen", cfg.Hotkeys.FullScreen), OnClick: runShot(captureMode("fullscreen"))},
	}
	// Recording: separate Start (video), Start GIF, and a shared Stop. Stop starts
	// greyed out; while recording, both Start items grey and Stop enables. Each
	// Start item appears only if its mode is supported on this build.
	if app.RecordingSupported() {
		items = append(items, tray.MenuItem{Separator: true})
		if app.RecordingModeSupported(capture.VideoFull) {
			items = append(items, tray.MenuItem{ID: "record-start", Title: label("Start Recording", cfg.Hotkeys.Record), OnClick: startRec(capture.VideoFull, "tray")})
		}
		if app.RecordingModeSupported(capture.VideoRegion) || app.RecordingModeSupported(capture.VideoFull) {
			items = append(items, tray.MenuItem{ID: "record-region", Title: "Start Region Recording", OnClick: startRegionRec})
		}
		if app.RecordingModeSupported(capture.GIF) {
			items = append(items, tray.MenuItem{ID: "record-gif", Title: "Start GIF Recording", OnClick: startRec(capture.GIF, "tray")})
		}
		items = append(items,
			tray.MenuItem{ID: "record-stop", Title: label("Stop Recording", cfg.Hotkeys.Record), OnClick: stopRec("tray"), Disabled: true},
		)
	}
	items = append(items,
		tray.MenuItem{Separator: true},
		tray.MenuItem{ID: "upload-toggle", Title: uploadItemTitle(), OnClick: uploadToggle},
	)
	if settingsL != nil {
		items = append(items, tray.MenuItem{ID: "settings", Title: "Settings...", OnClick: func() {
			go settingsL.open(ctx)
		}})
	}
	if updates != nil {
		items = append(items, updates.menuItem(ctx))
		updates.start(ctx)
	}
	items = append(items,
		tray.MenuItem{Separator: true},
		tray.MenuItem{ID: "quit", Title: label("Quit", cfg.Hotkeys.Quit), OnClick: func() {
			log.Info().Msg("quit requested")
			quit()
		}},
	)
	spec := tray.MenuSpec{Tooltip: "GoShareIt", Icon: trayIcon(), Items: items}
	if err := tr.Run(ctx, spec); err != nil && ctx.Err() == nil {
		return err
	}
	// Best-effort: finalize an in-flight recording on shutdown so the child
	// process is interrupted and the partial file is not orphaned.
	if app.Recording() {
		if _, err := app.StopRecording(context.Background()); err != nil {
			log.Warn().Err(err).Msg("stop recording on shutdown failed")
		}
	}
	return nil
}

// setupFileLog mirrors logs into <app root>/goshareit.log. windowsgui builds
// have no visible stderr and .app bundles hide it too, so without this,
// startup failures on real installs are undiagnosable. Truncates at 5MB.
func setupFileLog() {
	dir, err := config.Dir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	path := filepath.Join(dir, "goshareit.log")
	flags := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	if fi, err := os.Stat(path); err == nil && fi.Size() > 5<<20 {
		flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return
	}
	log.Logger = log.Output(zerolog.MultiLevelWriter(zerolog.ConsoleWriter{Out: os.Stderr}, f))
}

// splitHotkeys splits a comma-separated alternatives string into individual
// chord strings; empty entries (and an empty value) yield nothing.
func splitHotkeys(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func captureMode(s string) capture.Mode {
	switch s {
	case "fullscreen":
		return capture.FullScreen
	case "window":
		return capture.ActiveWindow
	default:
		return capture.RegionInteractive
	}
}

// acquireConfig determines the config file to load. Precedence:
// GOSHAREIT_CONFIG_PATH, an explicit -config flag, then the first existing file
// among the standard locations. If none exist (first run) it writes a starter
// config + empty password file and returns didSetup=true so the caller can guide
// the user instead of failing - the app must never fatal merely because it is
// unconfigured.
func acquireConfig(flagVal string) (path string, didSetup bool, secretPath string, err error) {
	if env := os.Getenv(config.EnvConfigPath); env != "" {
		return env, false, "", nil
	}
	if flagVal != "config.yaml" {
		return flagVal, false, "", nil
	}
	if p := firstExistingConfig(); p != "" {
		return p, false, "", nil
	}
	def, err := config.DefaultConfigPath()
	if err != nil {
		return "", false, "", err
	}
	secretPath, err = config.WriteStarter(def)
	if err != nil {
		return "", false, "", err
	}
	return def, true, secretPath, nil
}

// firstExistingConfig returns the first config file that exists among the working
// dir and standard user locations, or "" if none. This lets a bundled .app
// launched with cwd "/" still find a user config. The app root (~/.goshareit,
// ~/goshareit on Windows) wins; the pre-v1.1 locations stay as read fallbacks so
// existing installs keep working.
func firstExistingConfig() string {
	candidates := []string{"config.yaml"}
	if dir, err := config.Dir(); err == nil {
		candidates = append(candidates, filepath.Join(dir, "config.yaml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".config", "goshareit", "config.yaml"),
			filepath.Join(home, "Library", "Application Support", "GoShareIt", "config.yaml"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func historyPath() string {
	dir, err := config.Dir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "history.jsonl")
}
