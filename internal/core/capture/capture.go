// Package capture defines the portable capture abstraction. The core depends
// only on the Capturer interface; concrete OS backends live under platform/.
package capture

import (
	"context"
	"errors"
	"image"
)

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
	Edit            bool
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

// Sentinel errors for the Recorder state machine.
var (
	ErrAlreadyRecording = errors.New("capture: recording already in progress")
	ErrNotRecording     = errors.New("capture: no recording in progress")
)

// Recorder is the stateful capture seam for screen recording. Unlike Capturer
// (one-shot), a Recorder is started, runs in the background, and is later
// stopped to finalize the media. Implementations are platform-specific and must
// be safe for concurrent Start/Stop/Recording calls.
type Recorder interface {
	// Start begins recording for the given mode (VideoRegion, VideoFull). It
	// returns once the OS recorder is running; it does not block for the
	// duration. Calling Start while already recording returns ErrAlreadyRecording.
	Start(ctx context.Context, mode Mode) error

	// Stop ends the active recording, finalizes the container, and returns the
	// encoded media as a Result (Kind=KindVideo, Mime="video/mp4"). Calling Stop
	// with no active recording returns ErrNotRecording.
	Stop(ctx context.Context) (Result, error)

	// Recording reports whether a recording is currently in progress.
	Recording() bool

	// Capabilities advertises which video modes this Recorder supports.
	Capabilities() Caps
}

// RegionRecorder is a Recorder that can additionally crop its recording to a
// rectangle. The base Recorder interface is unchanged; recorders opt in by also
// implementing StartRegion. Callers use a type assertion to detect support.
type RegionRecorder interface {
	Recorder

	// StartRegion begins recording cropped to rect (screen pixel coords,
	// top-left origin). An empty rect means full screen (equivalent to Start).
	StartRegion(ctx context.Context, mode Mode, rect image.Rectangle) error
}
