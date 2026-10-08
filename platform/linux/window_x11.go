//go:build linux

package linux

import (
	"errors"
	"fmt"
	"image"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xinerama"
	"github.com/jezek/xgb/xproto"
)

// primaryOrigin returns the root-window position of the first xinerama screen.
// kbinani/screenshot expresses every rectangle relative to that screen, so
// root-relative coordinates (window geometry, ffmpeg x11grab offsets) convert
// by subtracting or adding this point.
func primaryOrigin() (image.Point, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return image.Point{}, fmt.Errorf("linux capture: X connection: %w", err)
	}
	defer c.Close()
	if err := xinerama.Init(c); err != nil {
		return image.Point{}, nil // no xinerama: single screen at the root origin
	}
	reply, err := xinerama.QueryScreens(c).Reply()
	if err != nil || reply.Number == 0 {
		return image.Point{}, nil
	}
	return image.Pt(int(reply.ScreenInfo[0].XOrg), int(reply.ScreenInfo[0].YOrg)), nil
}

// activeWindowRect returns the root-relative rectangle of the focused
// top-level window (_NET_ACTIVE_WINDOW), grown by the window manager's frame
// (_NET_FRAME_EXTENTS) so the capture includes the title bar like the macOS
// and Windows backends do.
func activeWindowRect() (image.Rectangle, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("linux capture: X connection: %w", err)
	}
	defer c.Close()
	root := xproto.Setup(c).DefaultScreen(c).Root

	active, err := atom(c, "_NET_ACTIVE_WINDOW")
	if err != nil {
		return image.Rectangle{}, err
	}
	prop, err := xproto.GetProperty(c, false, root, active, xproto.AtomWindow, 0, 1).Reply()
	if err != nil || prop.ValueLen == 0 || len(prop.Value) < 4 {
		return image.Rectangle{}, errors.New("linux capture: no active window (_NET_ACTIVE_WINDOW unset)")
	}
	win := xproto.Window(xgb.Get32(prop.Value))
	if win == 0 {
		return image.Rectangle{}, errors.New("linux capture: no active window")
	}

	geom, err := xproto.GetGeometry(c, xproto.Drawable(win)).Reply()
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("linux capture: window geometry: %w", err)
	}
	pos, err := xproto.TranslateCoordinates(c, win, root, 0, 0).Reply()
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("linux capture: window position: %w", err)
	}
	r := image.Rect(int(pos.DstX), int(pos.DstY), int(pos.DstX)+int(geom.Width), int(pos.DstY)+int(geom.Height))

	if extents, err := atom(c, "_NET_FRAME_EXTENTS"); err == nil {
		if ext, err := xproto.GetProperty(c, false, win, extents, xproto.AtomCardinal, 0, 4).Reply(); err == nil && len(ext.Value) >= 16 {
			left := int(xgb.Get32(ext.Value[0:]))
			right := int(xgb.Get32(ext.Value[4:]))
			top := int(xgb.Get32(ext.Value[8:]))
			bottom := int(xgb.Get32(ext.Value[12:]))
			r = image.Rect(r.Min.X-left, r.Min.Y-top, r.Max.X+right, r.Max.Y+bottom)
		}
	}
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return image.Rectangle{}, errors.New("linux capture: empty active window rect")
	}
	return r, nil
}

func atom(c *xgb.Conn, name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(c, true, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, fmt.Errorf("linux capture: atom %s: %w", name, err)
	}
	if reply.Atom == 0 {
		return 0, fmt.Errorf("linux capture: the window manager does not provide %s", name)
	}
	return reply.Atom, nil
}
