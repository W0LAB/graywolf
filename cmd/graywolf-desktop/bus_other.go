//go:build !linux

package main

// sessionBusError: only Linux needs a session bus for the tray and the
// single-instance lock.
func sessionBusError() error { return nil }
