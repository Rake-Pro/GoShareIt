package utilities

import (
	"github.com/atotto/clipboard"
	"github.com/Rake-Pro/goshareit/logger"
)

func CopyToClipboard(text string) {
	err := clipboard.WriteAll(text)
	if err != nil {
		logger.Error("Failed to copy to clipboard: " + err.Error())
		return
	}
	logger.Debug("Copied to clipboard: " + text)
}
