package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSplitDesktopFlags(t *testing.T) {
	hidden, rest := splitDesktopFlags([]string{"-debug", "-hidden", "-modem", "x"})
	if !hidden {
		t.Fatal("-hidden not detected")
	}
	if strings.Join(rest, " ") != "-debug -modem x" {
		t.Fatalf("rest = %v", rest)
	}
	hidden, rest = splitDesktopFlags([]string{"--hidden"})
	if !hidden || len(rest) != 0 {
		t.Fatalf("--hidden: hidden=%v rest=%v", hidden, rest)
	}
	if hidden, _ := splitDesktopFlags(nil); hidden {
		t.Fatal("hidden with no args")
	}
}

func TestProbeTarget(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:8080": "127.0.0.1:8080",
		"0.0.0.0:8080":   "127.0.0.1:8080",
		":9090":          "127.0.0.1:9090",
		"[::]:8080":      "127.0.0.1:8080",
		"192.168.1.5:80": "192.168.1.5:80",
	}
	for in, want := range cases {
		got, ok := probeTarget(in)
		if !ok || got != want {
			t.Errorf("probeTarget(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "127.0.0.1:0", "nonsense"} {
		if _, ok := probeTarget(in); ok {
			t.Errorf("probeTarget(%q) accepted", in)
		}
	}
}

func TestProbeExistingStation(t *testing.T) {
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			_, _ = io.WriteString(w, `{"version":"0.14.14","commit":"abc","platform":"windows"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer gw.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"hello":"world"}`)
	}))
	defer other.Close()
	locked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer locked.Close()

	ctx := context.Background()
	addr := strings.TrimPrefix(gw.URL, "http://")
	if u, ok := probeExistingStation(ctx, addr); !ok || u != gw.URL+"/" {
		t.Fatalf("graywolf not detected: %q %v", u, ok)
	}
	if _, ok := probeExistingStation(ctx, strings.TrimPrefix(other.URL, "http://")); ok {
		t.Fatal("unrelated server detected as graywolf")
	}
	// Another desktop app's bearer-gated listener is not "a station to attach to".
	if _, ok := probeExistingStation(ctx, strings.TrimPrefix(locked.URL, "http://")); ok {
		t.Fatal("401 detected as graywolf")
	}
	if _, ok := probeExistingStation(ctx, "127.0.0.1:1"); ok {
		t.Fatal("closed port detected as graywolf")
	}
}

func TestStatusLine(t *testing.T) {
	var s stationStatus
	if got := statusLine(s); got != "Running - no channels configured" {
		t.Fatalf("empty: %q", got)
	}
	s.Channels = append(s.Channels,
		struct {
			RxFrames uint64 `json:"rx_frames"`
			TxFrames uint64 `json:"tx_frames"`
		}{10, 2},
		struct {
			RxFrames uint64 `json:"rx_frames"`
			TxFrames uint64 `json:"tx_frames"`
		}{5, 1})
	if got := statusLine(s); got != "Running - 2 channels - RX 15 / TX 3" {
		t.Fatalf("channels: %q", got)
	}
	s.Igate = &struct {
		Connected bool `json:"connected"`
	}{true}
	if got := statusLine(s); !strings.HasSuffix(got, " - iGate up") {
		t.Fatalf("igate: %q", got)
	}
}

func TestPollStatusUsesTokenAndReportsFailure(t *testing.T) {
	var sawAuth string
	ok := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, `{"channels":[{"rx_frames":7,"tx_frames":1}]}`)
	}))
	defer srv.Close()

	lines := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	go pollStatus(ctx, srv.URL, "tok", time.Hour, func(l string) { lines <- l })
	if got := <-lines; got != "Running - 1 channel - RX 7 / TX 1" {
		t.Fatalf("line = %q", got)
	}
	cancel()
	if sawAuth != "Bearer tok" {
		t.Fatalf("Authorization = %q", sawAuth)
	}

	ok = false
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go pollStatus(ctx2, srv.URL, "tok", time.Hour, func(l string) { lines <- l })
	if got := <-lines; got != "Station not responding" {
		t.Fatalf("failure line = %q", got)
	}
}

func TestDesktopStateCloseNotice(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := loadState(dir, logger)
	if !s.markCloseNoticeShown() {
		t.Fatal("first call should report the notice is due")
	}
	if s.markCloseNoticeShown() {
		t.Fatal("second call should not")
	}
	if !loadState(dir, logger).CloseNoticeShown {
		t.Fatal("flag not persisted")
	}
	// A corrupt file is ignored, not fatal.
	if err := os.WriteFile(filepath.Join(dir, "desktop-state.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if loadState(dir, logger).CloseNoticeShown {
		t.Fatal("corrupt state should load as defaults")
	}
}
