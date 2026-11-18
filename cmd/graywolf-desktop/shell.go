package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/chrissnell/graywolf/pkg/app"
)

const statusPollInterval = 10 * time.Second

// shell owns the window, tray and station lifecycle.
type shell struct {
	cfg     app.Config
	logger  *slog.Logger
	token   string
	dataDir string
	state   *desktopState
	trayOK  bool

	wapp   *application.App
	window *application.WebviewWindow
	status *application.MenuItem // nil without a tray
	tray   *application.SystemTray

	ctx            context.Context
	cancel         context.CancelFunc
	bound          chan net.Addr
	runDone        chan error
	stationStarted atomic.Bool
	quitOnce       sync.Once
	stopOnce       sync.Once
}

func (s *shell) showWindow() {
	s.window.Show()
	s.window.Restore()
	s.window.Focus()
}

func (s *shell) setStatus(line string) {
	if s.status != nil {
		s.status.SetLabel(line)
	}
	if s.tray != nil {
		s.tray.SetTooltip("Graywolf - " + line)
	}
}

// onWindowClosing hides to the tray, or quits when there is no tray
// (otherwise the station would keep running with no way to reach it).
func (s *shell) onWindowClosing(e *application.WindowEvent) {
	e.Cancel()
	if !s.trayOK {
		go s.quit()
		return
	}
	s.window.Hide()
	if s.state.markCloseNoticeShown() {
		s.wapp.Dialog.Info().
			SetTitle("Graywolf is still running").
			SetMessage("Graywolf keeps running in the system tray so your station stays on the air.\n\n" +
				"Click the tray icon to open it again, or choose Quit Graywolf from the tray menu to stop the station.").
			Show()
	}
}

func (s *shell) buildTray() {
	if !s.trayOK {
		return
	}
	menu := application.NewMenu()
	s.status = menu.Add("Starting...").SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Open Graywolf").OnClick(func(*application.Context) { s.showWindow() })

	enabled, err := s.wapp.Autostart.IsEnabled()
	if err != nil {
		s.logger.Warn("desktop: autostart status unavailable", "err", err)
	} else {
		menu.AddCheckbox("Start Graywolf when I log in", enabled).OnClick(func(c *application.Context) {
			s.setAutostart(c.ClickedMenuItem())
		})
	}
	menu.AddSeparator()
	menu.Add("Quit Graywolf").OnClick(func(*application.Context) { go s.quit() })

	s.tray = s.wapp.SystemTray.New()
	s.tray.SetIcon(iconPNG)
	s.tray.SetTooltip("Graywolf")
	s.tray.SetMenu(menu)
	s.tray.OnClick(s.showWindow)
}

// setAutostart applies the checkbox's new state. Autostart launches with
// -hidden so the app starts quietly in the tray.
func (s *shell) setAutostart(item *application.MenuItem) {
	want := item.Checked()
	var err error
	if want {
		err = s.wapp.Autostart.EnableWithOptions(application.AutostartOptions{
			Arguments: []string{hiddenFlag},
		})
	} else {
		err = s.wapp.Autostart.Disable()
	}
	if err != nil {
		s.logger.Error("desktop: change autostart", "enable", want, "err", err)
		item.SetChecked(!want)
		s.wapp.Dialog.Error().SetTitle("Graywolf").
			SetMessage(fmt.Sprintf("Could not change the start-at-login setting:\n\n%v", err)).Show()
		return
	}
	s.logger.Info("desktop: autostart changed", "enabled", want)
}

// start runs once the GUI is up. If a Graywolf station is already
// running on this machine (typically the systemd / NSSM service), the
// window shows that station instead of starting a second one: two
// stations on one radio and sound card interfere, and they would fight
// over the same config DB. No blocking choice dialog on purpose -- Wails
// dialogs fire no callback when dismissed, which would hang startup.
func (s *shell) start(probeAddr string) {
	if target, ok := probeTarget(probeAddr); ok {
		if existing, found := probeExistingStation(s.ctx, target); found {
			s.attachToExisting(existing)
			return
		}
	}
	s.startStation()
}

func (s *shell) attachToExisting(existing string) {
	s.logger.Info("desktop: a station is already running; showing it instead of starting another", "url", existing)
	s.setStatus("Showing the station already running at " + existing)
	s.window.SetURL(existing)
	s.showWindow()
	s.wapp.Dialog.Info().
		SetTitle("Graywolf is already running").
		SetMessage(fmt.Sprintf("A Graywolf station is already running on this computer at %s, "+
			"for example as a background service. This window shows that station.\n\n"+
			"To run the station from this app instead, stop that service and start Graywolf again.", existing)).
		Show()
}

func (s *shell) startStation() {
	s.stationStarted.Store(true)
	go func() {
		s.runDone <- app.New(s.cfg, s.logger).Run(s.ctx)
	}()

	select {
	case addr := <-s.bound:
		base := "http://" + addr.String()
		u := url.URL{
			Scheme:   "http",
			Host:     addr.String(),
			Path:     "/",
			RawQuery: url.Values{"gwdesktop": {s.token}}.Encode(),
		}
		s.logger.Info("desktop: station listening", "addr", addr.String())
		s.window.SetURL(u.String())
		if s.trayOK {
			go pollStatus(s.ctx, base, s.token, statusPollInterval, s.setStatus)
		}
	case err := <-s.runDone:
		// Station failed before the HTTP listener came up.
		s.logger.Error("graywolf failed to start", "err", err)
		s.showWindow()
		s.wapp.Dialog.Error().SetTitle("Graywolf").
			SetMessage(fmt.Sprintf("Graywolf could not start:\n\n%v\n\nLog: %s", err,
				filepath.Join(s.dataDir, "graywolf-desktop.log"))).
			Show()
		s.runDone <- err // let quit() observe it
		go s.quit()
	}
}

// stopStation cancels the station and waits (bounded) for it to stop.
// Idempotent; safe from any goroutine, including the Windows message
// loop during WM_ENDSESSION.
func (s *shell) stopStation() {
	s.stopOnce.Do(func() {
		s.cancel()
		if !s.stationStarted.Load() {
			return
		}
		select {
		case err := <-s.runDone:
			if err != nil {
				s.logger.Error("graywolf exited with error", "err", err)
			}
		case <-time.After(s.cfg.ShutdownTimeout + 5*time.Second):
			s.logger.Error("desktop: station did not stop in time")
		}
	})
}

// quitWatchdog bounds how long the GUI may take to go away after the
// station has stopped. An open modal dialog runs its own GTK/Win32 loop
// and can keep Wails' quit from ever finishing; without this the process
// would outlive logout/shutdown.
const quitWatchdog = 5 * time.Second

// quit stops the station first, then the GUI. Safe to call from any
// goroutine and more than once.
func (s *shell) quit() {
	s.quitOnce.Do(func() {
		s.logger.Info("desktop: quit requested")
		s.stopStation()
		time.AfterFunc(quitWatchdog, func() {
			s.logger.Warn("desktop: GUI did not exit after quit; forcing exit")
			os.Exit(0)
		})
		s.wapp.Quit()
	})
}

func (s *shell) registerHooks(probeAddr string) {
	s.window.RegisterHook(events.Common.WindowClosing, s.onWindowClosing)
	s.wapp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go s.start(probeAddr)
	})
}
