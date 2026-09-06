package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ReleaseNotes is one published release's notes, as shown in the changelog
// window before a minor/major update.
type ReleaseNotes struct {
	Version string
	Date    time.Time
	Body    string // GitHub release body (markdown)
}

// MinorBump reports whether moving from current to next changes the major or
// minor version (1.2.9 -> 1.3.0 yes, 1.2.9 -> 1.2.10 no). Unparsable
// versions count as no bump.
func MinorBump(current, next string) bool {
	c, _, err := parseSemver(strings.TrimPrefix(current, "v"))
	if err != nil {
		return false
	}
	n, _, err := parseSemver(strings.TrimPrefix(next, "v"))
	if err != nil {
		return false
	}
	return c[0] != n[0] || c[1] != n[1]
}

// Notes returns the notes of every published release newer than current and
// up to next, oldest first, so the changelog window can show the whole span
// a user is jumping across.
func (u *Updater) Notes(ctx context.Context, current, next string) ([]ReleaseNotes, error) {
	req, err := u.apiRequest(ctx, "/repos/"+u.cfg.Repo+"/releases?per_page=100", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	resp, err := u.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: notes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: notes: unexpected status %s", resp.Status)
	}
	var list []struct {
		TagName     string    `json:"tag_name"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		PublishedAt time.Time `json:"published_at"`
		Body        string    `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return nil, fmt.Errorf("update: decode releases: %w", err)
	}
	cur := strings.TrimPrefix(current, "v")
	top := strings.TrimPrefix(next, "v")
	var out []ReleaseNotes
	for _, r := range list {
		if r.Draft || r.Prerelease {
			continue
		}
		v := strings.TrimPrefix(r.TagName, "v")
		newer, err := semverGreater(v, cur)
		if err != nil || !newer {
			continue
		}
		if above, err := semverGreater(v, top); err != nil || above {
			continue
		}
		out = append(out, ReleaseNotes{Version: v, Date: r.PublishedAt, Body: r.Body})
	}
	sort.Slice(out, func(i, j int) bool {
		g, _ := semverGreater(out[j].Version, out[i].Version)
		return g
	})
	return out, nil
}

var (
	mdLink   = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	mdInline = regexp.MustCompile("[*_`]{1,2}")
)

// NoteLine is one rendered line of release notes.
type NoteLine struct {
	Text    string
	Heading bool
}

// RenderNotes flattens markdown release bodies into plain lines: headings
// keep their text and are flagged, bullets get a bullet glyph, links keep
// their label, emphasis markers are dropped, blank runs collapse. Each
// release is introduced by a "vX.Y.Z (date)" heading.
func RenderNotes(notes []ReleaseNotes) []NoteLine {
	var out []NoteLine
	for _, n := range notes {
		head := "v" + n.Version
		if !n.Date.IsZero() {
			head += "  (" + n.Date.Format("2006-01-02") + ")"
		}
		out = append(out, NoteLine{Text: head, Heading: true})
		blank := false
		for _, raw := range strings.Split(strings.ReplaceAll(n.Body, "\r\n", "\n"), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				blank = true
				continue
			}
			if blank && len(out) > 0 && !out[len(out)-1].Heading {
				out = append(out, NoteLine{Text: ""})
			}
			blank = false
			line = mdLink.ReplaceAllString(line, "$1")
			switch {
			case strings.HasPrefix(line, "#"):
				out = append(out, NoteLine{Text: strings.TrimSpace(strings.TrimLeft(line, "#")), Heading: true})
			case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
				out = append(out, NoteLine{Text: "• " + mdInline.ReplaceAllString(line[2:], "")})
			default:
				out = append(out, NoteLine{Text: mdInline.ReplaceAllString(line, "")})
			}
		}
		out = append(out, NoteLine{Text: ""})
	}
	return out
}
