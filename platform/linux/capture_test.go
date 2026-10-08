//go:build linux

package linux

import (
	"image"
	"image/color"
	"testing"
)

func TestIsWayland(t *testing.T) {
	cases := []struct {
		session, waylandDisplay string
		want                    bool
	}{
		{"wayland", "", true},
		{"x11", "wayland-0", false},
		{"", "wayland-0", true},
		{"", "", false},
		{"tty", "", false},
	}
	for _, c := range cases {
		t.Setenv("XDG_SESSION_TYPE", c.session)
		t.Setenv("WAYLAND_DISPLAY", c.waylandDisplay)
		if got := IsWayland(); got != c.want {
			t.Errorf("XDG_SESSION_TYPE=%q WAYLAND_DISPLAY=%q: got %v, want %v", c.session, c.waylandDisplay, got, c.want)
		}
	}
}

func TestCrop(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 10, 10))
	frame.Set(4, 4, color.RGBA{R: 255, A: 255})

	out, err := crop(frame, image.Rect(3, 3, 6, 6))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Bounds(); got != image.Rect(0, 0, 3, 3) {
		t.Fatalf("bounds = %v, want 3x3 at origin", got)
	}
	if r, _, _, _ := out.At(1, 1).RGBA(); r == 0 {
		t.Error("crop lost the marked pixel")
	}

	// Partly outside: clipped to the frame, not an error.
	out, err = crop(frame, image.Rect(8, 8, 20, 20))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Bounds(); got != image.Rect(0, 0, 2, 2) {
		t.Errorf("clipped bounds = %v, want 2x2", got)
	}

	// Fully outside: error.
	if _, err := crop(frame, image.Rect(20, 20, 30, 30)); err == nil {
		t.Error("expected an error for a rect outside the frame")
	}
}
