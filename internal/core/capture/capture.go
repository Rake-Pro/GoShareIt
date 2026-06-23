// Package capture defines the portable capture abstraction. The core depends
// only on the Capturer interface; concrete OS backends live under platform/.
package capture

import "context"

// Mode identifies what kind of capture the user requested.
type Mode int

const (
	RegionInteractive Mode = iota
	FullScreen
	ActiveWindow
	WindowPick
	LastRegion
	VideoRegion
	VideoFull
	GIF
)

func (m Mode) String() string {
	switch m {
	case RegionInteractive:
		return "RegionInteractive"
	case FullScreen:
		return "FullScreen"
	case ActiveWindow:
		return "ActiveWindow"
	case WindowPick:
		return "WindowPick"
	case LastRegion:
		return "LastRegion"
	case VideoRegion:
		return "VideoRegion"
	case VideoFull:
		return "VideoFull"
	case GIF:
		return "GIF"
	default:
		return "Unknown"
	}
}

// Kind distinguishes still images from video/animation output.
type Kind int

const (
	KindImage Kind = iota
	KindVideo
)

// Request is the input to a capture operation.
type Request struct {
	Mode            Mode
	CopyToClipboard bool
	SaveLocal       bool
	SaveDir         string
}

// Result is the output of a capture operation. Bytes holds the encoded media
// (PNG for images); Path is set when the backend wrote a local file.
type Result struct {
	Path  string
	Bytes []byte
	Mime  string
	Kind  Kind
}

// Caps advertises which modes a backend supports.
type Caps struct {
	Modes []Mode
}

// Capturer is the OS capture seam. Implementations are platform-specific.
type Capturer interface {
	Capture(ctx context.Context, r Request) (Result, error)
	Capabilities() Caps
}
