// Package fake provides in-memory implementations of the core seams for tests
// and for the linux build (which has no real OS backend).
package fake

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
	"github.com/Rake-Pro/GoShareIt/internal/core/tray"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

// Capturer returns a fixed PNG result.
type Capturer struct {
	Result capture.Result
	Err    error
	Calls  []capture.Request
	mu     sync.Mutex
}

// NewCapturer returns a Capturer producing a tiny fake PNG.
func NewCapturer() *Capturer {
	return &Capturer{Result: capture.Result{
		Bytes: []byte("\x89PNG\r\n\x1a\nfake"),
		Mime:  "image/png",
		Kind:  capture.KindImage,
	}}
}

func (c *Capturer) Capture(_ context.Context, r capture.Request) (capture.Result, error) {
	c.mu.Lock()
	c.Calls = append(c.Calls, r)
	c.mu.Unlock()
	if c.Err != nil {
		return capture.Result{}, c.Err
	}
	return c.Result, nil
}

func (c *Capturer) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{
		capture.RegionInteractive, capture.FullScreen, capture.ActiveWindow,
	}}
}

// Recorder is an in-memory Recorder state machine for tests and the linux
// build. Start sets recording=true and arms a fake video Result; Stop returns
// it and clears recording.
type Recorder struct {
	Result    capture.Result
	StartErr  error
	StopErr   error
	mu        sync.Mutex
	recording bool
	Starts    []capture.Mode
}

// NewRecorder returns a Recorder producing a tiny fake mp4.
func NewRecorder() *Recorder {
	return &Recorder{Result: capture.Result{
		Bytes: []byte("\x00\x00\x00\x18ftypmp42fakevideo"),
		Mime:  "video/mp4",
		Kind:  capture.KindVideo,
	}}
}

func (r *Recorder) Start(_ context.Context, mode capture.Mode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.StartErr != nil {
		return r.StartErr
	}
	if r.recording {
		return capture.ErrAlreadyRecording
	}
	r.recording = true
	r.Starts = append(r.Starts, mode)
	return nil
}

func (r *Recorder) Stop(_ context.Context) (capture.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.StopErr != nil {
		return capture.Result{}, r.StopErr
	}
	if !r.recording {
		return capture.Result{}, capture.ErrNotRecording
	}
	r.recording = false
	return r.Result, nil
}

func (r *Recorder) Recording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

func (r *Recorder) Capabilities() capture.Caps {
	return capture.Caps{Modes: []capture.Mode{capture.VideoRegion, capture.VideoFull}}
}

// Uploader records uploads and returns canned links.
type Uploader struct {
	Result upload.UploadResult
	Err    error

	mu     sync.Mutex
	Names  []string
	Bodies [][]byte
	Mimes  []string
}

// NewUploader returns an Uploader with canned share links.
func NewUploader() *Uploader {
	return &Uploader{Result: upload.UploadResult{
		PublicURL:  "https://example.test/s/faketoken",
		DirectURL:  "https://example.test/s/faketoken/download",
		ShareToken: "faketoken",
	}}
}

func (u *Uploader) Upload(_ context.Context, name string, body io.Reader, _ int64, mime string) (upload.UploadResult, error) {
	b, _ := io.ReadAll(body)
	u.mu.Lock()
	u.Names = append(u.Names, name)
	u.Bodies = append(u.Bodies, b)
	u.Mimes = append(u.Mimes, mime)
	u.mu.Unlock()
	if u.Err != nil {
		return upload.UploadResult{}, u.Err
	}
	return u.Result, nil
}

// Clipboard is an in-memory clipboard.
type Clipboard struct {
	mu    sync.Mutex
	Text  string
	Image []byte
}

func (c *Clipboard) WriteText(s string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Text = s
	return nil
}

func (c *Clipboard) WriteImage(png []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Image = append([]byte(nil), png...)
	return nil
}

func (c *Clipboard) ReadImage() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.Image) == 0 {
		return nil, false
	}
	return append([]byte(nil), c.Image...), true
}

func (c *Clipboard) ReadText() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Text == "" {
		return "", false
	}
	return c.Text, true
}

// Notifier records notifications.
type Notifier struct {
	mu            sync.Mutex
	Notifications []notify.Notification
}

func (n *Notifier) Notify(notification notify.Notification) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Notifications = append(n.Notifications, notification)
	return nil
}

// Count returns how many notifications were recorded.
func (n *Notifier) Count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Notifications)
}

// Tray is a no-op tray that blocks until ctx is done.
type Tray struct{}

func (Tray) Run(ctx context.Context, _ tray.MenuSpec) error {
	<-ctx.Done()
	return ctx.Err()
}

// HotkeyManager records registrations and can fire them.
type HotkeyManager struct {
	mu   sync.Mutex
	fns  map[string]func()
	Keys map[string]string
}

// NewHotkeyManager returns an empty manager.
func NewHotkeyManager() *HotkeyManager {
	return &HotkeyManager{fns: map[string]func(){}, Keys: map[string]string{}}
}

func (h *HotkeyManager) Register(id, keys string, fn func()) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.fns[id]; ok {
		return fmt.Errorf("hotkey %q already registered", id)
	}
	h.fns[id] = fn
	h.Keys[id] = keys
	return nil
}

func (h *HotkeyManager) Unregister(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.fns, id)
	delete(h.Keys, id)
}

func (h *HotkeyManager) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Fire invokes a registered hotkey by id (test helper).
func (h *HotkeyManager) Fire(id string) {
	h.mu.Lock()
	fn := h.fns[id]
	h.mu.Unlock()
	if fn != nil {
		fn()
	}
}
