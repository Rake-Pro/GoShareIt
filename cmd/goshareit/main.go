// Command goshareit is the OS-agnostic entry point. Per-GOOS wire_<goos>.go
// files supply buildProviders, which constructs the OS-specific seams.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/edit"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file (overridden by GOSHAREIT_CONFIG_PATH)")
	flag.Parse()

	cfgFile, didSetup, secretPath, err := acquireConfig(*cfgPath)
	if err != nil {
		log.Fatal().Err(err).Msg("resolve config")
	}
	if didSetup {
		log.Info().
			Str("config", cfgFile).
			Str("secret", secretPath).
			Msg("first-run setup complete - add your Nextcloud app password to the secret file, review base_url/username in the config, then relaunch")
		return
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Fatal().Err(err).Str("config", cfgFile).Msg("load config")
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
	providers.Uploader = upload.NewNextcloud(upload.NextcloudConfig{
		BaseURL:         cfg.Nextcloud.BaseURL,
		Username:        cfg.Nextcloud.Username,
		DavUser:         cfg.Nextcloud.DavUser,
		Password:        cfg.Password(),
		RemoteDir:       cfg.Nextcloud.RemoteDir,
		ShareExpireDays: cfg.Upload.ShareExpireDays,
		SharePassword:   cfg.Upload.SharePassword,
	}, nil)

	// The editor launcher is portable (CGO-free); the GUI it spawns lives in a
	// separate goshareit-editor binary. It is only ever invoked when
	// cfg.Editor.Enabled, so a missing editor binary is harmless when disabled.
	providers.Editor = edit.Launcher{
		HelperPath:  cfg.Editor.HelperPath,
		Timeout:     time.Duration(cfg.Editor.TimeoutSeconds) * time.Second,
		Tool:        cfg.Editor.DefaultTool,
		Color:       cfg.Editor.Color,
		StrokeWidth: cfg.Editor.StrokeWidth,
		Tools:       cfg.Editor.Tools,
	}

	app, err := core.New(cfg, providers, logger, hist)
	if err != nil {
		logger.Fatal().Err(err).Msg("init app")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := run(ctx, app, cancel); err != nil {
		logger.Fatal().Err(err).Msg("run")
	}
}

// run wires hotkeys and the tray, then blocks until ctx is cancelled. It is
// portable: the tray runs on the main goroutine (required by some OSes) while
// the hotkey manager runs alongside.
func run(ctx context.Context, app *core.App, quit func()) error {
	cfg := app.Config()
	tr := app.Tray()

	runShot := func(mode capture.Mode) func() {
		return func() {
			if _, err := app.RunCapture(ctx, mode); err != nil {
				log.Error().Err(err).Msg("capture failed")
			}
		}
	}

	// Recording uses two separate menu items (Start / Stop); the inapplicable one
	// is greyed out. updateRecordItems reflects the current state onto the tray
	// (no-op when there is no tray). Start and Stop guard on app.Recording() so a
	// hotkey and a tray click can never disagree.
	updateRecordItems := func(recording bool) {
		if tr != nil {
			// While recording, both Start items grey out and Stop enables.
			tr.SetItemEnabled("record-start", !recording)
			tr.SetItemEnabled("record-gif", !recording)
			tr.SetItemEnabled("record-stop", recording)
		}
	}
	startRec := func(mode capture.Mode) func() {
		return func() {
			if !app.RecordingSupported() || app.Recording() {
				return
			}
			if err := app.StartRecording(ctx, mode); err != nil {
				log.Error().Err(err).Str("mode", mode.String()).Msg("start recording failed")
				return
			}
			updateRecordItems(true)
		}
	}
	stopRec := func() {
		if !app.Recording() {
			return
		}
		if _, err := app.StopRecording(ctx); err != nil {
			log.Error().Err(err).Msg("stop recording failed")
		}
		updateRecordItems(false)
	}
	recordToggle := func() {
		if !app.RecordingSupported() {
			log.Warn().Msg("recording not supported on this build")
			return
		}
		if app.Recording() {
			stopRec()
		} else {
			startRec(capture.VideoFull)()
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

	hk := app.Hotkeys()
	if hk != nil {
		bindings := []struct {
			id, keys string
			fn       func()
		}{
			{"region", cfg.Hotkeys.Region, runShot(captureMode("region"))},
			{"fullscreen", cfg.Hotkeys.FullScreen, runShot(captureMode("fullscreen"))},
			{"window", cfg.Hotkeys.Window, runShot(captureMode("window"))},
			{"quit", cfg.Hotkeys.Quit, quit},
		}
		if app.RecordingSupported() && cfg.Hotkeys.Record != "" {
			bindings = append(bindings, struct {
				id, keys string
				fn       func()
			}{"record", cfg.Hotkeys.Record, recordToggle})
		}
		for _, b := range bindings {
			if b.keys == "" {
				continue
			}
			if err := hk.Register(b.id, b.keys, b.fn); err != nil {
				log.Warn().Err(err).Str("id", b.id).Msg("register hotkey")
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
			items = append(items, tray.MenuItem{ID: "record-start", Title: label("Start Recording", cfg.Hotkeys.Record), OnClick: startRec(capture.VideoFull)})
		}
		if app.RecordingModeSupported(capture.GIF) {
			items = append(items, tray.MenuItem{ID: "record-gif", Title: "Start GIF Recording", OnClick: startRec(capture.GIF)})
		}
		items = append(items,
			tray.MenuItem{ID: "record-stop", Title: label("Stop Recording", cfg.Hotkeys.Record), OnClick: stopRec, Disabled: true},
		)
	}
	items = append(items,
		tray.MenuItem{Separator: true},
		tray.MenuItem{ID: "quit", Title: label("Quit", cfg.Hotkeys.Quit), OnClick: func() {
			log.Info().Msg("quit requested")
			quit()
		}},
	)
	spec := tray.MenuSpec{Tooltip: "GoShareIt", Items: items}
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
// launched with cwd "/" still find a user config.
func firstExistingConfig() string {
	candidates := []string{"config.yaml"}
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
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "goshareit", "history.jsonl")
}
