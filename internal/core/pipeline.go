package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/capture"
	"github.com/Rake-Pro/GoShareIt/internal/core/edit"
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
	action := edit.ActionDefault
	if req.Edit && res.Kind == capture.KindImage {
		edited, a2, ok, eerr := a.editor.Edit(ctx, res, edit.Opts{CanUpload: a.UploadEnabled()})
		switch {
		case eerr != nil:
			a.log.Warn().Err(eerr).Msg("editor failed; using original capture")
		case ok:
			res = edited
			action = a2
		default:
			// skipped or cancelled: keep original res unchanged.
		}
	}

	return a.processResult(ctx, res, action)
}

// processResult runs after-capture -> name -> upload -> after-upload on an
// already-captured Result. It is shared by one-shot capture and recording stop.
// action carries an explicit per-capture override from the editor's action
// buttons (edit.ActionDefault when no editor ran, or the plain confirm button
// was used): ActionDefault follows the config-driven after-capture behavior
// (local save, clipboard image copy) below; ActionCopy/ActionSave/ActionUpload
// override it for this capture only.
func (a *App) processResult(ctx context.Context, res capture.Result, action edit.Action) (upload.UploadResult, error) {
	if len(res.Bytes) == 0 {
		return upload.UploadResult{}, fmt.Errorf("capture: empty result")
	}

	fname := name.Render(a.cfg.Upload.FilenameTemplate, extFor(res))

	switch action {
	case edit.ActionCopy:
		if err := a.clipboard.WriteImage(res.Bytes); err != nil {
			a.log.Warn().Err(err).Msg("copy image to clipboard failed")
		}
		return upload.UploadResult{}, a.finishLocalOnly(fname, res, fname+" (copied)", "capture complete (copied)")
	case edit.ActionSave:
		dir := a.cfg.AfterCapture.SaveDir
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return upload.UploadResult{}, fmt.Errorf("save local: home dir: %w", err)
			}
			dir = filepath.Join(home, "Pictures", "GoShareIt")
		}
		if err := a.saveLocal(res, dir); err != nil {
			return upload.UploadResult{}, err
		}
		return upload.UploadResult{}, a.finishLocalOnly(fname, res, fname+" (saved locally)", "capture complete (saved locally)")
	case edit.ActionUpload:
		if !a.UploadEnabled() {
			// The Upload button is greyed out in the UI when uploads are off;
			// if it still arrives, fall through to the normal local-only path
			// below instead of erroring.
			break
		}
		return a.upload(ctx, fname, res)
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

	// 4a. Local-only mode: uploads toggled off. History and notification still
	// happen; the after-upload URL copy has nothing to copy and is skipped.
	if !a.UploadEnabled() {
		body := fname
		if a.cfg.AfterCapture.SaveLocal {
			body = fname + " (saved locally)"
		}
		return upload.UploadResult{}, a.finishLocalOnly(fname, res, body, "capture complete (uploads disabled)")
	}

	return a.upload(ctx, fname, res)
}

// finishLocalOnly appends a name-only history entry and, per
// cfg.AfterUpload.Notify, fires a notification with the given body. It backs
// every no-upload outcome: uploads-disabled, and the editor's Copy/Save
// actions.
func (a *App) finishLocalOnly(fname string, res capture.Result, body, logMsg string) error {
	if err := a.history.Append(history.Entry{Name: fname, Time: time.Now()}); err != nil {
		a.log.Warn().Err(err).Msg("history append failed")
	}
	if a.cfg.AfterUpload.Notify && a.notifier != nil {
		n := notify.Notification{Title: "GoShareIt", Body: body, ThumbnailPath: res.Path}
		if err := a.notifier.Notify(n); err != nil {
			a.log.Warn().Err(err).Msg("notify failed")
		}
	}
	a.log.Info().Str("name", fname).Msg(logMsg)
	return nil
}

// upload runs the upload + after-upload steps (clipboard URL, history,
// notify) unchanged. Shared by the default config-driven path and the
// editor's explicit Upload action.
func (a *App) upload(ctx context.Context, fname string, res capture.Result) (upload.UploadResult, error) {
	up, err := a.uploader.Upload(ctx, fname, bytes.NewReader(res.Bytes), int64(len(res.Bytes)), res.Mime)
	if err != nil {
		return upload.UploadResult{}, fmt.Errorf("upload: %w", err)
	}

	// After-upload: clipboard URL, history, notify.
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
