package utilities

import (
	"github.com/Rake-Pro/goshareit/logger"
	"github.com/atotto/clipboard"
)

func CopyToClipboard(text string) {
	err := clipboard.WriteAll(text)
	if err != nil {
		logger.Error("Failed to copy to clipboard: " + err.Error())
		return
	}
	logger.Debug("Copied to clipboard: " + text)
}
