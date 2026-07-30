// Package history records uploads in an append-only JSON-lines log and can
// re-copy a past upload's link to the clipboard.
package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/clipboard"
)

// Entry is one recorded upload.
type Entry struct {
	Name       string    `json:"name"`
	Time       time.Time `json:"time"`
	PublicURL  string    `json:"public_url"`
	DirectURL  string    `json:"direct_url"`
	ShareToken string    `json:"share_token"`
}

// History is an append-only log backed by a file.
type History struct {
	mu   sync.Mutex
	path string
}

// New opens (creating parent dirs as needed) a history log at path.
func New(path string) (*History, error) {
	if path == "" {
		return nil, fmt.Errorf("history: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("history: mkdir: %w", err)
	}
	// Entries hold public share URLs, which are capability links: anyone with
	// the URL can fetch the file. Tighten a log written by an older version
	// (0644) so the fix is not limited to fresh installs. Best-effort: a
	// missing file or a filesystem without unix modes is not an error.
	_ = os.Chmod(path, 0o600)
	return &History{path: path}, nil
}

// Append writes one entry. Time is stamped if zero.
func (h *History) Append(e Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("history: open: %w", err)
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("history: marshal: %w", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("history: write: %w", err)
	}
	return nil
}

// List returns all entries in file order.
func (h *History) List() ([]Entry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f, err := os.Open(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("history: open: %w", err)
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("history: parse: %w", err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("history: scan: %w", err)
	}
	return out, nil
}

// ReCopy copies the DirectURL of the entry with the given token to the
// clipboard. If token is empty, the most recent entry is used.
func (h *History) ReCopy(cb clipboard.Clipboard, token string) error {
	entries, err := h.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("history: no entries")
	}
	var target *Entry
	if token == "" {
		target = &entries[len(entries)-1]
	} else {
		for i := range entries {
			if entries[i].ShareToken == token {
				target = &entries[i]
				break
			}
		}
	}
	if target == nil {
		return fmt.Errorf("history: token %q not found", token)
	}
	return cb.WriteText(target.DirectURL)
}
