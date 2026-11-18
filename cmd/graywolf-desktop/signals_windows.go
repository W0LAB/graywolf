//go:build windows

package main

// wmEndSession is WM_ENDSESSION: Windows is logging off or shutting
// down (wParam != 0). GUI apps get no SIGTERM.
const wmEndSession = 0x0016

// handleSessionEnd is a no-op on Windows; see sessionEndInterceptor.
func (s *shell) handleSessionEnd() {}

// sessionEndInterceptor stops the station synchronously when the session
// ends. Windows may terminate the process as soon as WM_ENDSESSION
// returns, so the ordered stop must finish inside the handler; the
// message is then passed on to Wails.
func sessionEndInterceptor(s *shell) func(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	return func(_ uintptr, msg uint32, wParam, _ uintptr) (uintptr, bool) {
		if msg == wmEndSession && wParam != 0 {
			s.logger.Info("desktop: Windows session ending; stopping station")
			s.stopStation()
		}
		return 0, false
	}
}
