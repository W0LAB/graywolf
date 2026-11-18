package main

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chrissnell/graywolf/pkg/app"
)

func TestNewToken(t *testing.T) {
	a, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newToken()
	if len(a) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars", len(a))
	}
	if _, err := hex.DecodeString(a); err != nil {
		t.Fatalf("token not hex: %v", err)
	}
	if a == b {
		t.Fatal("two launches produced the same token")
	}
}

func TestApplyDesktopDefaults_UsesDataDirForUnsetPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps the headless %ProgramData% layout")
	}
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	cfg := app.DefaultConfig()
	dir, err := applyDesktopDefaults(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "graywolf")
	if dir != want {
		t.Fatalf("data dir = %q, want %q", dir, want)
	}
	if cfg.DBPath != filepath.Join(want, "graywolf.db") ||
		cfg.HistoryDBPath != filepath.Join(want, "graywolf-history.db") ||
		cfg.TileCacheDir != filepath.Join(want, "tiles") {
		t.Fatalf("paths not moved to data dir: %+v", cfg)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Fatalf("data dir not created: %v", err)
	}
}

func TestApplyDesktopDefaults_ExplicitConfigKeepsLogBesideIt(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	custom := t.TempDir()

	cfg := app.DefaultConfig()
	cfg.DBPath = filepath.Join(custom, "my.db")
	cfg.HistoryDBPath = filepath.Join(custom, "hist.db")
	dir, err := applyDesktopDefaults(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if dir != custom {
		t.Fatalf("data dir = %q, want the -config dir %q", dir, custom)
	}
	if cfg.DBPath != filepath.Join(custom, "my.db") || cfg.HistoryDBPath != filepath.Join(custom, "hist.db") {
		t.Fatalf("explicit paths were overwritten: %+v", cfg)
	}
}

func TestDesktopDataDir_IgnoresRelativeXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG is Unix-only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "relative/path")
	dir, err := desktopDataDir(app.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "share", "graywolf"); dir != want {
		t.Fatalf("dir = %q, want %q (relative XDG_DATA_HOME must be ignored per spec)", dir, want)
	}
}

func TestNewFileHandler_RotatesOversizedLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graywolf-desktop.log")
	if err := os.WriteFile(path, make([]byte, maxLogBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	_, closeFn := newFileHandler(app.DefaultConfig(), dir)
	defer closeFn()

	if fi, err := os.Stat(path + ".1"); err != nil || fi.Size() != maxLogBytes+1 {
		t.Fatalf("old log not rotated to .1: %v", err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 {
		t.Fatalf("fresh log not started: %v", err)
	}
}

// The placeholder must be a URL Wails' GetStartURL passes through intact
// (not the HTML option -- see the window options in main()).
func TestStartingURL(t *testing.T) {
	u := startingURL()
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme != "data" || parsed.String() != u {
		t.Fatalf("startingURL does not round-trip: %v %q", err, parsed)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(u, "data:text/html;base64,"))
	if err != nil || string(b) != startingHTML {
		t.Fatalf("payload mismatch: %v", err)
	}
}
