package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/history"
	"github.com/Rake-Pro/GoShareIt/internal/core/name"
	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
	"github.com/Rake-Pro/GoShareIt/internal/core/upload"
)

// runPipeline: capture -> processResult. It is the entry point for one-shot
// captures.
func (a *App) runPipeline(ctx context.Context, req capture.Request) (upload.UploadResult, error) {
	res, err := a.capturer.Capture(ctx, req)
	if err != nil {
		return upload.UploadResult{}, fmt.Errorf("capture: %w", err)
	}

	// 1b. Optional edit (gated by config + per-request flag). Fail-open: an
	// editor error never aborts the capture; the original image flows on.
	if req.Edit && res.Kind == capture.KindImage {
		edited, ok, eerr := a.editor.Edit(ctx, res)
		switch {
		case eerr != nil:
			a.log.Warn().Err(eerr).Msg("editor failed; using original capture")
		case ok:
			res = edited
		default:
			// skipped or cancelled: keep original res unchanged.
		}
	}

	return a.processResult(ctx, res)
}

// processResult runs after-capture -> name -> upload -> after-upload on an
// already-captured Result. It is shared by one-shot capture and recording stop.
// After-capture behavior (local save, clipboard image copy) is driven by config.
func (a *App) processResult(ctx context.Context, res capture.Result) (upload.UploadResult, error) {
	if len(res.Bytes) == 0 {
		return upload.UploadResult{}, fmt.Errorf("capture: empty result")
	}

	// 2. After-capture: optional local save + clipboard image copy.
	if a.cfg.AfterCapture.SaveLocal {
		if err := a.saveLocal(res, a.cfg.AfterCapture.SaveDir); err != nil {
			return upload.UploadResult{}, err
		}
	}
	if a.cfg.AfterCapture.CopyImageToClipboard && res.Kind == capture.KindImage {
		if err := a.clipboard.WriteImage(res.Bytes); err != nil {
			a.log.Warn().Err(err).Msg("copy image to clipboard failed")
		}
	}

	// 3. Name.
	fname := name.Render(a.cfg.Upload.FilenameTemplate, extFor(res))

	// 4a. Local-only mode: uploads toggled off. History and notification still
	// happen; the after-upload URL copy has nothing to copy and is skipped.
	if !a.UploadEnabled() {
		if err := a.history.Append(history.Entry{Name: fname, Time: time.Now()}); err != nil {
			a.log.Warn().Err(err).Msg("history append failed")
		}
		if a.cfg.AfterUpload.Notify && a.notifier != nil {
			body := fname
			if a.cfg.AfterCapture.SaveLocal {
				body = fname + " (saved locally)"
			}
			n := notify.Notification{Title: "GoShareIt", Body: body, ThumbnailPath: res.Path}
			if err := a.notifier.Notify(n); err != nil {
				a.log.Warn().Err(err).Msg("notify failed")
			}
		}
		a.log.Info().Str("name", fname).Msg("capture complete (uploads disabled)")
		return upload.UploadResult{}, nil
	}

	// 4. Upload.
	up, err := a.uploader.Upload(ctx, fname, bytes.NewReader(res.Bytes), int64(len(res.Bytes)), res.Mime)
	if err != nil {
		return upload.UploadResult{}, fmt.Errorf("upload: %w", err)
	}

	// 5. After-upload: clipboard URL, history, notify.
	linkToCopy := up.DirectURL
	if !a.cfg.Upload.DirectLink {
		linkToCopy = up.PublicURL
	}
	if a.cfg.AfterUpload.CopyURLToClipboard {
		if err := a.clipboard.WriteText(linkToCopy); err != nil {
			a.log.Warn().Err(err).Msg("copy url to clipboard failed")
		}
	}

	if err := a.history.Append(history.Entry{
		Name:       fname,
		Time:       time.Now(),
		PublicURL:  up.PublicURL,
		DirectURL:  up.DirectURL,
		ShareToken: up.ShareToken,
	}); err != nil {
		a.log.Warn().Err(err).Msg("history append failed")
	}

	if a.cfg.AfterUpload.Notify && a.notifier != nil {
		n := notify.Notification{
			Title:         "GoShareIt",
			Body:          fname,
			ThumbnailPath: res.Path,
			OpenURL:       up.PublicURL,
		}
		if err := a.notifier.Notify(n); err != nil {
			a.log.Warn().Err(err).Msg("notify failed")
		}
	}

	a.log.Info().Str("name", fname).Str("url", linkToCopy).Msg("upload complete")
	return up, nil
}

func (a *App) saveLocal(res capture.Result, dir string) error {
	if dir == "" {
		return fmt.Errorf("save_local enabled but save_dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save local: mkdir: %w", err)
	}
	fname := name.Render(a.cfg.Upload.FilenameTemplate, extFor(res))
	path := filepath.Join(dir, fname)
	if err := os.WriteFile(path, res.Bytes, 0o644); err != nil {
		return fmt.Errorf("save local: write: %w", err)
	}
	res.Path = path
	return nil
}

func extFor(res capture.Result) string {
	switch res.Mime {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	}
	if res.Kind == capture.KindVideo {
		return "mp4"
	}
	return "png"
}
