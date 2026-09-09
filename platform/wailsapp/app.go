//go:build darwin || windows

// Package wailsapp implements the tray, global-hotkey, notification and
// confirm-dialog seams on top of a single Wails v3 application.
//
// MAIN-LOOP OWNERSHIP: exactly one *application.App exists per process, and
// Tray.Run drives it with app.Run() - which must be called from the main
// goroutine, because macOS pins AppKit to the process's first thread. Every
// other call in this package marshals onto that loop through the Wails
// managers, so the tray, the global shortcuts, the dialogs and the
// notification service all share one event loop instead of the separate ones
// the fyne.io/systray + golang.design/x/hotkey + osascript/PowerShell stack
// needed.
//
// STARTUP ORDER: managers may be driven before app.Run() - the systray and any
// global shortcut registered early are queued and applied when the app starts.
// Dialogs and notifications are not: they reach into the platform app, which
// only exists once Run has begun, so those calls wait for the
// ApplicationStarted event first (see waitStarted).
package wailsapp

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// startupWait bounds how long a dialog or notification blocks waiting for the
// application to start. Reaching it means app.Run was never called (or failed),
// so the caller gets an error rather than hanging forever.
const startupWait = 30 * time.Second

// Provider owns the Wails application and the four seams built on it. Create
// exactly one per process.
type Provider struct {
	app   *application.App
	notif *notifications.NotificationService

	started   chan struct{}
	startOnce sync.Once

	authOnce   sync.Once
	notifySeq  atomic.Uint64
	dialogOpen atomic.Bool // one confirm dialog at a time

	tray      *Tray
	hotkeys   *Manager
	notifier  *Notifier
	confirmer *Confirmer
}

// New builds the Wails application and the seams over it. It does not start
// the app; Tray.Run does that.
func New() *Provider {
	p := &Provider{
		started: make(chan struct{}),
		notif:   notifications.New(),
	}
	p.app = application.New(application.Options{
		Name:        "GoShareIt",
		Description: "Screenshot and screen recording",
		Assets:      application.AlphaAssets,
		Services:    []application.Service{application.NewService(p.notif)},
		// Menu-bar / notification-area app: no dock icon, and no window is
		// ever created, so nothing may quit the app on "last window closed".
		Mac:     application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		// The host installs its own SIGINT/SIGTERM handling in main.go and
		// shuts down by cancelling the run context; a second handler inside
		// Wails would quit the app behind that path's back.
		DisableDefaultSignalHandler: true,
		LogLevel:                    slog.LevelWarn,
		ErrorHandler: func(err error) {
			// Deferred global-shortcut registrations report OS rejections here
			// (Register returned before the app was running), as does anything
			// else Wails cannot surface to a caller.
			log.Warn().Err(err).Msg("wails")
		},
		WarningHandler: func(msg string) { log.Debug().Str("warning", msg).Msg("wails") },
	})
	p.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		p.startOnce.Do(func() { close(p.started) })
		// Off the event goroutine: on macOS the consent sheet blocks until the
		// user answers it.
		go p.requestAuthorization()
	})
	p.notif.OnNotificationResponse(p.handleNotificationResponse)

	p.tray = &Tray{p: p, items: map[string]*application.MenuItem{}}
	p.hotkeys = &Manager{p: p, accels: map[string]string{}}
	p.notifier = &Notifier{p: p}
	p.confirmer = &Confirmer{p: p}
	return p
}

// Tray returns the system-tray seam. Its Run owns the main loop.
func (p *Provider) Tray() *Tray { return p.tray }

// Hotkeys returns the global-hotkey seam.
func (p *Provider) Hotkeys() *Manager { return p.hotkeys }

// Notifier returns the desktop-notification seam.
func (p *Provider) Notifier() *Notifier { return p.notifier }

// Confirmer returns the blocking confirm-dialog seam.
func (p *Provider) Confirmer() *Confirmer { return p.confirmer }

// ReconcileAutostart makes the OS login-item registration match enabled
// (SMAppService or a LaunchAgent on macOS, the HKCU Run key on Windows). It is
// best-effort: a failure is logged and never fatal, because a login item that
// could not be written must not stop the app from running now.
func (p *Provider) ReconcileAutostart(enabled bool) {
	var err error
	if enabled {
		err = p.app.Autostart.Enable()
	} else {
		err = p.app.Autostart.Disable()
	}
	if err != nil {
		log.Warn().Err(err).Bool("enabled", enabled).Msg("start at login: could not apply the setting")
		return
	}
	log.Debug().Bool("enabled", enabled).Msg("start at login reconciled")
}

// waitStarted blocks until the application is running, or d elapses. It
// reports whether the app is running.
func (p *Provider) waitStarted(d time.Duration) bool {
	select {
	case <-p.started:
		return true
	default:
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.started:
		return true
	case <-t.C:
		return false
	}
}

// running reports whether the application has started, without blocking.
func (p *Provider) running() bool {
	select {
	case <-p.started:
		return true
	default:
		return false
	}
}

// onMainThread runs fn on the application's main thread once the app is
// running. Before that there is no main loop to dispatch onto and the Wails
// objects are still plain Go state, so fn runs inline.
func (p *Provider) onMainThread(fn func()) {
	if !p.running() {
		fn()
		return
	}
	application.InvokeAsync(fn)
}
