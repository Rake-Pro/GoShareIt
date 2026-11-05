package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// devFixture fakes the GitHub API (stable latest + tag lookups) and the
// download host (dev manifest). Empty stableTag / manifest mean 404.
type devFixture struct {
	stableTag string
	manifest  string            // raw JSON body
	tagAssets map[string]string // tag -> asset name served by releases/tags/<tag>
}

func (f devFixture) start(t *testing.T) (api, feed *httptest.Server) {
	t.Helper()
	am := http.NewServeMux()
	am.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if f.stableTag == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"id":1,"name":"stable-asset","size":1}]}`, f.stableTag)
	})
	am.HandleFunc("/repos/o/r/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/repos/o/r/releases/tags/")
		name, ok := f.tagAssets[tag]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":%q,"prerelease":true,"assets":[{"id":7,"name":%q,"size":1}]}`, tag, name)
	})
	fm := http.NewServeMux()
	fm.HandleFunc("/o/r/releases/download/dev-latest/dev-latest.json", func(w http.ResponseWriter, r *http.Request) {
		if f.manifest == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, f.manifest)
	})
	api, feed = httptest.NewServer(am), httptest.NewServer(fm)
	t.Cleanup(api.Close)
	t.Cleanup(feed.Close)
	return api, feed
}

func manifestJSON(version, tag string) string {
	return fmt.Sprintf(`{"name":"GoShareIt","version":%q,"tag":%q,"published_at":"2026-11-05T02:00:00Z","notes":"n","assets":[{"name":"x","url":"https://evil.example/x","sha256":"00","size":1}]}`, version, tag)
}

func checkWith(t *testing.T, f devFixture, current, channel string) (*Release, error) {
	t.Helper()
	api, feed := f.start(t)
	u, err := New(Config{Repo: "o/r", Current: current, Channel: channel, APIBaseURL: api.URL, FeedBaseURL: feed.URL})
	if err != nil {
		t.Fatal(err)
	}
	return u.Check(context.Background())
}

func TestDevCheckDevNewerThanStable(t *testing.T) {
	ver := "0.4.7-dev.12.ab12cd3"
	f := devFixture{
		stableTag: "v0.4.6",
		manifest:  manifestJSON(ver, "dev-ab12cd3"),
		tagAssets: map[string]string{"dev-ab12cd3": AssetName(ver)},
	}
	rel, err := checkWith(t, f, "0.4.6", ChannelDev)
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != ver || rel.TagName != "dev-ab12cd3" {
		t.Fatalf("got %+v", rel)
	}
	if a, ok := findAsset(rel.assets, AssetName(ver)); !ok || a.ID != 7 {
		t.Errorf("assets not taken from the tag lookup: %+v", rel.assets)
	}
}

func TestDevCheckStableNewerThanDev(t *testing.T) {
	f := devFixture{
		stableTag: "v0.4.7",
		manifest:  manifestJSON("0.4.7-dev.12.ab12cd3", "dev-ab12cd3"),
		tagAssets: map[string]string{"dev-ab12cd3": "x"},
	}
	rel, err := checkWith(t, f, "0.4.6", ChannelDev)
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "0.4.7" || rel.TagName != "v0.4.7" {
		t.Fatalf("got %+v", rel)
	}
}

func TestDevCheckManifest404UsesStable(t *testing.T) {
	rel, err := checkWith(t, devFixture{stableTag: "v0.4.7"}, "0.4.6", ChannelDev)
	if err != nil || rel == nil || rel.Version != "0.4.7" {
		t.Fatalf("got %+v, %v", rel, err)
	}
	rel, err = checkWith(t, devFixture{stableTag: "v0.4.6"}, "0.4.6", ChannelDev)
	if err != nil || rel != nil {
		t.Fatalf("up to date: got %+v, %v", rel, err)
	}
	// Neither published yet: not an error on the dev channel.
	rel, err = checkWith(t, devFixture{}, "0.4.6", ChannelDev)
	if err != nil || rel != nil {
		t.Fatalf("nothing published: got %+v, %v", rel, err)
	}
}

func TestDevCheckOnlyManifest(t *testing.T) {
	ver := "0.1.0-dev.3.abcdef0"
	f := devFixture{manifest: manifestJSON(ver, "dev-abcdef0"), tagAssets: map[string]string{"dev-abcdef0": AssetName(ver)}}
	rel, err := checkWith(t, f, "0.0.1", ChannelDev)
	if err != nil || rel == nil || rel.Version != ver {
		t.Fatalf("got %+v, %v", rel, err)
	}
}

func TestDevCheckNotNewerThanCurrent(t *testing.T) {
	ver := "0.4.7-dev.12.ab12cd3"
	f := devFixture{
		stableTag: "v0.4.6",
		manifest:  manifestJSON(ver, "dev-ab12cd3"),
		tagAssets: map[string]string{"dev-ab12cd3": AssetName(ver)},
	}
	rel, err := checkWith(t, f, ver, ChannelDev)
	if err != nil || rel != nil {
		t.Fatalf("same build: got %+v, %v", rel, err)
	}
	rel, err = checkWith(t, f, "0.4.7-dev.13.ffffff0", ChannelDev)
	if err != nil || rel != nil {
		t.Fatalf("newer local dev build: got %+v, %v", rel, err)
	}
}

func TestDevCheckRejectsBadTag(t *testing.T) {
	for _, tag := range []string{"dev-../../x", "v1.2.3", "dev-ABCDEF0", "dev-abc123", "dev-abc12345", "dev-abc1234/x"} {
		f := devFixture{
			stableTag: "v0.4.6",
			manifest:  manifestJSON("0.4.7-dev.12.ab12cd3", tag),
			tagAssets: map[string]string{tag: "x"},
		}
		rel, err := checkWith(t, f, "0.4.6", ChannelDev)
		if err == nil || rel != nil {
			t.Errorf("tag %q: got %+v, %v; want an error", tag, rel, err)
		}
	}
}

func TestDevCheckManifestErrorFallsBackToStable(t *testing.T) {
	api, _ := devFixture{stableTag: "v0.4.7"}.start(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(bad.Close)
	u, _ := New(Config{Repo: "o/r", Current: "0.4.6", Channel: ChannelDev, APIBaseURL: api.URL, FeedBaseURL: bad.URL})
	rel, err := u.Check(context.Background())
	if err != nil || rel == nil || rel.Version != "0.4.7" {
		t.Fatalf("got %+v, %v", rel, err)
	}
	// Both failing with real errors is an error.
	u, _ = New(Config{Repo: "o/r", Current: "0.4.6", Channel: ChannelDev, APIBaseURL: bad.URL, FeedBaseURL: bad.URL})
	if rel, err := u.Check(context.Background()); err == nil || rel != nil {
		t.Fatalf("both failing: got %+v, %v", rel, err)
	}
}

func TestStableChannelNoDowngradeFromDevBuild(t *testing.T) {
	f := devFixture{
		stableTag: "v0.4.6",
		manifest:  manifestJSON("0.4.8-dev.1.ab12cd3", "dev-ab12cd3"), // must be ignored on stable
		tagAssets: map[string]string{"dev-ab12cd3": "x"},
	}
	rel, err := checkWith(t, f, "0.4.7-dev.12.ab12cd3", ChannelStable)
	if err != nil || rel != nil {
		t.Fatalf("got %+v, %v", rel, err)
	}
	rel, err = checkWith(t, f, "0.4.7-dev.12.ab12cd3", "")
	if err != nil || rel != nil {
		t.Fatalf("empty channel: got %+v, %v", rel, err)
	}
	// A stable release at the same X.Y.Z does replace the dev build.
	f.stableTag = "v0.4.7"
	rel, err = checkWith(t, f, "0.4.7-dev.12.ab12cd3", ChannelStable)
	if err != nil || rel == nil || rel.Version != "0.4.7" {
		t.Fatalf("got %+v, %v", rel, err)
	}
}

func TestDevCheckRejectsVersionTagMismatch(t *testing.T) {
	for _, ver := range []string{"0.4.7-dev.12.ffffff0", "0.4.7-dev.12", "0.4.7-beta.1"} {
		f := devFixture{
			stableTag: "v0.4.6",
			manifest:  manifestJSON(ver, "dev-ab12cd3"),
			tagAssets: map[string]string{"dev-ab12cd3": AssetName(ver)},
		}
		if rel, err := checkWith(t, f, "0.4.6", ChannelDev); err == nil {
			t.Errorf("version %q with tag dev-ab12cd3: want error, got %+v", ver, rel)
		}
	}
}

// A real stable-lookup failure with no newer dev build is a failed check, not
// "up to date".
func TestDevCheckStableErrorSurfacesWhenDevNotNewer(t *testing.T) {
	ver := "0.4.7-dev.12.ab12cd3"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	t.Cleanup(api.Close)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, manifestJSON(ver, "dev-ab12cd3"))
	}))
	t.Cleanup(feed.Close)
	u, err := New(Config{Repo: "o/r", Current: ver, Channel: ChannelDev, APIBaseURL: api.URL, FeedBaseURL: feed.URL})
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := u.Check(context.Background()); err == nil {
		t.Fatalf("want the stable lookup error, got release %+v", rel)
	}
}

// End to end on the dev channel: manifest -> tag lookup -> download verified
// against the dev release's own checksums.txt (the manifest's hashes are not
// trusted).
func TestDevCheckAndDownload(t *testing.T) {
	ver := "0.4.7-dev.12.ab12cd3"
	payload := []byte("dev build bytes")
	sum := sha256.Sum256(payload)
	name := AssetName(ver)
	sums := hex.EncodeToString(sum[:]) + "  " + name + "\n"
	am := http.NewServeMux()
	am.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v0.4.6","assets":[]}`)
	})
	am.HandleFunc("/repos/o/r/releases/tags/dev-ab12cd3", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"dev-ab12cd3","prerelease":true,"assets":[{"id":1,"name":%q,"size":%d},{"id":2,"name":"checksums.txt","size":%d}]}`, name, len(payload), len(sums))
	})
	am.HandleFunc("/repos/o/r/releases/assets/1", func(w http.ResponseWriter, r *http.Request) { w.Write(payload) })
	am.HandleFunc("/repos/o/r/releases/assets/2", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sums) })
	api := httptest.NewServer(am)
	t.Cleanup(api.Close)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, manifestJSON(ver, "dev-ab12cd3"))
	}))
	t.Cleanup(feed.Close)

	u, err := New(Config{Repo: "o/r", Current: "0.4.6", Channel: ChannelDev, APIBaseURL: api.URL, FeedBaseURL: feed.URL})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != ver || rel.TagName != "dev-ab12cd3" {
		t.Fatalf("expected dev release %s, got %+v", ver, rel)
	}
	path, err := u.Download(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Error("downloaded payload mismatch")
	}
}
