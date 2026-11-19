//go:build linux

package linux

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// fakePortal answers Screenshot with the request path the real portal would
// use and leaves the Response signal to the test.
type fakePortal struct{ paths chan dbus.ObjectPath }

func (p *fakePortal) Screenshot(sender dbus.Sender, parent string, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	tok, _ := opts["handle_token"].Value().(string)
	s := strings.ReplaceAll(strings.TrimPrefix(string(sender), ":"), ".", "_")
	path := dbus.ObjectPath(portalPath + "/request/" + s + "/" + tok)
	p.paths <- path
	return path, nil
}

// privateSessionBus starts a throwaway dbus-daemon and points the session bus
// address at it; the test is skipped where dbus-daemon is not installed.
func privateSessionBus(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon not installed")
	}
	dir := t.TempDir()
	conf := filepath.Join(dir, "bus.conf")
	if err := os.WriteFile(conf, []byte(`<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig><type>session</type><listen>unix:dir=`+dir+`</listen>
<policy context="default"><allow send_destination="*" eavesdrop="true"/><allow eavesdrop="true"/><allow own="*"/></policy></busconfig>`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("dbus-daemon", "--config-file="+conf, "--nofork", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skip("dbus-daemon:", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(addr))
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Another session-bus peer that answers on the portal's request path first
// must be ignored: its file is neither returned nor deleted, and the real
// portal's answer still wins.
func TestPortalResponseOnlyFromPortal(t *testing.T) {
	privateSessionBus(t)

	pc, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	fp := &fakePortal{paths: make(chan dbus.ObjectPath, 1)}
	if err := pc.Export(fp, portalPath, "org.freedesktop.portal.Screenshot"); err != nil {
		t.Fatal(err)
	}
	if r, err := pc.RequestName(portalDest, dbus.NameFlagDoNotQueue); err != nil || r != dbus.RequestNameReplyPrimaryOwner {
		t.Fatal("own portal name:", r, err)
	}
	ac, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer ac.Close()

	dir := t.TempDir()
	victim := filepath.Join(dir, "important-document.txt")
	if err := os.WriteFile(victim, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	planted := filepath.Join(dir, "planted.png")
	writePNG(t, planted, 7, 3)
	real := filepath.Join(dir, "real.png")
	writePNG(t, real, 5, 5)

	type result struct {
		img image.Image
		err error
	}
	done := make(chan result, 1)
	go func() {
		img, err := portalScreenshot(context.Background(), false)
		done <- result{img, err}
	}()
	path := <-fp.paths
	for _, target := range []string{victim, planted} {
		if err := ac.Emit(path, portalRequestIf+".Response", uint32(0),
			map[string]dbus.Variant{"uri": dbus.MakeVariant("file://" + target)}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond) // let the spoofed signals arrive first
	if err := pc.Emit(path, portalRequestIf+".Response", uint32(0),
		map[string]dbus.Variant{"uri": dbus.MakeVariant("file://" + real)}); err != nil {
		t.Fatal(err)
	}

	var res result
	select {
	case res = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("no result")
	}
	if res.err != nil {
		t.Fatal(res.err)
	}
	if got := res.img.Bounds(); got != image.Rect(0, 0, 5, 5) {
		t.Fatalf("got the spoofed image (%v)", got)
	}
	for _, p := range []string{victim, planted} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was deleted: %v", filepath.Base(p), err)
		}
	}
	if _, err := os.Stat(real); !os.IsNotExist(err) {
		t.Error("the portal's own file was not removed after reading")
	}
}

func TestReadPortalFileKeepsUndecodableFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(p, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPortalFile("file://" + p); err == nil {
		t.Fatal("expected a decode error")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file was removed: %v", err)
	}
}

// A PNG whose header claims enormous dimensions is refused before decoding.
func TestReadPortalFileRejectsHugeImage(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	// IHDR data starts at byte 16 (8 signature + 4 length + 4 type); its CRC
	// covers the type and the 13 data bytes.
	binary.BigEndian.PutUint32(b[16:], 100000)
	binary.BigEndian.PutUint32(b[20:], 100000)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	p := filepath.Join(t.TempDir(), "bomb.png")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readPortalFile("file://" + p)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v, want too large", err)
	}
}
