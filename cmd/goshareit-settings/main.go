//go:build darwin || windows

// Command goshareit-settings is the out-of-process configuration UI (Wails v3,
// vanilla JS frontend, no node build). The tray host launches it like the
// editor helper: a sibling binary, so it never contends with the host's
// main loop. Build production binaries with -tags production.
package main

import (
	"flag"
	"net/http"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/version"
	"github.com/Rake-Pro/GoShareIt/internal/settings"
)

func main() {
	cfgFlag := flag.String("config", "", "config file to edit (default: standard location)")
	flag.Parse()

	path := *cfgFlag
	if env := os.Getenv(config.EnvConfigPath); env != "" {
		path = env
	}
	if path == "" {
		var err error
		if path, err = config.DefaultConfigPath(); err != nil {
			log.Fatal().Err(err).Msg("resolve config path")
		}
	}

	svc := &settings.Service{ConfigPath: path, Version: version.Version, Packaged: isPackaged()}
	app := application.New(application.Options{
		Name:        "GoShareIt Settings",
		Description: "GoShareIt configuration",
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: http.FileServer(http.FS(assets()))},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	// Native dialog/browser hooks hang off the app managers; no startup
	// context capture is needed in v3.
	svc.PickDir = func() (string, error) {
		return app.Dialog.OpenFile().
			SetTitle("Choose folder").
			CanChooseDirectories(true).
			CanChooseFiles(false).
			CanCreateDirectories(true).
			PromptForSingleSelection()
	}
	svc.OpenURL = app.Browser.OpenURL
	svc.Close = app.Quit

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "GoShareIt Settings",
		Width:  760,
		Height: 820,
		URL:    "/",
	})
	if err := app.Run(); err != nil {
		log.Fatal().Err(err).Msg("settings ui")
	}
	// Tell the host whether anything was saved: it restarts to apply only on
	// ExitSaved, so closing without saving discards cleanly.
	if svc.DidSave() {
		os.Exit(settings.ExitSaved)
	}
}
