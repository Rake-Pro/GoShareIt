//go:build windows

package windows

import (
	"fmt"
	"os/exec"
	"strings"
)

// Chooser shows a blocking three-button dialog via PowerShell's WPF
// MessageBox. The generic two-button Confirmer seam moved to platform/wailsapp
// with the rest of the UI, but the Wails dialog API renders a question dialog
// on Windows as a Win32 MessageBox with a fixed Yes/No button set and cannot
// express Yes/No/Cancel - which the Smart App Control notice needs, because it
// offers three answers. This is the only remaining PowerShell dialog and it is
// shown at most once per install.
type Chooser struct{}

// NewChooser returns a Windows three-button dialog provider.
func NewChooser() *Chooser { return &Chooser{} }

// Choose shows a blocking Yes/No/Cancel dialog and returns "Yes", "No" or
// "Cancel". Callers spell out in body what each button does, since the labels
// are fixed.
func (c *Chooser) Choose(title, body string) (string, error) {
	script := buildChoiceScript(title, body)
	cmd := noConsole(exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", script,
	))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("windows choose: powershell failed: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func buildChoiceScript(title, body string) string {
	return strings.Join([]string{
		`Add-Type -AssemblyName PresentationFramework`,
		`$result = [System.Windows.MessageBox]::Show('` + psEscape(body) + `', '` + psEscape(title) + `', 'YesNoCancel', 'Warning')`,
		`Write-Output $result`,
	}, "; ")
}

// psEscape escapes a string for embedding inside a PowerShell single-quoted
// literal (only the single quote needs doubling).
func psEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
