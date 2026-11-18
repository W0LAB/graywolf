package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// maxOpenURLLen bounds the URL accepted by /api/desktop/open-url.
const maxOpenURLLen = 4096

type openURLRequest struct {
	URL string `json:"url"`
}

// registerDesktopRoutes installs the desktop-shell-only endpoints on the
// auth-required api mux. Only called when Config.OpenExternalURL is set
// (cmd/graywolf-desktop). Deliberately kept out of pkg/webapi: these are
// host-shell plumbing, not part of the documented REST API.
func registerDesktopRoutes(apiMux *http.ServeMux, open func(string) error, logger *slog.Logger) {
	apiMux.HandleFunc("POST /api/desktop/open-url", func(w http.ResponseWriter, r *http.Request) {
		var req openURLRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxOpenURLLen+256)).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}
		u, ok := validateExternalURL(req.URL)
		if !ok {
			http.Error(w, `{"error":"only absolute http(s) URLs can be opened"}`, http.StatusBadRequest)
			return
		}
		if err := open(u); err != nil {
			logger.Warn("desktop: open external URL failed", "err", err)
			http.Error(w, `{"error":"could not open the system browser"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// validateExternalURL accepts only absolute http/https URLs with a host,
// so a page script can't use the endpoint to launch file:, custom
// protocol handlers or local executables through the OS opener.
func validateExternalURL(raw string) (string, bool) {
	if raw == "" || len(raw) > maxOpenURLLen || strings.ContainsAny(raw, "\r\n\x00") {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", false
	}
	return u.String(), true
}
