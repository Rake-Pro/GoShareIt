package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Job is the hand-off from the host to the out-of-process updater
// (goshareit-editor --update <file>): everything the updater needs to wait
// for the host to exit, fetch and verify the release, install it and start
// the new host again. It lives in a small JSON file the updater deletes when
// done.
type Job struct {
	Version    string   `json:"version"`           // release the host found, e.g. "1.4.0"
	Repo       string   `json:"repo"`              // "owner/name"
	APIBaseURL string   `json:"api_base"`          // "" = GitHub
	Current    string   `json:"current"`           // host version, for the newer-than check
	Channel    string   `json:"channel,omitempty"` // "stable" | "dev"; "" (old job files) = stable
	HostPID    int      `json:"host_pid"`          // process to wait for before touching files
	Relaunch   string   `json:"relaunch"`          // SelfLaunchPath of the host (exe, or .app on darwin)
	HostExe    string   `json:"host_exe"`          // the host binary; the install happens next to it
	Args       []string `json:"args"`              // host args to forward on relaunch (e.g. --config)
	Theme      string   `json:"theme"`             // "light" | "dark" | "" (system) for the updater window
}

// WriteJob stores j as JSON in a fresh temp file and returns its path.
func WriteJob(j Job) (string, error) {
	f, err := os.CreateTemp("", "goshareit-update-*.json")
	if err != nil {
		return "", fmt.Errorf("update: job file: %w", err)
	}
	if err := json.NewEncoder(f).Encode(j); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("update: write job: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("update: write job: %w", err)
	}
	return f.Name(), nil
}

// ReadJob loads a job file written by WriteJob.
func ReadJob(path string) (Job, error) {
	var j Job
	b, err := os.ReadFile(path)
	if err != nil {
		return j, fmt.Errorf("update: read job: %w", err)
	}
	if err := json.Unmarshal(b, &j); err != nil {
		return j, fmt.Errorf("update: parse job %s: %w", filepath.Base(path), err)
	}
	if j.Version == "" || j.Repo == "" || j.Relaunch == "" {
		return j, fmt.Errorf("update: job %s is incomplete", filepath.Base(path))
	}
	return j, nil
}

// updaterOldFiles are the only "<file>.old" names applyFlat creates: the
// release archives carry exactly these executables.
var updaterOldFiles = map[string]bool{
	"goshareit.old": true, "goshareit-editor.old": true, "goshareit-settings.old": true,
	"goshareit.exe.old": true, "goshareit-editor.exe.old": true, "goshareit-settings.exe.old": true,
}

// isStageDir matches the names os.MkdirTemp gives the ".goshareit-update-"
// stage directories: the prefix plus a decimal random suffix.
func isStageDir(name string) bool {
	rest, ok := strings.CutPrefix(name, ".goshareit-update-")
	if !ok || rest == "" {
		return false
	}
	for _, c := range rest {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// CleanupOld removes what a previous update left next to the running
// binary: the replaced executables ("<exe>.old") and abandoned
// ".goshareit-update-<n>" stage directories. Nothing else is touched: a
// portable install may share its directory with the user's own files. On
// darwin it also drops the "<App>.app.old" bundle beside the current one.
// Best-effort: files still in use are skipped and retried on the next start.
func CleanupOld() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return
	}
	dirs := []string{filepath.Dir(exe)}
	if bundle := bundleRoot(exe); bundle != "" {
		os.RemoveAll(bundle + ".old")
		dirs = append(dirs, filepath.Dir(bundle))
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			switch {
			case !e.IsDir() && updaterOldFiles[name]:
				os.Remove(filepath.Join(dir, name))
			case e.IsDir() && isStageDir(name):
				os.RemoveAll(filepath.Join(dir, name))
			}
		}
	}
}
