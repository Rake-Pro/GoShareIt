//go:build darwin || windows

package wailsapp

import (
	"fmt"
	"strconv"
	"strings"
)

// toAccelerator converts one GoShareIt chord ("Cmd+Shift+1", "Ctrl+PrintScreen")
// into a Wails accelerator ("cmd+shift+1"). The chord grammar is unchanged from
// the golang.design/x/hotkey backends: tokens are separated by "+", modifier
// spelling is per-OS (see modifierAccel), the key may appear anywhere in the
// chord, and "+" itself is unaddressable because it is the separator.
//
// Wails requires the key last, so the tokens are reordered here.
func toAccelerator(chord string) (string, error) {
	var mods []string
	seen := map[string]bool{}
	key := ""
	for _, raw := range strings.Split(chord, "+") {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			continue
		}
		if mod, ok := modifierAccel(token); ok {
			if !seen[mod] {
				seen[mod] = true
				mods = append(mods, mod)
			}
			continue
		}
		k, err := keyAccel(token)
		if err != nil {
			return "", err
		}
		if key != "" {
			return "", fmt.Errorf("multiple non-modifier keys")
		}
		key = k
	}
	if key == "" {
		return "", fmt.Errorf("no key specified")
	}
	return strings.Join(append(mods, key), "+"), nil
}

// keyAliases maps the spelled-out punctuation names the chord grammar accepts
// onto the single characters Wails parses.
var keyAliases = map[string]string{
	"grave":    "`",
	"backtick": "`",
	"minus":    "-",
	"equals":   "=",
}

// commonKeyAccel resolves the key tokens both OSes accept: digits, letters,
// punctuation (US layout), "space" and f1..f20. The set matches what the
// previous backends bound, and every member is in the Wails per-OS key table.
func commonKeyAccel(token string) (string, error) {
	if alias, ok := keyAliases[token]; ok {
		return alias, nil
	}
	if token == "space" {
		return token, nil
	}
	if len(token) == 1 {
		return token, nil
	}
	if token[0] == 'f' {
		if n, err := strconv.Atoi(token[1:]); err == nil && n >= 1 && n <= 20 {
			return token, nil
		}
	}
	return "", fmt.Errorf("unknown token %q", token)
}
