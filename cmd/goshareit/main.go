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

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file (overridden by GOSHAREIT_CONFIG_PATH)")
	flag.Parse()

	cfg, err := config.Load(resolveConfigPath(*cfgPath))
	if err != nil {
		log.Fatal().Err(err).Msg("load config")
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

	capture := func(mode capture.Mode) func() {
		return func() {
			if _, err := app.RunCapture(ctx, mode); err != nil {
				log.Error().Err(err).Msg("capture failed")
			}
		}
	}

	hk := app.Hotkeys()
	if hk != nil {
		bindings := []struct {
			id, keys string
			fn       func()
		}{
			{"region", cfg.Hotkeys.Region, capture(captureMode("region"))},
			{"fullscreen", cfg.Hotkeys.FullScreen, capture(captureMode("fullscreen"))},
			{"window", cfg.Hotkeys.Window, capture(captureMode("window"))},
			{"quit", cfg.Hotkeys.Quit, quit},
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

	tr := app.Tray()
	if tr == nil {
		<-ctx.Done()
		return nil
	}
	spec := tray.MenuSpec{
		Tooltip: "GoShareIt",
		Items: []tray.MenuItem{
			{ID: "region", Title: "Capture Region", OnClick: capture(captureMode("region"))},
			{ID: "fullscreen", Title: "Capture Full Screen", OnClick: capture(captureMode("fullscreen"))},
			{Separator: true},
			{ID: "quit", Title: "Quit", OnClick: func() {
				log.Info().Msg("quit requested")
				quit()
			}},
		},
	}
	if err := tr.Run(ctx, spec); err != nil && ctx.Err() == nil {
		return err
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

// resolveConfigPath picks the config file to load. An explicit -config flag (any
// value other than the default) is honored as-is. Otherwise it searches the
// working dir, then standard user locations, so a bundled .app launched with cwd
// "/" still finds config. GOSHAREIT_CONFIG_PATH (handled in config.Load) still
// overrides everything.
func resolveConfigPath(flagVal string) string {
	if flagVal != "config.yaml" {
		return flagVal
	}
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
	return flagVal
}

func historyPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "goshareit", "history.jsonl")
}
