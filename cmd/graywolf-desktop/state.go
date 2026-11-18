package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// desktopState is small, shell-only UI state that survives restarts
// (desktop-state.json in the data dir). Station configuration lives in
// graywolf.db, never here.
type desktopState struct {
	// CloseNoticeShown records that the operator has seen the one-time
	// "still running in the tray" notice.
	CloseNoticeShown bool `json:"close_notice_shown"`

	mu     sync.Mutex
	path   string
	logger *slog.Logger
}

func loadState(dataDir string, logger *slog.Logger) *desktopState {
	s := &desktopState{path: filepath.Join(dataDir, "desktop-state.json"), logger: logger}
	b, err := os.ReadFile(s.path)
	if err == nil {
		if err := json.Unmarshal(b, s); err != nil {
			logger.Warn("desktop: ignoring unreadable state file", "path", s.path, "err", err)
		}
	}
	return s
}

// markCloseNoticeShown sets the flag and persists it. Returns true if
// this call flipped it (i.e. the notice should be shown now).
func (s *desktopState) markCloseNoticeShown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.CloseNoticeShown {
		return false
	}
	s.CloseNoticeShown = true
	s.saveLocked()
	return true
}

func (s *desktopState) saveLocked() {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err == nil {
		err = os.Rename(tmp, s.path)
		if err == nil {
			return
		}
	}
	s.logger.Warn("desktop: could not save state", "path", s.path)
}
