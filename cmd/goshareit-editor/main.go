//go:build darwin || windows

// Command goshareit-editor is the out-of-process annotation editor helper for
// GoShareIt. It is invoked by the menu-bar host with a captured PNG and writes
// the annotated PNG back, communicating outcome purely through the exit code:
//
//	0   confirmed: --out written with the edited PNG.
//	64  cancelled/skipped/Esc/window-closed: --out NOT written.
//	1   error (bad args, decode failure, render/encode failure).
//
// Invocation:
//
//	goshareit-editor --in <input.png> --out <output.png> \
//	    [--tool <name>] [--color <#rrggbb>] [--stroke <int>] [--tools <csv>] \
//	    [--theme light|dark|system] [--confirm-label <text>]
//
// It is build-tagged for darwin and windows because Gio needs cgo on macOS and
// a GPU backend on both; the Linux/CGO-disabled host build excludes it.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"strings"

	"gioui.org/app"
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/editor/region"
	"github.com/Rake-Pro/GoShareIt/internal/editor/ui"
)

func main() {
	in := flag.String("in", "", "path to the input PNG to annotate")
	out := flag.String("out", "", "path to write the edited PNG on confirm")
	regionMode := flag.Bool("region", false, "run the interactive screen-region selector instead of the editor")
	tool := flag.String("tool", "", "initial tool (crop|arrow|rect|ellipse|text)")
	colorHex := flag.String("color", "", "initial color as #rrggbb")
	stroke := flag.Int("stroke", 0, "initial stroke width")
	toolsCSV := flag.String("tools", "", "comma-separated tool whitelist")
	theme := flag.String("theme", "", "theme: light|dark|system (system resolves via OS detection)")
	confirmLabel := flag.String("confirm-label", "", "label rendered on the confirm button (\"\" -> Done)")
	flag.Parse()

	if *regionMode {
		runRegion(*out)
		return
	}

	if *in == "" || *out == "" {
		log.Error().Msg("--in and --out are required")
		os.Exit(1)
	}

	img, err := decodePNG(*in)
	if err != nil {
		log.Error().Err(err).Str("in", *in).Msg("decode input")
		os.Exit(1)
	}

	opts := ui.Options{
		Tool:         ui.Tool(strings.ToLower(*tool)),
		Stroke:       *stroke,
		Theme:        resolveTheme(*theme),
		ConfirmLabel: *confirmLabel,
	}
	if c, ok := parseHexColor(*colorHex); ok {
		opts.Color = c
	}
	if *toolsCSV != "" {
		for _, t := range strings.Split(*toolsCSV, ",") {
			t = strings.TrimSpace(strings.ToLower(t))
			if t != "" {
				opts.Tools = append(opts.Tools, ui.Tool(t))
			}
		}
	}

	// Gio's app.Main must own the main goroutine; the editor event loop runs on
	// a separate goroutine and terminates the process with the IPC exit code.
	go func() {
		result, confirmed, rerr := ui.Run(img, opts)
		if rerr != nil {
			log.Error().Err(rerr).Msg("editor")
			os.Exit(1)
		}
		if !confirmed || result == nil {
			os.Exit(64)
		}
		if err := encodePNG(*out, result); err != nil {
			log.Error().Err(err).Str("out", *out).Msg("encode output")
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

// runRegion runs the interactive region selector. On confirm it writes a single
// "x,y,w,h" line to outPath and exits 0; on cancel it exits 64 without writing;
// on error it logs and exits 1. Mirrors the editor's IPC exit-code contract.
func runRegion(outPath string) {
	if outPath == "" {
		log.Error().Msg("--out is required with --region")
		os.Exit(1)
	}
	go func() {
		rect, ok, rerr := region.Run()
		if rerr != nil {
			log.Error().Err(rerr).Msg("region selector")
			os.Exit(1)
		}
		if !ok {
			os.Exit(64)
		}
		line := fmt.Sprintf("%d,%d,%d,%d", rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy())
		if err := os.WriteFile(outPath, []byte(line), 0o600); err != nil {
			log.Error().Err(err).Str("out", outPath).Msg("write region")
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func encodePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// resolveTheme normalizes the --theme flag to "light" or "dark". "system"
// (or empty/unrecognized) resolves via native OS detection, implemented per
// platform in theme_darwin.go / theme_windows.go.
func resolveTheme(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	default:
		if detectSystemDark() {
			return "dark"
		}
		return "light"
	}
}

// parseHexColor parses #rrggbb (with or without leading #) into an opaque NRGBA.
func parseHexColor(s string) (color.NRGBA, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{
		R: byte(v >> 16),
		G: byte(v >> 8),
		B: byte(v),
		A: 0xff,
	}, true
}
