//go:build linux

package linux

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

const (
	portalDest             = "org.freedesktop.portal.Desktop"
	portalPath             = "/org/freedesktop/portal/desktop"
	portalScreenshotMethod = "org.freedesktop.portal.Screenshot.Screenshot"
	portalRequestIf        = "org.freedesktop.portal.Request"
)

// ErrPortalCancelled is returned when the user dismisses the portal's own
// picker (interactive capture) without taking a screenshot.
var ErrPortalCancelled = fmt.Errorf("linux capture: %w", capture.ErrCancelled)

// portalWait bounds a non-interactive portal screenshot; interactive calls are
// bounded only by ctx, because the user is driving the compositor's picker.
const portalWait = 30 * time.Second

var portalSeq atomic.Uint64

// portalScreenshot takes a screenshot through the XDG desktop portal and
// returns the decoded image. interactive=false grabs the whole desktop (all
// outputs stitched, origin top-left) without any UI; interactive=true opens
// the compositor's screenshot picker (GNOME Shell's capture UI, KDE's
// Spectacle flow) and returns whatever the user selected.
func portalScreenshot(ctx context.Context, interactive bool) (image.Image, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("linux capture: session bus: %w", err)
	}
	defer conn.Close()

	// The portal names its request object from our unique bus name and the
	// handle token, so the Response signal can be subscribed before the call
	// is made (the spec's recommended race-free pattern).
	token := fmt.Sprintf("goshareit%d", portalSeq.Add(1))
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	reqPath := dbus.ObjectPath(fmt.Sprintf("%s/request/%s/%s", portalPath, sender, token))

	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(reqPath),
		dbus.WithMatchInterface(portalRequestIf),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return nil, fmt.Errorf("linux capture: portal signal match: %w", err)
	}

	if !interactive {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, portalWait)
		defer cancel()
	}

	options := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"interactive":  dbus.MakeVariant(interactive),
		"modal":        dbus.MakeVariant(true),
	}
	var handle dbus.ObjectPath
	call := conn.Object(portalDest, portalPath).CallWithContext(ctx, portalScreenshotMethod, 0, "", options)
	if err := call.Store(&handle); err != nil {
		return nil, fmt.Errorf("linux capture: portal Screenshot call: %w (is xdg-desktop-portal running?)", err)
	}
	if handle != reqPath {
		// Pre-0.9 portals pick their own request path; subscribe to that too.
		_ = conn.AddMatchSignal(
			dbus.WithMatchObjectPath(handle),
			dbus.WithMatchInterface(portalRequestIf),
			dbus.WithMatchMember("Response"),
		)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("linux capture: portal screenshot: %w", ctx.Err())
		case sig, ok := <-signals:
			if !ok {
				return nil, errors.New("linux capture: session bus closed")
			}
			if sig.Path != reqPath && sig.Path != handle {
				continue
			}
			if len(sig.Body) != 2 {
				return nil, fmt.Errorf("linux capture: malformed portal response %v", sig.Body)
			}
			code, _ := sig.Body[0].(uint32)
			results, _ := sig.Body[1].(map[string]dbus.Variant)
			switch code {
			case 0:
			case 1:
				return nil, ErrPortalCancelled
			default:
				return nil, fmt.Errorf("linux capture: portal screenshot failed (response %d)", code)
			}
			uri, _ := results["uri"].Value().(string)
			return readPortalFile(uri)
		}
	}
}

// readPortalFile decodes the PNG the portal wrote and removes it: the portal
// leaves the file to the caller (it lands in ~/Pictures on GNOME).
func readPortalFile(uri string) (image.Image, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return nil, fmt.Errorf("linux capture: portal returned an unusable uri %q", uri)
	}
	f, err := os.Open(u.Path)
	if err != nil {
		return nil, fmt.Errorf("linux capture: open portal screenshot: %w", err)
	}
	defer func() {
		f.Close()
		os.Remove(u.Path)
	}()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("linux capture: decode portal screenshot: %w", err)
	}
	return img, nil
}
