package capture

import (
	"context"
	"fmt"
	"sync"
)

// compositeRecorder routes Start/Stop to a per-family sub-recorder by mode: GIF
// goes to the gif recorder, VideoRegion/VideoFull to the video recorder. Either
// may be nil, meaning that family is unsupported.
type compositeRecorder struct {
	video Recorder
	gif   Recorder

	mu     sync.Mutex
	active Recorder
}

// NewCompositeRecorder builds a Recorder that dispatches by mode to the video or
// gif sub-recorder. Either may be nil to mark that family unsupported.
func NewCompositeRecorder(video, gif Recorder) Recorder {
	return &compositeRecorder{video: video, gif: gif}
}

func (c *compositeRecorder) Start(ctx context.Context, mode Mode) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil && c.active.Recording() {
		return ErrAlreadyRecording
	}
	var target Recorder
	switch mode {
	case GIF:
		target = c.gif
	case VideoRegion, VideoFull:
		target = c.video
	default:
		return fmt.Errorf("capture: mode %s unsupported", mode)
	}
	if target == nil {
		return fmt.Errorf("capture: mode %s unsupported", mode)
	}
	if err := target.Start(ctx, mode); err != nil {
		return err
	}
	c.active = target
	return nil
}

func (c *compositeRecorder) Stop(ctx context.Context) (Result, error) {
	c.mu.Lock()
	active := c.active
	c.mu.Unlock()
	if active == nil {
		return Result{}, ErrNotRecording
	}
	res, err := active.Stop(ctx)
	c.mu.Lock()
	c.active = nil
	c.mu.Unlock()
	return res, err
}

func (c *compositeRecorder) Recording() bool {
	c.mu.Lock()
	active := c.active
	c.mu.Unlock()
	return active != nil && active.Recording()
}

func (c *compositeRecorder) Capabilities() Caps {
	var modes []Mode
	if c.video != nil {
		modes = append(modes, c.video.Capabilities().Modes...)
	}
	if c.gif != nil {
		modes = append(modes, c.gif.Capabilities().Modes...)
	}
	return Caps{Modes: modes}
}
