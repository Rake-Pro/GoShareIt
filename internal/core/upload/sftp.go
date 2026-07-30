package upload

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const sftpDialTimeout = 10 * time.Second

// SFTPConfig configures the SFTP uploader. Password/PrivateKeyPEM/
// PrivateKeyPassphrase must already be resolved.
type SFTPConfig struct {
	Host                 string
	Port                 int // default 22
	User                 string
	Password             string // used when PrivateKeyPEM is empty
	PrivateKeyPEM        string // PEM-encoded private key; takes precedence over Password
	PrivateKeyPassphrase string // passphrase for an encrypted PrivateKeyPEM
	RemoteDir            string
	URLTemplate          string // public link template; placeholder {name}. Required for a usable public URL.
	HostKeyFingerprint   string // "SHA256:..." (ssh-keygen -lf format); empty leaves the host key unverified
}

// SFTP uploads files over SFTP.
type SFTP struct {
	cfg SFTPConfig
}

// NewSFTP builds an uploader.
func NewSFTP(cfg SFTPConfig) *SFTP {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	return &SFTP{cfg: cfg}
}

func (s *SFTP) addr() string {
	return net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
}

func (s *SFTP) clientConfig() (*ssh.ClientConfig, error) {
	var auth []ssh.AuthMethod
	if s.cfg.PrivateKeyPEM != "" {
		var signer ssh.Signer
		var err error
		if s.cfg.PrivateKeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(s.cfg.PrivateKeyPEM), []byte(s.cfg.PrivateKeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(s.cfg.PrivateKeyPEM))
		}
		if err != nil {
			return nil, fmt.Errorf("sftp: parse private key: %w", err)
		}
		auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else {
		auth = []ssh.AuthMethod{ssh.Password(s.cfg.Password)}
	}

	// Structured as a variable (rather than always calling
	// ssh.InsecureIgnoreHostKey inline) so a later config-validation phase
	// can warn callers when HostKeyFingerprint is left empty.
	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	if s.cfg.HostKeyFingerprint != "" {
		want := s.cfg.HostKeyFingerprint
		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			if got := ssh.FingerprintSHA256(key); got != want {
				return fmt.Errorf("sftp: host key fingerprint mismatch: got %s, want %s", got, want)
			}
			return nil
		}
	}

	return &ssh.ClientConfig{
		User:            s.cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         sftpDialTimeout,
	}, nil
}

// Upload dials the SSH server, uploads the file over SFTP, and returns its
// share link.
func (s *SFTP) Upload(ctx context.Context, name string, body io.Reader, size int64, mime string) (UploadResult, error) {
	cfg, err := s.clientConfig()
	if err != nil {
		return UploadResult{}, err
	}

	client, err := ssh.Dial("tcp", s.addr(), cfg)
	if err != nil {
		return UploadResult{}, fmt.Errorf("sftp: ssh dial: %w", err)
	}
	defer client.Close()

	sc, err := sftp.NewClient(client)
	if err != nil {
		return UploadResult{}, fmt.Errorf("sftp: new client: %w", err)
	}
	defer sc.Close()

	if err := s.upload(sc, name, body); err != nil {
		return UploadResult{}, err
	}
	return s.result(name), nil
}

// upload writes body to RemoteDir/name via an already-connected sftp client.
// Split out from Upload so it can be exercised against an in-process test
// server without a real SSH handshake.
func (s *SFTP) upload(client *sftp.Client, name string, body io.Reader) error {
	if dir := s.dir(); dir != "" {
		if err := client.MkdirAll(dir); err != nil {
			return fmt.Errorf("sftp: mkdir %s: %w", dir, err)
		}
	}
	remotePath := s.remotePath(name)
	f, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("sftp: create %s: %w", remotePath, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return fmt.Errorf("sftp: write %s: %w", remotePath, err)
	}
	return nil
}

func (s *SFTP) dir() string {
	return strings.TrimRight(s.cfg.RemoteDir, "/")
}

func (s *SFTP) remotePath(name string) string {
	dir := s.dir()
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func (s *SFTP) result(name string) UploadResult {
	if s.cfg.URLTemplate == "" {
		p := s.remotePath(name)
		return UploadResult{PublicURL: p, DirectURL: p}
	}
	link := renderURLTemplate(s.cfg.URLTemplate, map[string]string{"name": name})
	return UploadResult{PublicURL: link, DirectURL: link}
}
