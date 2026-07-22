package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Nextcloud Login Flow v2. The browser session performs the actual
// authentication - password, OIDC/SSO, 2FA, whatever the server is configured
// for - and the app receives a dedicated app password at the end. This is the
// "log in with the browser" path; the manual app-password field stays as the
// fallback.

type loginFlowStart struct {
	Poll struct {
		Token    string `json:"token"`
		Endpoint string `json:"endpoint"`
	} `json:"poll"`
	Login string `json:"login"`
}

type loginFlowResult struct {
	Server      string `json:"server"`
	LoginName   string `json:"loginName"`
	AppPassword string `json:"appPassword"`
}

// startLoginFlow registers a new login-flow session and returns the browser
// URL plus the poll credentials.
func startLoginFlow(ctx context.Context, client *http.Client, baseURL string) (*loginFlowStart, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+"/index.php/login/v2", nil)
	if err != nil {
		return nil, fmt.Errorf("login: request: %w", err)
	}
	req.Header.Set("User-Agent", "GoShareIt")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login: start flow: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("login: start flow: unexpected status %s", resp.Status)
	}
	var start loginFlowStart
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&start); err != nil {
		return nil, fmt.Errorf("login: decode start: %w", err)
	}
	if start.Login == "" || start.Poll.Token == "" || start.Poll.Endpoint == "" {
		return nil, fmt.Errorf("login: server returned an incomplete flow response")
	}
	return &start, nil
}

// pollLoginFlow polls until the user completes the browser login (404 while
// pending, 200 with credentials when done) or ctx expires.
func pollLoginFlow(ctx context.Context, client *http.Client, start *loginFlowStart, interval time.Duration) (*loginFlowResult, error) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("login: timed out waiting for the browser sign-in")
		case <-tick.C:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, start.Poll.Endpoint,
			strings.NewReader(url.Values{"token": {start.Poll.Token}}.Encode()))
		if err != nil {
			return nil, fmt.Errorf("login: poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "GoShareIt")
		resp, err := client.Do(req)
		if err != nil {
			// Transient network errors keep polling until the deadline.
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}
		var result loginFlowResult
		derr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
		resp.Body.Close()
		if derr != nil {
			return nil, fmt.Errorf("login: decode result: %w", derr)
		}
		if result.AppPassword == "" {
			return nil, fmt.Errorf("login: server returned no app password")
		}
		return &result, nil
	}
}

// osOpenURL opens url in the default browser; the GUI shell can override via
// Service.OpenURL for a native implementation.
func osOpenURL(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("login: open browser: %w", err)
	}
	return cmd.Process.Release()
}
