//go:build windows

package windows

import (
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
)

// recordFramerate is the gdigrab capture rate. 30fps balances smoothness and
// file size for typical share-a-clip usage.
const recordFramerate = "30"

// stopQuitTimeout bounds how long Stop waits for ffmpeg to exit after receiving
// 'q' on stdin before falling back to a hard terminate. A clean 'q' lets ffmpeg
// flush and write the moov atom so the mp4 is playable; killing it does not, so
// the kill path is a last resort that may yield a truncated file.
const stopQuitTimeout = 10 * time.Second

// Recorder records the screen to an mp4 by shelling out to ffmpeg's gdigrab
// input. It implements capture.Recorder. Windows has no first-party screen
// recorder we can drive headlessly (the Game Bar / Xbox capture API is not
// scriptable), so ffmpeg is the portable choice. Ships need a bundled or
// PATH-installed ffmpeg; see Start for the discovery error.
type Recorder struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	outPath string
	// done is closed by the wait goroutine when cmd exits; waitErr holds the
	// process exit error observed there.
	done    chan struct{}
	waitErr error
}

// NewRecorder returns an ffmpeg/gdigrab backed Recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// Capabilities advertises the video modes this Recorder supports. Both full
// desktop and region recording are wired: VideoRegion crops via gdigrab's
// -offset_x/-offset_y/-video_size flags when StartRegion is given a non-empty
// rect. GIF output is deferred to P3b.
func (r *Recorder) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{capture.VideoFull, capture.VideoRegion}}
}

// Recording reports whether a recording is currently in progress.
func (r *Recorder) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd != nil
}

// Start spawns ffmpeg to record the full desktop to a temp mp4. It returns once
// the process is launched; it does not block for the recording duration. It is
// equivalent to StartRegion with an empty rect.
//
// mode handling: VideoFull records the whole virtual desktop. VideoRegion
// records the full desktop unless a rect is supplied via StartRegion.
func (r *Recorder) Start(ctx context.Context, mode capture.Mode) error {
	return r.StartRegion(ctx, mode, image.Rectangle{})
}

// StartRegion spawns ffmpeg to record either the full desktop (empty rect) or a
// sub-rectangle of it. rect is in screen pixel coordinates with a top-left
// origin; an empty rect (rect.Empty()) records the whole virtual desktop,
// matching Start's behavior.
//
// For a non-empty rect, gdigrab crops with -offset_x/-offset_y (the top-left
// origin of the grab) and -video_size WxH. Because the output pixel format is
// yuv420p, both width and height must be even; odd dimensions are rounded DOWN
// to the nearest even value (libx264 rejects odd dimensions for yuv420p). The
// rounding can drop up to one pixel off the right/bottom edge of the selection.
func (r *Recorder) StartRegion(ctx context.Context, mode capture.Mode, rect image.Rectangle) error {
	switch mode {
	case capture.VideoFull, capture.VideoRegion:
		// supported
	default:
		return fmt.Errorf("windows recorder: unsupported mode: %s", mode)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd != nil {
		return capture.ErrAlreadyRecording
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("windows recorder: ffmpeg not found; install ffmpeg or bundle it: %w", err)
	}

	out := filepath.Join(os.TempDir(), fmt.Sprintf("goshareit_rec_%d.mp4", time.Now().UnixNano()))

	// gdigrab -i desktop grabs the whole virtual desktop. ultrafast/yuv420p keep
	// CPU low and produce a widely playable H.264 mp4. -y overwrites the temp.
	args := []string{
		"-y",
		"-f", "gdigrab",
		"-framerate", recordFramerate,
	}
	if !rect.Empty() {
		// yuv420p requires even dimensions; round width/height down to even.
		w := rect.Dx() &^ 1
		h := rect.Dy() &^ 1
		if w <= 0 || h <= 0 {
			return fmt.Errorf("windows recorder: region too small after even-rounding: %dx%d", w, h)
		}
		args = append(args,
			"-offset_x", strconv.Itoa(rect.Min.X),
			"-offset_y", strconv.Itoa(rect.Min.Y),
			"-video_size", fmt.Sprintf("%dx%d", w, h),
		)
	}
	args = append(args,
		"-i", "desktop",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		out,
	)
	// ctx is NOT passed to CommandContext: cancellation must trigger a clean 'q'
	// stop (handled below), not the hard SIGKILL that exec.CommandContext sends,
	// which would corrupt the mp4. We watch ctx separately.
	cmd := exec.Command(ffmpeg, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("windows recorder: stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("windows recorder: start ffmpeg: %w", err)
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

	// If the caller's context is cancelled while recording, stop cleanly so the
	// mp4 is finalized rather than left as a dangling ffmpeg process.
	go func() {
		select {
		case <-ctx.Done():
			_, _ = r.Stop(context.Background())
		case <-done:
		}
	}()

	return nil
}

// Stop ends the recording and returns the finalized mp4. It writes 'q' to
// ffmpeg's stdin (ffmpeg's documented graceful-quit key), which makes ffmpeg
// flush buffers and write the moov atom so the file is seekable/playable, then
// waits for the process to exit. Only if the graceful quit does not complete
// within stopQuitTimeout does it fall back to Process.Kill, which can leave the
// mp4 truncated.
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

	// Request a clean quit. Writing 'q' is preferred over signals on Windows,
	// where there is no SIGINT delivery to a child the way there is on Unix.
	_, _ = io.WriteString(stdin, "q\n")
	_ = stdin.Close()

	killed := false
	select {
	case <-done:
		// ffmpeg exited; mp4 finalized.
	case <-time.After(stopQuitTimeout):
		// Graceful quit failed - hard kill as a last resort (may truncate mp4).
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

	// A clean 'q' quit makes ffmpeg exit 0. A killed process reports a non-nil
	// error which we ignore (we deliberately killed it); any other non-nil error
	// from a non-killed run is a real failure.
	if waitErr != nil && !killed {
		return capture.Result{}, fmt.Errorf("windows recorder: ffmpeg exited: %w", waitErr)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		return capture.Result{}, fmt.Errorf("windows recorder: read recording: %w", err)
	}
	if len(data) == 0 {
		return capture.Result{}, fmt.Errorf("windows recorder: recording is empty (ffmpeg may have been killed before finalizing)")
	}
	_ = os.Remove(out) // bytes are in memory; don't leave the temp mp4 behind

	return capture.Result{
		Bytes: data,
		Mime:  "video/mp4",
		Kind:  capture.KindVideo,
	}, nil
}
