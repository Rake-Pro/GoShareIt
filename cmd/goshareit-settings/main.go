//go:build darwin || windows

// Command goshareit-settings is the out-of-process configuration UI (Wails v2,
// vanilla JS frontend, no node build). The tray host launches it like the
// editor helper: a sibling binary, so it never contends with the host's
// systray main loop. Build production binaries with -tags desktop,production.
package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

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

	svc := &settings.Service{ConfigPath: path, Version: version.Version}
	err := wails.Run(&options.App{
		Title:  "GoShareIt Settings",
		Width:  760,
		Height: 820,
		AssetServer: &assetserver.Options{
			Assets: assets(),
		},
		Bind: []interface{}{svc},
	})
	if err != nil {
		log.Fatal().Err(err).Msg("settings ui")
	}
}
