//go:build linux

package main

import (
	"log/slog"

	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// singleInstanceOptions returns Wails' single-instance options, or nil
// when no session D-Bus is reachable. On Linux Wails implements the lock
// as a session-bus name and treats a missing bus as fatal (os.Exit(1)
// inside application.New), which would make the app silently fail to
// launch on minimal setups (WSL, bare window managers, some kiosks).
func singleInstanceOptions(logger *slog.Logger) *application.SingleInstanceOptions {
	conn, err := dbus.SessionBusPrivate()
	if err == nil {
		err = conn.Auth(nil)
		if err == nil {
			err = conn.Hello()
		}
		_ = conn.Close()
	}
	if err != nil {
		logger.Warn("desktop: no session D-Bus; single-instance lock and tray icon unavailable", "err", err)
		return nil
	}
	return &application.SingleInstanceOptions{UniqueID: singleInstanceID}
}
