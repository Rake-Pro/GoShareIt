package capture

import (
	"os"
	"os/exec"

	"github.com/Rake-Pro/goshareit/logger"
)

func TakeScreenshot(path string, interactive bool) error {
	args := []string{"-x", path}
	if interactive {
		args = []string{"-i", "-x", path}
	}

	logger.Info("Starting screenshot: " + path)
	cmd := exec.Command("screencapture", args...)
	err := cmd.Run()
	if err != nil {
		logger.Error("Screenshot command failed: " + err.Error())
		return err
	}

	// Check if screenshot file was created, if it wasn't then just kill it here. User probably cancelled.
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		logger.Warn("Screenshot cancelled or file not created: " + path)
		return err
	}

	logger.Info("Screenshot saved: " + path)
	return nil
}
