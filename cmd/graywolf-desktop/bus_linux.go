//go:build linux

package main

import "github.com/godbus/dbus/v5"

// sessionBusError reports whether a session D-Bus is reachable. On Linux
// both Wails' single-instance lock (a bus name) and the tray icon
// (StatusNotifierItem) need it, and Wails treats a missing bus during
// application.New as fatal (os.Exit(1) with no output). Minimal setups
// (WSL, bare window managers, some kiosks) have no session bus.
func sessionBusError() error {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Auth(nil); err != nil {
		return err
	}
	return conn.Hello()
}
