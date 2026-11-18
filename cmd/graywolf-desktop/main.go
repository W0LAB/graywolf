// Command graywolf-desktop is the native desktop shell for Graywolf on
// Windows and Linux (Phase 0 spike -- see
// .context/2026-11-17-native-desktop-shell.md).
//
// It runs the same app.App as the headless `graywolf` binary, in
// process. That App spawns graywolf-modem (sibling binary) exactly as it
// does headless, so audio / PTT / DSP are unchanged. The SPA is shown in
// a native window (WebView2 on Windows, WebKitGTK on Linux) pointed at a
// loopback-only listener on an OS-chosen port, guarded by a per-launch
// bearer token -- the same model the Android app uses.
//
// The token reaches the SPA through the initial URL (?gwdesktop=...),
// which web/src/lib/desktopBridge.js captures and strips at boot. It is
// deliberately NOT exposed as GraywolfWebInterface: that global means
// "Android" to the SPA.
package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/chrissnell/graywolf/pkg/app"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
)

//go:embed icon.png
var iconPNG []byte

const singleInstanceID = "com.nw5w.graywolf.desktop"

const startingHTML = `<!doctype html><html><head><meta charset="utf-8">
<style>html,body{height:100%;margin:0;background:#1b1d21;color:#c9ccd1;
font:15px system-ui,sans-serif;display:flex;align-items:center;justify-content:center}</style>
</head><body>Starting Graywolf&hellip;</body></html>`

func main() {
	cfg, err := app.ParseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	cfg.Version = Version
	cfg.GitCommit = GitCommit

	dataDir, err := applyDesktopDefaults(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger, closeLog := setupLogger(cfg, dataDir)
	defer closeLog()
	slog.SetDefault(logger)

	// Loopback only, OS-chosen port. Phase 4 adds an opt-in LAN listener.
	cfg.HTTPAddr = "127.0.0.1:0"
	token, err := newToken()
	if err != nil {
		logger.Error("generate launch token", "err", err)
		os.Exit(1)
	}
	cfg.BearerToken = token

	bound := make(chan net.Addr, 1)
	cfg.OnHTTPListenerBound = func(a net.Addr) { bound <- a }

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)

	wapp := application.New(application.Options{
		Name:        "Graywolf",
		Description: "APRS / AX.25 packet radio station",
		Icon:        iconPNG,
		// Without these, Wails' own fatal errors (e.g. single-instance
		// lock failure) exit the process with no output at all.
		Logger: logger.With("component", "wails"),
		ErrorHandler: func(err error) {
			logger.Error("desktop: wails error", "err", err)
		},
		SingleInstance: singleInstanceOptions(logger),
		Linux: application.LinuxOptions{
			ProgramName:                   "graywolf",
			DisableQuitOnLastWindowClosed: true,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			WebviewUserDataPath:           filepath.Join(dataDir, "webview"),
		},
	})

	window := wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "Graywolf",
		Width:     1400,
		Height:    900,
		MinWidth:  800,
		MinHeight: 560,
		HTML:      startingHTML,
		// Spike: leave devtools on so we can inspect WebGL / console
		// errors on test machines. Phase 2 gates this behind -debug.
		DevToolsEnabled:  true,
		BackgroundColour: application.NewRGB(0x1b, 0x1d, 0x21),
	})

	showWindow := func() {
		window.Show()
		window.Restore()
		window.Focus()
	}

	// Closing the window hides it; the station keeps running in the tray.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	var quitOnce sync.Once
	quit := func() {
		quitOnce.Do(func() {
			logger.Info("desktop: quit requested; stopping station")
			cancel()
			select {
			case err := <-runDone:
				if err != nil {
					logger.Error("graywolf exited with error", "err", err)
				}
			case <-time.After(cfg.ShutdownTimeout + 5*time.Second):
				logger.Error("desktop: station did not stop in time")
			}
			wapp.Quit()
		})
	}

	menu := application.NewMenu()
	menu.Add("Open Graywolf").OnClick(func(*application.Context) { showWindow() })
	menu.AddSeparator()
	menu.Add("Quit Graywolf").OnClick(func(*application.Context) { go quit() })

	tray := wapp.SystemTray.New()
	tray.SetIcon(iconPNG)
	tray.SetLabel("Graywolf")
	tray.SetMenu(menu)
	tray.OnClick(showWindow)

	// Start the station off the UI thread; wapp.Run owns the main thread.
	go func() {
		runDone <- app.New(cfg, logger).Run(ctx)
	}()

	go func() {
		select {
		case addr := <-bound:
			u := url.URL{
				Scheme:   "http",
				Host:     addr.String(),
				Path:     "/",
				RawQuery: url.Values{"gwdesktop": {token}}.Encode(),
			}
			logger.Info("desktop: station listening", "addr", addr.String())
			window.SetURL(u.String())
		case err := <-runDone:
			// Station failed before the HTTP listener came up.
			logger.Error("graywolf failed to start", "err", err)
			msg := fmt.Sprintf("Graywolf could not start:\n\n%v\n\nLog: %s", err, filepath.Join(dataDir, "graywolf-desktop.log"))
			wapp.Dialog.Error().SetTitle("Graywolf").SetMessage(msg).Show()
			runDone <- err // let quit() observe it
			go quit()
		}
	}()

	if err := wapp.Run(); err != nil {
		logger.Error("desktop shell", "err", err)
	}
	// Window manager / OS ended the app without going through the tray.
	quit()
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// setupLogger logs to stderr and to a file in the data dir (a GUI app
// has no visible console on Windows).
func setupLogger(cfg app.Config, dataDir string) (*slog.Logger, func()) {
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	var w io.Writer = os.Stderr
	closeFn := func() {}
	if f, err := os.OpenFile(filepath.Join(dataDir, "graywolf-desktop.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		w = io.MultiWriter(os.Stderr, f)
		closeFn = func() { _ = f.Close() }
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})), closeFn
}

// applyDesktopDefaults replaces the headless defaults for the DB, history
// DB and tile cache with the per-platform desktop data dir, unless the
// operator passed those flags explicitly. Returns the data dir (also used
// for the log file and webview profile).
func applyDesktopDefaults(cfg *app.Config) (string, error) {
	def := app.DefaultConfig()
	dir, err := desktopDataDir(def)
	if err != nil {
		return "", err
	}
	if cfg.DBPath == def.DBPath {
		cfg.DBPath = filepath.Join(dir, "graywolf.db")
	} else {
		// Explicit -config: keep the log file and webview profile beside
		// it rather than in the default data dir.
		abs, err := filepath.Abs(cfg.DBPath)
		if err != nil {
			return "", err
		}
		dir = filepath.Dir(abs)
	}
	if cfg.HistoryDBPath == def.HistoryDBPath {
		cfg.HistoryDBPath = filepath.Join(dir, "graywolf-history.db")
	}
	if cfg.TileCacheDir == def.TileCacheDir {
		cfg.TileCacheDir = filepath.Join(dir, "tiles")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create data dir %s: %w", dir, err)
	}
	return dir, nil
}
