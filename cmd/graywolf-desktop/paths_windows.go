//go:build windows

package main

import (
	"path/filepath"

	"github.com/chrissnell/graywolf/pkg/app"
)

// desktopDataDir keeps the headless Windows layout (%ProgramData%\Graywolf)
// so an operator moving from graywolf.exe to the desktop app keeps their
// config, history and tiles.
func desktopDataDir(def app.Config) (string, error) {
	return filepath.Dir(def.DBPath), nil
}
