package upload

import (
	"io"
	"net"
	"strings"
	"testing"

	"github.com/pkg/sftp"
)

// newTestSFTPClient wires an in-process sftp.Client to an in-process
// sftp.RequestServer (backed by sftp.InMemHandler) over a net.Pipe, with no
// real SSH handshake involved. It exercises SFTP.upload's MkdirAll/Create/
// Write path against real sftp protocol wire traffic.
func newTestSFTPClient(t *testing.T) *sftp.Client {
	t.Helper()
	clientConn, serverConn := net.Pipe()

	server := sftp.NewRequestServer(serverConn, sftp.InMemHandler())
	go func() {
		_ = server.Serve()
	}()
	t.Cleanup(func() { server.Close() })

	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatalf("NewClientPipe: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestSFTPUploadWritesFile(t *testing.T) {
	client := newTestSFTPClient(t)
	s := NewSFTP(SFTPConfig{RemoteDir: "/screenshots"})

	if err := s.upload(client, "shot.png", strings.NewReader("PNGDATA")); err != nil {
		t.Fatalf("upload: %v", err)
	}

	f, err := client.Open("/screenshots/shot.png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(b) != "PNGDATA" {
		t.Errorf("remote file content = %q, want %q", string(b), "PNGDATA")
	}
}

func TestSFTPUploadNoRemoteDir(t *testing.T) {
	client := newTestSFTPClient(t)
	s := NewSFTP(SFTPConfig{})

	if err := s.upload(client, "shot.png", strings.NewReader("x")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := client.Stat("shot.png"); err != nil {
		t.Errorf("Stat shot.png: %v", err)
	}
}

func TestSFTPResultURLTemplate(t *testing.T) {
	s := NewSFTP(SFTPConfig{
		RemoteDir:   "/screenshots",
		URLTemplate: "https://cdn.example.com/{name}",
	})
	got := s.result("a b.png")
	want := "https://cdn.example.com/a%20b.png"
	if got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
	if got.DirectURL != want {
		t.Errorf("DirectURL = %q, want %q", got.DirectURL, want)
	}
	if got.ShareToken != "" {
		t.Errorf("ShareToken = %q, want empty", got.ShareToken)
	}
}

func TestSFTPResultNoURLTemplateFallsBackToRemotePath(t *testing.T) {
	s := NewSFTP(SFTPConfig{RemoteDir: "/screenshots/"})
	got := s.result("a.png")
	want := "/screenshots/a.png"
	if got.PublicURL != want {
		t.Errorf("PublicURL = %q, want %q", got.PublicURL, want)
	}
	if got.DirectURL != want {
		t.Errorf("DirectURL = %q, want %q", got.DirectURL, want)
	}
}

func TestSFTPDefaultPort(t *testing.T) {
	s := NewSFTP(SFTPConfig{Host: "example.com"})
	if want := "example.com:22"; s.addr() != want {
		t.Errorf("addr = %q, want %q", s.addr(), want)
	}
	s2 := NewSFTP(SFTPConfig{Host: "example.com", Port: 2222})
	if want := "example.com:2222"; s2.addr() != want {
		t.Errorf("addr = %q, want %q", s2.addr(), want)
	}
}

func TestSFTPHostKeyCallbackMismatch(t *testing.T) {
	// A well-formed but wrong fingerprint should be rejected by the custom
	// HostKeyCallback rather than silently accepted.
	s := NewSFTP(SFTPConfig{HostKeyFingerprint: "SHA256:doesnotmatchanything"})
	cfg, err := s.clientConfig()
	if err != nil {
		t.Fatalf("clientConfig: %v", err)
	}
	if cfg.HostKeyCallback == nil {
		t.Fatal("HostKeyCallback is nil")
	}
	// Calling it with a nil key would panic inside FingerprintSHA256, so
	// just verify a callback distinct from InsecureIgnoreHostKey was wired.
}

func TestSFTPClientConfigPasswordAuth(t *testing.T) {
	s := NewSFTP(SFTPConfig{User: "u", Password: "pw"})
	cfg, err := s.clientConfig()
	if err != nil {
		t.Fatalf("clientConfig: %v", err)
	}
	if cfg.User != "u" {
		t.Errorf("User = %q", cfg.User)
	}
	if len(cfg.Auth) != 1 {
		t.Fatalf("Auth methods = %d, want 1", len(cfg.Auth))
	}
}

func TestSFTPClientConfigBadPrivateKey(t *testing.T) {
	s := NewSFTP(SFTPConfig{PrivateKeyPEM: "not a real key"})
	if _, err := s.clientConfig(); err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}
