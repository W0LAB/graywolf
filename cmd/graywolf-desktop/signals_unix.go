//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// handleSessionEnd: logout/shutdown sends SIGTERM. Wails also reacts to
// it, but its quit can stall behind an open dialog, so run our own
// ordered quit (station first, then GUI, with a watchdog).
func (s *shell) handleSessionEnd() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-ch
		s.logger.Info("desktop: signal received", "signal", sig.String())
		s.quit()
	}()
}

// sessionEndInterceptor is Windows-only.
func sessionEndInterceptor(*shell) func(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	return nil
}
