//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/chrissnell/graywolf/pkg/app"
)

// desktopDataDir returns $XDG_DATA_HOME/graywolf (default
// ~/.local/share/graywolf). The headless default is the process cwd,
// which is meaningless for an app launched from a desktop menu.
func desktopDataDir(_ app.Config) (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" && filepath.IsAbs(x) {
		return filepath.Join(x, "graywolf"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("cannot determine home directory for desktop data dir")
	}
	return filepath.Join(home, ".local", "share", "graywolf"), nil
}
