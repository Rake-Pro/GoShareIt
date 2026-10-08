//go:build linux

package linux

import (
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// recordFramerate is the x11grab capture rate. 30fps balances smoothness and
// file size for typical share-a-clip usage.
const recordFramerate = "30"

// stopQuitTimeout bounds how long Stop waits for ffmpeg to exit after receiving
// 'q' on stdin before falling back to a hard kill. A clean 'q' lets ffmpeg
// flush and write the moov atom so the mp4 is playable; killing it does not, so
// the kill path is a last resort that may yield a truncated file.
const stopQuitTimeout = 10 * time.Second

// Recorder records an X11 screen to an mp4 by shelling out to ffmpeg's x11grab
// input. It implements capture.Recorder. On Wayland it advertises no modes:
// clients cannot read the screen there without the ScreenCast portal and a
// PipeWire consumer, which ffmpeg does not provide on its own.
type Recorder struct {
	wayland bool

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	outPath string
	done    chan struct{}
	waitErr error
}

// NewRecorder returns an ffmpeg/x11grab backed Recorder.
func NewRecorder() *Recorder {
	return &Recorder{wayland: IsWayland()}
}

// Capabilities advertises full-desktop and region recording on X11, nothing
// on Wayland (the host then hides the recording items).
func (r *Recorder) Capabilities() capture.Caps {
	if r.wayland {
		return capture.Caps{}
	}
	return capture.Caps{Modes: []capture.Mode{capture.VideoFull, capture.VideoRegion}}
}

// Recording reports whether ffmpeg is currently running.
func (r *Recorder) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd != nil
}

// Start begins a full-desktop recording.
func (r *Recorder) Start(ctx context.Context, mode capture.Mode) error {
	return r.StartRegion(ctx, mode, image.Rectangle{})
}

// StartRegion begins recording, cropped to rect when non-empty. rect is in
// capture coordinates (relative to the first xinerama screen, like every
// rectangle the Capturer produces); x11grab wants root-window offsets, so the
// screen origin is added here.
func (r *Recorder) StartRegion(ctx context.Context, mode capture.Mode, rect image.Rectangle) error {
	switch mode {
	case capture.VideoFull, capture.VideoRegion:
	default:
		return fmt.Errorf("linux recorder: unsupported mode: %s", mode)
	}
	if r.wayland {
		return fmt.Errorf("linux recorder: screen recording is not available on Wayland sessions")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd != nil {
		return capture.ErrAlreadyRecording
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("linux recorder: ffmpeg not found; install it with your package manager: %w", err)
	}

	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}
	out := filepath.Join(os.TempDir(), fmt.Sprintf("goshareit_rec_%d.mp4", time.Now().UnixNano()))

	args := []string{
		"-y",
		"-f", "x11grab",
		"-framerate", recordFramerate,
	}
	input := display
	if !rect.Empty() {
		origin, err := primaryOrigin()
		if err != nil {
			return err
		}
		rect = rect.Add(origin)
		// libx264 with yuv420p needs even dimensions.
		w := rect.Dx() &^ 1
		h := rect.Dy() &^ 1
		if w <= 0 || h <= 0 {
			return fmt.Errorf("linux recorder: region too small after even-rounding: %dx%d", w, h)
		}
		args = append(args, "-video_size", fmt.Sprintf("%dx%d", w, h))
		input = fmt.Sprintf("%s+%d,%d", display, rect.Min.X, rect.Min.Y)
	}
	args = append(args,
		"-i", input,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		out,
	)
	cmd := exec.Command(ffmpeg, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("linux recorder: stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("linux recorder: start ffmpeg: %w", err)
	}

	r.cmd = cmd
	r.stdin = stdin
	r.outPath = out
	r.done = make(chan struct{})

	done := r.done
	go func() {
		err := cmd.Wait()
		r.mu.Lock()
		r.waitErr = err
		r.mu.Unlock()
		close(done)
	}()

	// Stop on context cancellation (host shutdown) so ffmpeg never outlives us.
	go func() {
		select {
		case <-ctx.Done():
			_, _ = r.Stop(context.Background())
		case <-done:
		}
	}()

	return nil
}

// Stop ends the recording and returns the mp4 bytes.
func (r *Recorder) Stop(ctx context.Context) (capture.Result, error) {
	r.mu.Lock()
	if r.cmd == nil {
		r.mu.Unlock()
		return capture.Result{}, capture.ErrNotRecording
	}
	cmd := r.cmd
	stdin := r.stdin
	out := r.outPath
	done := r.done
	r.mu.Unlock()

	// 'q' on stdin asks ffmpeg to finish cleanly (flush + write trailer).
	_, _ = io.WriteString(stdin, "q\n")
	_ = stdin.Close()

	killed := false
	select {
	case <-done:
	case <-time.After(stopQuitTimeout):
		_ = cmd.Process.Kill()
		killed = true
		<-done
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		killed = true
		<-done
	}

	r.mu.Lock()
	waitErr := r.waitErr
	r.cmd = nil
	r.stdin = nil
	r.outPath = ""
	r.done = nil
	r.mu.Unlock()

	if waitErr != nil && !killed {
		return capture.Result{}, fmt.Errorf("linux recorder: ffmpeg exited: %w", waitErr)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		return capture.Result{}, fmt.Errorf("linux recorder: read recording: %w", err)
	}
	if len(data) == 0 {
		return capture.Result{}, fmt.Errorf("linux recorder: recording is empty (ffmpeg may have been killed before finalizing)")
	}
	_ = os.Remove(out)

	return capture.Result{
		Bytes: data,
		Mime:  "video/mp4",
		Kind:  capture.KindVideo,
	}, nil
}
