//go:build windows

package windows

import (
	"fmt"
	"os/exec"
	"strings"
)

// Confirmer shows blocking native dialogs via PowerShell's WPF MessageBox,
// the Windows analog of darwin's osascript `display dialog`.
//
// Limitation: System.Windows.MessageBox only offers fixed button sets (no
// custom labels), so okLabel/cancelLabel are advisory here - the dialog
// always shows the standard Yes/No buttons.
type Confirmer struct{}

// NewConfirmer returns a Windows confirm-dialog provider.
func NewConfirmer() *Confirmer { return &Confirmer{} }

// Confirm shows a blocking Yes/No dialog and reports whether the user chose
// Yes. It blocks until the user answers.
func (c *Confirmer) Confirm(title, body, _, _ string) (bool, error) {
	script := buildConfirmScript(title, body)
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", script,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("windows confirm: powershell failed: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out)) == "Yes", nil
}

// buildConfirmScript returns a PowerShell snippet that shows a WPF MessageBox
// with the given title and body and writes the result ("Yes"/"No") to stdout.
func buildConfirmScript(title, body string) string {
	return strings.Join([]string{
		`Add-Type -AssemblyName PresentationFramework`,
		`$result = [System.Windows.MessageBox]::Show('` + psEscape(body) + `', '` + psEscape(title) + `', 'YesNo', 'Question')`,
		`Write-Output $result`,
	}, "; ")
}
