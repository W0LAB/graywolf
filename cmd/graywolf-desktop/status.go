package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// stationStatus is the subset of GET /api/status the tray shows.
type stationStatus struct {
	Channels []struct {
		RxFrames uint64 `json:"rx_frames"`
		TxFrames uint64 `json:"tx_frames"`
	} `json:"channels"`
	Igate *struct {
		Connected bool `json:"connected"`
	} `json:"igate"`
}

// statusLine renders the one-line tray summary.
func statusLine(s stationStatus) string {
	if len(s.Channels) == 0 {
		return "Running - no channels configured"
	}
	var rx, tx uint64
	for _, c := range s.Channels {
		rx += c.RxFrames
		tx += c.TxFrames
	}
	parts := []string{
		fmt.Sprintf("Running - %d channel%s", len(s.Channels), plural(len(s.Channels))),
		fmt.Sprintf("RX %d / TX %d", rx, tx),
	}
	if s.Igate != nil {
		if s.Igate.Connected {
			parts = append(parts, "iGate up")
		} else {
			parts = append(parts, "iGate down")
		}
	}
	return strings.Join(parts, " - ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// fetchStatus reads the station's own /api/status with the launch token.
func fetchStatus(ctx context.Context, base, token string) (stationStatus, error) {
	var s stationStatus
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/status", nil)
	if err != nil {
		return s, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return s, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s, fmt.Errorf("status %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&s)
	return s, err
}

// pollStatus calls update with a fresh summary every interval until ctx
// ends.
func pollStatus(ctx context.Context, base, token string, interval time.Duration, update func(string)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s, err := fetchStatus(ctx, base, token)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			update("Station not responding")
		} else {
			update(statusLine(s))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
