//go:build darwin || windows

package wailsapp

import (
	"fmt"
	"runtime"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/Rake-Pro/GoShareIt/internal/core/notify"
)

// confirmTimeout bounds how long a caller waits for an answer. Wails exposes no
// way to dismiss a MessageDialog it has shown, so unlike the NSAlert path this
// replaces, the dialog itself is NOT cancelled at the deadline: it stays on
// screen until the user answers. The timeout only releases the waiting
// goroutine with a "no", and the one-dialog guard below stays held until the
// stale dialog is finally answered.
const confirmTimeout = 120 * time.Second

// Notifier posts desktop notifications through the Wails notification service
// (UNUserNotificationCenter on macOS, Windows toasts on Windows).
type Notifier struct{ p *Provider }

// Notify shows a non-blocking banner. When the notification carries an
// OpenURL, the URL rides along as notification data and clicking the banner
// opens it in the default browser (see handleNotificationResponse).
//
// ThumbnailPath is still ignored: attaching it needs the file to survive until
// the notification is delivered, which the capture pipeline does not promise.
func (n *Notifier) Notify(notification notify.Notification) error {
	if !n.p.waitStarted(startupWait) {
		return fmt.Errorf("notify: the application is not running")
	}

	title := notification.Title
	if title == "" {
		title = "GoShareIt"
	}
	options := notifications.NotificationOptions{
		ID:    "goshareit-" + strconv.FormatUint(n.p.notifySeq.Add(1), 10),
		Title: title,
		Body:  notification.Body,
	}
	if notification.OpenURL != "" {
		options.Data = map[string]any{"url": notification.OpenURL}
	}
	if err := n.p.notif.SendNotification(options); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// requestAuthorization asks for notification permission once per process, from
// its own goroutine right after the application starts. It is never called on
// the Notify path: on macOS the request blocks on the consent sheet until the
// user answers (minutes, in the worst case), which would stall the capture
// pipeline behind it. Notify sends regardless and reports what the OS says.
func (p *Provider) requestAuthorization() {
	p.authOnce.Do(func() {
		granted, err := p.notif.RequestNotificationAuthorization()
		switch {
		case err != nil:
			log.Warn().Err(err).Msg("notifications: authorization request failed")
		case !granted:
			log.Warn().Msg("notifications: not authorized - allow GoShareIt in the system notification settings")
		default:
			log.Debug().Msg("notifications authorized")
		}
	})
}

// handleNotificationResponse opens the link a notification was posted with when
// the user clicks the banner body. Action buttons are not used, so anything
// other than the default action is ignored.
func (p *Provider) handleNotificationResponse(result notifications.NotificationResult) {
	if result.Error != nil {
		log.Debug().Err(result.Error).Msg("notification response")
		return
	}
	if result.Response.ActionIdentifier != notifications.DefaultActionIdentifier {
		return
	}
	url, _ := result.Response.UserInfo["url"].(string)
	if url == "" {
		return
	}
	if err := p.app.Browser.OpenURL(url); err != nil {
		log.Warn().Err(err).Msg("notification click: could not open the link")
	}
}

// Confirmer shows blocking native two-button dialogs on the application's main
// thread.
type Confirmer struct{ p *Provider }

// Confirm shows a two-button dialog and blocks until the user answers or
// confirmTimeout elapses; the timeout, Escape and the cancel button are all a
// plain "no", not an error.
//
// The dialog is modal on the main loop and stays there until it is answered:
// the timeout releases this caller, it does not close the alert, because Wails
// offers no way to dismiss a MessageDialog it has shown. Until that stale
// dialog is answered, further Confirm calls fail with "another dialog is
// already open".
func (c *Confirmer) Confirm(title, body, okLabel, cancelLabel string) (bool, error) {
	if !c.p.waitStarted(startupWait) {
		return false, fmt.Errorf("confirm: the application is not running")
	}
	// One alert at a time: a second modal on top of the first is what the
	// NSAlert path used to refuse outright, and on Windows a second MessageBox
	// would sit behind the first with no way to reach it.
	if !c.p.dialogOpen.CompareAndSwap(false, true) {
		return false, fmt.Errorf("confirm: another dialog is already open")
	}
	if runtime.GOOS == "windows" {
		// Windows renders a question dialog as a Win32 MessageBox with the
		// fixed Yes/No button set, and Wails matches the pressed button back by
		// label - custom labels would never match, and the callback would never
		// fire. okLabel/cancelLabel stay advisory there, exactly as they were
		// under the PowerShell MessageBox this replaces.
		okLabel, cancelLabel = "Yes", "No"
	}

	// Buffered so the button callback never blocks on the main thread, even if
	// this goroutine has already given up waiting. The guard is released by
	// whichever button fires, so a timed-out dialog still frees it once the
	// user gets to it.
	answer := make(chan bool, 1)
	reply := func(v bool) func() {
		return func() {
			answer <- v
			c.p.dialogOpen.Store(false)
		}
	}
	dialog := c.p.app.Dialog.Question().SetTitle(title).SetMessage(body)
	dialog.AddButton(okLabel).SetAsDefault().OnClick(reply(true))
	dialog.AddButton(cancelLabel).SetAsCancel().OnClick(reply(false))
	// Show dispatches to the main thread and waits for that dispatch to
	// return. On Windows the platform dialog is a blocking MessageBox that
	// runs inside it, so Show only returns once the user has answered - from
	// this goroutine, or the tray, the hotkeys and Quit would all be frozen
	// behind it and the timeout below could never fire.
	go dialog.Show()

	timer := time.NewTimer(confirmTimeout)
	defer timer.Stop()
	select {
	case ok := <-answer:
		return ok, nil
	case <-timer.C:
		return false, nil
	}
}
