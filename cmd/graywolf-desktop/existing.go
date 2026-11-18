package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"
)

// probeTarget turns the operator's -http address (what a headless
// graywolf on this machine would listen on) into a dialable loopback
// address: wildcard and empty hosts become 127.0.0.1.
func probeTarget(httpAddr string) (string, bool) {
	host, port, err := net.SplitHostPort(httpAddr)
	if err != nil || port == "" || port == "0" {
		return "", false
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), true
}

// probeExistingStation checks whether a Graywolf station (typically the
// systemd / NSSM service) already answers at addr. It looks for the
// public /api/version fingerprint -- {"version","platform"} -- so an
// unrelated web server on the same port is not mistaken for one.
// Returns the station's base URL when found.
func probeExistingStation(ctx context.Context, addr string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	base := "http://" + addr
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/version", nil)
	if err != nil {
		return "", false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var v struct {
		Version  string `json:"version"`
		Platform string `json:"platform"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&v); err != nil {
		return "", false
	}
	if v.Version == "" || v.Platform == "" {
		return "", false
	}
	return base + "/", true
}
