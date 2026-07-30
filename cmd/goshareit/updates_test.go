package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Rake-Pro/GoShareIt/internal/core"
	"github.com/Rake-Pro/GoShareIt/internal/core/config"
	"github.com/Rake-Pro/GoShareIt/internal/core/fake"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/update"
)

// releaseServer stands up a minimal fake GitHub "latest release" endpoint
// reporting tag so Check() finds an update newer than "0.1.0". No assets are
// served, so a follow-on Download() fails fast with a clear "no asset" error
// - that is fine for these tests, which only assert whether install() was
// *attempted* (via the resulting "Update failed" notification), not whether
// it fully succeeds.
func releaseServer(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[]}`, tag)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestApp(t *testing.T, confirmer *fake.Confirmer) (*core.App, *fake.Notifier) {
	t.Helper()
	hist, err := history.New(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatalf("history.New: %v", err)
	}
	notifier := &fake.Notifier{}
	providers := core.Providers{
		Capturer:  fake.NewCapturer(),
		Uploader:  fake.NewUploader(),
		Clipboard: &fake.Clipboard{},
		Notifier:  notifier,
	}
	if confirmer != nil {
		providers.Confirmer = confirmer
	}
	app, err := core.New(&config.Config{}, providers, zerolog.Nop(), hist)
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	return app, notifier
}

func newTestController(t *testing.T, app *core.App, srv *httptest.Server) *updateController {
	t.Helper()
	upd, err := update.New(update.Config{Repo: "o/r", Current: "0.1.0", APIBaseURL: srv.URL})
	if err != nil {
		t.Fatalf("update.New: %v", err)
	}
	return newUpdateController(upd, app, time.Hour, func() {})
}

// TestUpdateCheckManualWithConfirmerAccept verifies a manual check that finds
// an update, with a Confirmer wired and answering "yes", proceeds straight to
// install() without ever sending the quiet "Update available" notification.
func TestUpdateCheckManualWithConfirmerAccept(t *testing.T) {
	srv := releaseServer(t, "v9.9.9")
	confirmer := &fake.Confirmer{Result: true}
	app, notifier := newTestApp(t, confirmer)
	c := newTestController(t, app, srv)

	c.check(context.Background(), true)

	if confirmer.Count() != 1 {
		t.Fatalf("Confirm calls = %d, want 1", confirmer.Count())
	}
	call := confirmer.Calls[0]
	if call.Title != "Update available" {
		t.Errorf("Confirm title = %q", call.Title)
	}
	if call.OKLabel != "Update Now" || call.CancelLabel != "Later" {
		t.Errorf("Confirm labels = %q/%q, want %q/%q", call.OKLabel, call.CancelLabel, "Update Now", "Later")
	}
	// install() was attempted (Download fails fast: no assets on the fake
	// release), surfaced as an "Update failed" notification - proof the
	// accept path drove straight into install() rather than stopping at the
	// quiet notify+retitle fallback.
	if notifier.Count() != 1 || notifier.Notifications[0].Title != "Update failed" {
		t.Fatalf("notifications = %+v, want one \"Update failed\"", notifier.Notifications)
	}
}

// TestUpdateCheckManualWithConfirmerDecline verifies a manual check with a
// Confirmer answering "no" does not install and does not notify.
func TestUpdateCheckManualWithConfirmerDecline(t *testing.T) {
	srv := releaseServer(t, "v9.9.9")
	confirmer := &fake.Confirmer{Result: false}
	app, notifier := newTestApp(t, confirmer)
	c := newTestController(t, app, srv)

	c.check(context.Background(), true)

	if confirmer.Count() != 1 {
		t.Fatalf("Confirm calls = %d, want 1", confirmer.Count())
	}
	if notifier.Count() != 0 {
		t.Fatalf("notifications = %+v, want none", notifier.Notifications)
	}
	c.mu.Lock()
	pending := c.pending
	c.mu.Unlock()
	if pending == nil {
		t.Error("pending release should stay set so the tray-menu install fallback still works")
	}
}

// TestUpdateCheckManualWithConfirmerError verifies a Confirm error is treated
// like "no": no install, no notification.
func TestUpdateCheckManualWithConfirmerError(t *testing.T) {
	srv := releaseServer(t, "v9.9.9")
	confirmer := &fake.Confirmer{Err: fmt.Errorf("dialog failed")}
	app, notifier := newTestApp(t, confirmer)
	c := newTestController(t, app, srv)

	c.check(context.Background(), true)

	if notifier.Count() != 0 {
		t.Fatalf("notifications = %+v, want none", notifier.Notifications)
	}
}

// TestUpdateCheckManualNoConfirmer verifies the pre-existing quiet fallback
// (notify + tray retitle, no dialog) is unchanged when no Confirmer is wired
// (linux/dev-style builds without one, or if Providers.Confirmer is nil).
func TestUpdateCheckManualNoConfirmer(t *testing.T) {
	srv := releaseServer(t, "v9.9.9")
	app, notifier := newTestApp(t, nil)
	c := newTestController(t, app, srv)

	c.check(context.Background(), true)

	if notifier.Count() != 1 || notifier.Notifications[0].Title != "Update available" {
		t.Fatalf("notifications = %+v, want one \"Update available\"", notifier.Notifications)
	}
}

// TestUpdateCheckBackgroundNeverConfirms verifies background (periodic)
// checks keep the quiet notify+retitle behavior even with a Confirmer wired -
// only manual clicks may pop a dialog.
func TestUpdateCheckBackgroundNeverConfirms(t *testing.T) {
	srv := releaseServer(t, "v9.9.9")
	confirmer := &fake.Confirmer{Result: true}
	app, notifier := newTestApp(t, confirmer)
	c := newTestController(t, app, srv)

	c.check(context.Background(), false)

	if confirmer.Count() != 0 {
		t.Fatalf("Confirm calls = %d, want 0 (background must never confirm)", confirmer.Count())
	}
	if notifier.Count() != 1 || notifier.Notifications[0].Title != "Update available" {
		t.Fatalf("notifications = %+v, want one \"Update available\"", notifier.Notifications)
	}
}
