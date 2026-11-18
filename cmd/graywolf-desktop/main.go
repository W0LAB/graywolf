// Command graywolf-desktop is the native desktop app for Graywolf on
// Windows and Linux. Plan: .context/2026-11-17-native-desktop-shell.md
// (local); wiki: docs/wiki/code-map.md "Native desktop shell".
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
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/chrissnell/graywolf/pkg/app"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
)

//go:embed icon.png
var iconPNG []byte

const singleInstanceID = "com.nw5w.graywolf.desktop"

// hiddenFlag starts the app in the tray without showing the window
// (used by start-at-login). Desktop-only, so it is stripped before the
// shared app.ParseFlags sees the arguments.
const hiddenFlag = "-hidden"

const startingHTML = `<!doctype html><html><head><meta charset="utf-8">
<style>html,body{height:100%;margin:0;background:#1b1d21;color:#c9ccd1;
font:15px system-ui,sans-serif;display:flex;align-items:center;justify-content:center}</style>
</head><body>Starting Graywolf&hellip;</body></html>`

// splitDesktopFlags removes desktop-only flags from args.
func splitDesktopFlags(args []string) (hidden bool, rest []string) {
	for _, a := range args {
		if a == hiddenFlag || a == "-"+hiddenFlag {
			hidden = true
			continue
		}
		rest = append(rest, a)
	}
	return hidden, rest
}

func main() {
	hidden, args := splitDesktopFlags(os.Args[1:])
	cfg, err := app.ParseFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	cfg.Version = Version
	cfg.GitCommit = GitCommit

	// Where a headless graywolf on this machine would listen (-http,
	// default 127.0.0.1:8080): probed at startup to avoid running two
	// stations on one radio.
	probeAddr := cfg.HTTPAddr

	dataDir, err := applyDesktopDefaults(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Shell logger: stderr + rotating file. The station's logbuffer
	// (System Logs page) is layered on after the single-instance check
	// below, so a second launch never opens the ring DB.
	inner, closeLog := newFileHandler(cfg, dataDir)
	defer closeLog()
	logger := slog.New(inner)
	slog.SetDefault(logger)

	busErr := sessionBusError()
	trayOK := busErr == nil
	if !trayOK {
		logger.Warn("desktop: no session D-Bus; tray icon and single-instance lock unavailable, closing the window will quit", "err", busErr)
	}

	// No login page in the app: create the station user on a fresh
	// install so user-scoped endpoints work under the launch token.
	cfg.EnsureStationUser = true
	// Loopback only, OS-chosen port. Phase 4 adds an opt-in LAN listener.
	cfg.HTTPAddr = "127.0.0.1:0"
	token, err := newToken()
	if err != nil {
		logger.Error("generate launch token", "err", err)
		os.Exit(1)
	}
	cfg.BearerToken = token

	ctx, cancel := context.WithCancel(context.Background())
	sh := &shell{
		logger:  logger,
		token:   token,
		dataDir: dataDir,
		state:   loadState(dataDir, logger),
		trayOK:  trayOK,
		ctx:     ctx,
		cancel:  cancel,
		bound:   make(chan net.Addr, 1),
		runDone: make(chan error, 1),
	}
	cfg.OnHTTPListenerBound = func(a net.Addr) { sh.bound <- a }

	var single *application.SingleInstanceOptions
	if trayOK { // on Linux the lock is a session-bus name
		single = &application.SingleInstanceOptions{
			UniqueID: singleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if sh.window != nil {
					sh.showWindow()
				}
			},
		}
	}

	sh.wapp = application.New(application.Options{
		Name:        "Graywolf",
		Description: "APRS / AX.25 packet radio station",
		Icon:        iconPNG,
		// Without these, Wails' own fatal errors exit the process with
		// no output at all.
		Logger: logger.With("component", "wails"),
		ErrorHandler: func(err error) {
			logger.Error("desktop: wails error", "err", err)
		},
		SingleInstance: single,
		// signals_*.go owns SIGTERM/SIGINT: stop the station first, then
		// the GUI, with a watchdog. Two handlers racing into Quit is what
		// we avoid here.
		DisableDefaultSignalHandler: true,
		Linux: application.LinuxOptions{
			ProgramName:                   "graywolf",
			DisableQuitOnLastWindowClosed: true,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			WebviewUserDataPath:           filepath.Join(dataDir, "webview"),
			WndProcInterceptor:            sessionEndInterceptor(sh),
		},
	})

	// Past the single-instance gate: attach the station's log ring.
	stationLogger, logDB := app.SetupLogger(inner, cfg)
	cfg.LogBuffer = logDB
	if logDB != nil {
		defer func() { _ = logDB.Close() }()
	}
	slog.SetDefault(stationLogger)
	sh.logger = stationLogger

	cfg.OpenExternalURL = func(u string) error { return sh.wapp.Browser.OpenURL(u) }
	sh.cfg = cfg

	sh.window = sh.wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "Graywolf",
		Width:     1400,
		Height:    900,
		MinWidth:  800,
		MinHeight: 560,
		HTML:      startingHTML,
		// Start-at-login opens straight to the tray -- but only if there
		// is a tray to come back from.
		Hidden: hidden && trayOK,
		// Web inspector only with -debug (right-click > Inspect).
		DevToolsEnabled:  cfg.Debug,
		BackgroundColour: application.NewRGB(0x1b, 0x1d, 0x21),
	})
	sh.buildTray()
	sh.registerHooks(probeAddr)
	sh.handleSessionEnd()

	if err := sh.wapp.Run(); err != nil {
		sh.logger.Error("desktop shell", "err", err)
	}
	// Window manager / OS / SIGTERM ended the app without the tray.
	sh.quit()
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// maxLogBytes caps graywolf-desktop.log; checked once at startup.
const maxLogBytes = 10 << 20

// newFileHandler returns a text handler writing to stderr and to
// graywolf-desktop.log in the data dir (a GUI app has no console on
// Windows). A log over maxLogBytes is rotated to .1 at startup.
func newFileHandler(cfg app.Config, dataDir string) (slog.Handler, func()) {
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	path := filepath.Join(dataDir, "graywolf-desktop.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogBytes {
		_ = os.Rename(path, path+".1")
	}
	var w io.Writer = os.Stderr
	closeFn := func() {}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		w = io.MultiWriter(os.Stderr, f)
		closeFn = func() { _ = f.Close() }
	}
	return slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}), closeFn
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
