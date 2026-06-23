package capture

import (
	"context"
	"testing"
)

// fakeRec is a minimal Recorder for composite routing tests. Importing the
// internal/core/fake package would cycle (it imports capture), so define a local
// double here.
type fakeRec struct {
	modes     []Mode
	recording bool
	starts    []Mode
}

func (f *fakeRec) Start(_ context.Context, mode Mode) error {
	if f.recording {
		return ErrAlreadyRecording
	}
	f.recording = true
	f.starts = append(f.starts, mode)
	return nil
}

func (f *fakeRec) Stop(_ context.Context) (Result, error) {
	if !f.recording {
		return Result{}, ErrNotRecording
	}
	f.recording = false
	return Result{}, nil
}

func (f *fakeRec) Recording() bool    { return f.recording }
func (f *fakeRec) Capabilities() Caps { return Caps{Modes: f.modes} }

func TestCompositeRoutesGIF(t *testing.T) {
	video := &fakeRec{modes: []Mode{VideoRegion, VideoFull}}
	gif := &fakeRec{modes: []Mode{GIF}}
	c := NewCompositeRecorder(video, gif)

	if err := c.Start(context.Background(), GIF); err != nil {
		t.Fatalf("Start GIF: %v", err)
	}
	if len(gif.starts) != 1 || gif.starts[0] != GIF {
		t.Errorf("gif recorder starts = %v", gif.starts)
	}
	if len(video.starts) != 0 {
		t.Errorf("video recorder should not have started: %v", video.starts)
	}
	if !c.Recording() {
		t.Error("composite should report recording")
	}
	if _, err := c.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if c.Recording() {
		t.Error("composite should not report recording after Stop")
	}
}

func TestCompositeRoutesVideo(t *testing.T) {
	video := &fakeRec{modes: []Mode{VideoRegion, VideoFull}}
	gif := &fakeRec{modes: []Mode{GIF}}
	c := NewCompositeRecorder(video, gif)

	if err := c.Start(context.Background(), VideoFull); err != nil {
		t.Fatalf("Start VideoFull: %v", err)
	}
	if len(video.starts) != 1 || video.starts[0] != VideoFull {
		t.Errorf("video recorder starts = %v", video.starts)
	}
	if len(gif.starts) != 0 {
		t.Errorf("gif recorder should not have started: %v", gif.starts)
	}
}

func TestCompositeCapabilitiesUnion(t *testing.T) {
	video := &fakeRec{modes: []Mode{VideoRegion, VideoFull}}
	gif := &fakeRec{modes: []Mode{GIF}}
	c := NewCompositeRecorder(video, gif)

	got := c.Capabilities().Modes
	want := map[Mode]bool{VideoRegion: false, VideoFull: false, GIF: false}
	for _, m := range got {
		if _, ok := want[m]; !ok {
			t.Errorf("unexpected mode %v", m)
		}
		want[m] = true
	}
	for m, seen := range want {
		if !seen {
			t.Errorf("missing mode %v in union", m)
		}
	}
}

func TestCompositeDoubleStart(t *testing.T) {
	video := &fakeRec{modes: []Mode{VideoFull}}
	gif := &fakeRec{modes: []Mode{GIF}}
	c := NewCompositeRecorder(video, gif)

	if err := c.Start(context.Background(), GIF); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := c.Start(context.Background(), VideoFull); err != ErrAlreadyRecording {
		t.Fatalf("second Start = %v, want ErrAlreadyRecording", err)
	}
}

func TestCompositeNilFamily(t *testing.T) {
	c := NewCompositeRecorder(nil, &fakeRec{modes: []Mode{GIF}})
	if err := c.Start(context.Background(), VideoFull); err == nil {
		t.Fatal("expected error starting unsupported (nil) video family")
	}
}

func TestCompositeStopWithoutStart(t *testing.T) {
	c := NewCompositeRecorder(&fakeRec{}, &fakeRec{})
	if _, err := c.Stop(context.Background()); err != ErrNotRecording {
		t.Fatalf("Stop = %v, want ErrNotRecording", err)
	}
}
