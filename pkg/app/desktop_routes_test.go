package app

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateExternalURL(t *testing.T) {
	ok := []string{
		"https://aprs.fi/#!call=a%2FW0LAB",
		"http://www.qrz.com/db/W0LAB",
		"HTTPS://github.com/chrissnell/graywolf",
	}
	bad := []string{
		"",
		"/relative",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ms-settings:privacy",
		"mailto:a@b.c",
		"https://user:pass@example.com/",
		"https://example.com/\r\nx",
		"https:///nohost",
		"https://example.com/" + strings.Repeat("a", maxOpenURLLen),
	}
	for _, u := range ok {
		if _, valid := validateExternalURL(u); !valid {
			t.Errorf("rejected %q", u)
		}
	}
	for _, u := range bad {
		if _, valid := validateExternalURL(u); valid {
			t.Errorf("accepted %q", u)
		}
	}
}

func TestDesktopOpenURLRoute(t *testing.T) {
	var opened []string
	fail := false
	mux := http.NewServeMux()
	registerDesktopRoutes(mux, func(u string) error {
		if fail {
			return errors.New("no browser")
		}
		opened = append(opened, u)
		return nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	do := func(method, body string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, "/api/desktop/open-url", strings.NewReader(body)))
		return rec.Code
	}

	if c := do(http.MethodPost, `{"url":"https://aprs.fi/"}`); c != http.StatusNoContent {
		t.Fatalf("valid URL: got %d", c)
	}
	if len(opened) != 1 || opened[0] != "https://aprs.fi/" {
		t.Fatalf("opener got %v", opened)
	}
	if c := do(http.MethodPost, `{"url":"file:///etc/passwd"}`); c != http.StatusBadRequest {
		t.Fatalf("file URL: got %d", c)
	}
	if c := do(http.MethodPost, `not json`); c != http.StatusBadRequest {
		t.Fatalf("bad JSON: got %d", c)
	}
	if c := do(http.MethodGet, ``); c != http.StatusMethodNotAllowed {
		t.Fatalf("GET: got %d", c)
	}
	fail = true
	if c := do(http.MethodPost, `{"url":"https://aprs.fi/"}`); c != http.StatusInternalServerError {
		t.Fatalf("opener failure: got %d", c)
	}
	if len(opened) != 1 {
		t.Fatalf("rejected requests reached the opener: %v", opened)
	}
}
