// Package update implements self-update from GitHub Releases. It is portable
// and CGO-free: the release feed is the GitHub REST API (the repo is public,
// so requests are anonymous), assets are verified against the release's
// checksums.txt, and Apply swaps the installed binaries (windows/linux) or
// the whole .app bundle (darwin).
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// DefaultAPIBaseURL is the GitHub REST API root.
const DefaultAPIBaseURL = "https://api.github.com"

// checksumsAsset is the asset holding "sha256  filename" lines for the release.
const checksumsAsset = "checksums.txt"

// Config configures an Updater.
type Config struct {
	Repo       string // "owner/name", e.g. "Rake-Pro/GoShareIt"
	Current    string // running version, e.g. "1.2.3" or "0.0.0-dev"
	APIBaseURL string // override for tests; DefaultAPIBaseURL when empty
	HTTPClient *http.Client
}

// Updater checks for, downloads and applies releases.
type Updater struct {
	cfg Config
}

// Release describes an available release.
type Release struct {
	Version string // "1.4.0"
	TagName string // "v1.4.0"
	assets  []asset
}

type asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type releaseJSON struct {
	TagName string  `json:"tag_name"`
	Draft   bool    `json:"draft"`
	Assets  []asset `json:"assets"`
}

// New returns an Updater. Repo and Current are required.
func New(cfg Config) (*Updater, error) {
	if cfg.Repo == "" {
		return nil, fmt.Errorf("update: repo is required")
	}
	if cfg.Current == "" {
		return nil, fmt.Errorf("update: current version is required")
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = DefaultAPIBaseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	return &Updater{cfg: cfg}, nil
}

// IsDev reports whether the running build is a dev build (never auto-updated).
func (u *Updater) IsDev() bool { return strings.Contains(u.cfg.Current, "-dev") }

// Check fetches the latest release and reports whether it is newer than the
// running version. A nil Release with nil error means "up to date".
func (u *Updater) Check(ctx context.Context) (*Release, error) {
	req, err := u.apiRequest(ctx, "/repos/"+u.cfg.Repo+"/releases/latest", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	resp, err := u.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("update: no release found (404): no releases published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: check: unexpected status %s", resp.Status)
	}
	var rj releaseJSON
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rj); err != nil {
		return nil, fmt.Errorf("update: decode release: %w", err)
	}
	ver := strings.TrimPrefix(rj.TagName, "v")
	newer, err := semverGreater(ver, strings.TrimPrefix(u.cfg.Current, "v"))
	if err != nil {
		return nil, fmt.Errorf("update: compare %q vs %q: %w", ver, u.cfg.Current, err)
	}
	if !newer {
		return nil, nil
	}
	return &Release{Version: ver, TagName: rj.TagName, assets: rj.Assets}, nil
}

// Download fetches this platform's asset for rel into a temp file and verifies
// its sha256 against the release's checksums.txt. It returns the archive path.
func (u *Updater) Download(ctx context.Context, rel *Release) (string, error) {
	name := AssetName(rel.Version)
	a, ok := findAsset(rel.assets, name)
	if !ok {
		return "", fmt.Errorf("update: release %s has no asset %q", rel.TagName, name)
	}
	sums, ok := findAsset(rel.assets, checksumsAsset)
	if !ok {
		return "", fmt.Errorf("update: release %s has no %s", rel.TagName, checksumsAsset)
	}
	sumBody, err := u.downloadAsset(ctx, sums.ID)
	if err != nil {
		return "", err
	}
	defer sumBody.Close()
	sumData, err := io.ReadAll(io.LimitReader(sumBody, 1<<20))
	if err != nil {
		return "", fmt.Errorf("update: read %s: %w", checksumsAsset, err)
	}
	want, err := checksumFor(string(sumData), name)
	if err != nil {
		return "", err
	}

	body, err := u.downloadAsset(ctx, a.ID)
	if err != nil {
		return "", err
	}
	defer body.Close()
	tmp, err := os.CreateTemp("", "goshareit-update-*"+archiveExt(name))
	if err != nil {
		return "", fmt.Errorf("update: temp file: %w", err)
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), body)
	cerr := tmp.Close()
	if err != nil || cerr != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("update: download %s: copy=%v close=%v", name, err, cerr)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("update: checksum mismatch for %s: got %s want %s", name, got, want)
	}
	log.Info().Str("asset", name).Str("version", rel.Version).Msg("update downloaded and verified")
	return tmp.Name(), nil
}

// downloadAsset streams a release asset by id. GitHub redirects to storage;
// Go's http client drops the Authorization header on the cross-host redirect.
func (u *Updater) downloadAsset(ctx context.Context, id int64) (io.ReadCloser, error) {
	req, err := u.apiRequest(ctx, "/repos/"+u.cfg.Repo+"/releases/assets/"+strconv.FormatInt(id, 10), "application/octet-stream")
	if err != nil {
		return nil, err
	}
	resp, err := u.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: download asset %d: %w", id, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("update: download asset %d: unexpected status %s", id, resp.Status)
	}
	return resp.Body, nil
}

func (u *Updater) apiRequest(ctx context.Context, path, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.APIBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("update: request %s: %w", path, err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

// AssetName returns the release asset filename the updater expects for the
// running platform. Must stay in sync with .github/workflows/release.yml.
func AssetName(version string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("GoShareIt_%s_darwin_universal.zip", version)
	case "windows":
		return fmt.Sprintf("GoShareIt_%s_windows_%s.zip", version, runtime.GOARCH)
	default:
		return fmt.Sprintf("GoShareIt_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	}
}

func findAsset(assets []asset, name string) (asset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return asset{}, false
}

// checksumFor extracts the sha256 for name from "sha256  filename" lines.
func checksumFor(sums, name string) (string, error) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			if len(fields[0]) != 64 {
				return "", fmt.Errorf("update: malformed checksum line for %s", name)
			}
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("update: %s missing entry for %s", checksumsAsset, name)
}

// semverGreater reports whether a > b for "X.Y.Z[-suffix]" versions. Suffixes
// are ignored for ordering except that a suffixed version loses a tie against
// the same bare version ("1.2.3-dev" < "1.2.3").
func semverGreater(a, b string) (bool, error) {
	am, as, err := parseSemver(a)
	if err != nil {
		return false, err
	}
	bm, bs, err := parseSemver(b)
	if err != nil {
		return false, err
	}
	for i := range am {
		if am[i] != bm[i] {
			return am[i] > bm[i], nil
		}
	}
	return as == "" && bs != "", nil
}

func parseSemver(v string) ([3]int, string, error) {
	var nums [3]int
	base, suffix, _ := strings.Cut(v, "-")
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return nums, "", fmt.Errorf("not X.Y.Z: %q", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nums, "", fmt.Errorf("not X.Y.Z: %q", v)
		}
		nums[i] = n
	}
	return nums, suffix, nil
}

// archiveExt returns ".zip" or ".tar.gz" matching the asset name.
func archiveExt(name string) string {
	if strings.HasSuffix(name, ".tar.gz") {
		return ".tar.gz"
	}
	return filepath.Ext(name)
}
