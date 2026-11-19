package notify

import "testing"

func TestIsWebURL(t *testing.T) {
	cases := map[string]bool{
		"https://example.com/s/abc":           true,
		"http://192.168.1.10/x.png":           true,
		"HTTPS://example.com/":                true,
		"file://attacker.example/share/x.exe": false,
		`\\attacker\share\x.exe`:              false,
		"search-ms:query=x":                   false,
		"ms-settings:privacy":                 false,
		"/Applications/Calculator.app":        false,
		"javascript:alert(1)":                 false,
		"https:///no-host":                    false,
		"":                                    false,
	}
	for in, want := range cases {
		if got := IsWebURL(in); got != want {
			t.Errorf("IsWebURL(%q) = %v, want %v", in, got, want)
		}
	}
}
