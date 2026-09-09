//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/platform/windows"
)

// warnSmartAppControl explains, once per install, why Windows may block
// GoShareIt and the two ways past it: turn Smart App Control off, or replace
// this copy with the Microsoft Store build (Microsoft-signed, so SAC allows
// it). The GitHub binaries are not Authenticode-signed and SAC has no per-app
// exception. This can only run in Evaluation mode (SAC does not block yet) or
// on the rare On machine that let this exact build through; in the plain
// blocked case the process never starts, which is why the installer and the
// README carry the same text. Store builds skip it entirely.
func warnSmartAppControl(chooser *windows.Chooser) {
	if windows.IsPackaged() {
		return
	}
	state, ok, err := windows.SmartAppControlState()
	if err != nil {
		log.Debug().Err(err).Msg("smart app control: state unreadable")
		return
	}
	if !ok || state == windows.SACOff {
		return
	}
	dir, err := config.Dir()
	if err != nil {
		return
	}
	marker := filepath.Join(dir, "smart-app-control-warned")
	if _, err := os.Stat(marker); err == nil {
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		return
	}
	log.Warn().Int("state", state).Msg("smart app control active: unsigned GoShareIt binaries will be blocked once it is On")
	if chooser == nil {
		return
	}

	lead := "Smart App Control is in evaluation mode on this PC. Once Windows switches it on, it will block this copy of GoShareIt and its updates."
	if state == windows.SACOn {
		lead = "Smart App Control is turned on on this PC. It will block this copy of GoShareIt's future updates, and may block this install after a reboot."
	}
	body := lead + " GoShareIt is free, open-source software and this download is not code-signed; Smart App Control has no per-app exception.\n\n" +
		"You have two options:\n" +
		"  1. Turn Smart App Control off: Windows Security > App & browser control > Smart App Control settings > Off.\n" +
		"  2. Uninstall this copy and install GoShareIt from the Microsoft Store instead. The Store build is signed by Microsoft and runs with Smart App Control on.\n\n" +
		"Yes = open Windows Security    No = open the Microsoft Store    Cancel = decide later"
	choice, err := chooser.Choose("Windows may block GoShareIt", body)
	if err != nil {
		log.Debug().Err(err).Msg("smart app control: dialog failed")
		return
	}
	// Shown once, whatever the answer; the README repeats the steps.
	if err := os.WriteFile(marker, []byte(""), 0o600); err != nil {
		log.Debug().Err(err).Msg("smart app control: could not write marker")
	}
	switch choice {
	case "Yes":
		if err := windows.OpenAppBrowserControl(); err != nil {
			log.Warn().Err(err).Msg("smart app control: could not open Windows Security")
		}
	case "No":
		if err := windows.OpenMSStore(); err != nil {
			log.Warn().Err(err).Msg("smart app control: could not open the Microsoft Store")
		}
	}
}
