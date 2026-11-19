package upload

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// policyClient trusts the httptest certificate and applies the production
// redirect policy.
func policyClient(srv *httptest.Server) *http.Client {
	c := srv.Client()
	c.CheckRedirect = checkRedirect
	return c
}

func TestDefaultClientPolicy(t *testing.T) {
	if defaultClient.Timeout != 0 {
		t.Errorf("default client has a total timeout (%v); large uploads would be cut off", defaultClient.Timeout)
	}
	if defaultClient.CheckRedirect == nil {
		t.Error("default client has no redirect policy")
	}
	tr, ok := defaultClient.Transport.(*http.Transport)
	if !ok || tr.ResponseHeaderTimeout == 0 || tr.Proxy == nil {
		t.Errorf("default transport: want a response-header timeout and the environment proxy, got %+v", tr)
	}
}

// A custom-uploader secret in a non-Authorization header must not follow a
// redirect to another host.
func TestCustomHeaderDroppedOnCrossHostRedirect(t *testing.T) {
	var mu sync.Mutex
	gotKey, hit := "", false
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotKey, hit = r.Header.Get("X-Api-Key"), true
		mu.Unlock()
		w.Write([]byte("https://example.test/x.png"))
	}))
	defer sink.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL+"/landing", http.StatusFound)
	}))
	defer origin.Close()

	c := NewCustom(CustomConfig{URL: origin.URL + "/upload", Headers: map[string]string{"X-Api-Key": "SECRET-123"}}, policyClient(origin))
	if _, err := c.Upload(context.Background(), "a.png", bytes.NewReader([]byte("png")), 3, "image/png"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !hit || gotKey != "" {
		t.Fatalf("redirect target hit=%v, X-Api-Key=%q; want hit with no key", hit, gotKey)
	}
}

func TestCustomHeaderKeptOnSameHostRedirect(t *testing.T) {
	var gotKey string
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload" {
			http.Redirect(w, r, srv.URL+"/landing", http.StatusFound)
			return
		}
		gotKey = r.Header.Get("X-Api-Key")
		w.Write([]byte("https://example.test/x.png"))
	}))
	defer srv.Close()

	c := NewCustom(CustomConfig{URL: srv.URL + "/upload", Headers: map[string]string{"X-Api-Key": "SECRET-123"}}, policyClient(srv))
	if _, err := c.Upload(context.Background(), "a.png", bytes.NewReader([]byte("png")), 3, "image/png"); err != nil {
		t.Fatal(err)
	}
	if gotKey != "SECRET-123" {
		t.Fatalf("same-host redirect lost the header: %q", gotKey)
	}
}

// Basic auth and the file must not be resent in cleartext when an https
// endpoint redirects to http.
func TestHTTPSDowngradeRedirectRefused(t *testing.T) {
	var mu sync.Mutex
	hit := false
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hit = true
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer sink.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	wd := NewWebDAV(WebDAVConfig{BaseURL: origin.URL, Username: "alice", Password: "hunter2"}, policyClient(origin))
	if _, err := wd.Upload(context.Background(), "a.png", bytes.NewReader([]byte("SCREENSHOT")), 10, "image/png"); err == nil {
		t.Fatal("expected the https->http redirect to fail the upload")
	}
	mu.Lock()
	defer mu.Unlock()
	if hit {
		t.Fatal("plain-http redirect target was contacted")
	}
}

// A key in the URL query must not appear in the transport error, which is
// logged and shown in the "Upload failed" notification.
func TestCustomTransportErrorRedactsURL(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close() // nothing listens: connection refused
	c := NewCustom(CustomConfig{URL: "https://" + addr + "/1/upload?key=SECRET-KEY-42"}, nil)
	_, err = c.Upload(context.Background(), "a.png", bytes.NewReader([]byte("png")), 3, "image/png")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SECRET-KEY-42") {
		t.Fatalf("secret in error text: %v", err)
	}
	if !strings.Contains(err.Error(), addr) {
		t.Errorf("error lost the host: %v", err)
	}
}
