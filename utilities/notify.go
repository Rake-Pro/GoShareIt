package utilities

import (
	"fmt"
	"github.com/rake8288/goshareit/logger"
	"os/exec"
	"strings"
)

func Notify(title, message string) {
	script := fmt.Sprintf(`tell application "System Events" to display dialog "%s" with title "%s" buttons {"OK"} default button "OK"`,
		escape(message), escape(title))

	cmd := exec.Command("osascript", "-e", script)

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to show macOS notification dialog: " + err.Error())
	}
}

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
