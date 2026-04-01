package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, skipAuth bool) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Config{
		Domain:     "test.local",
		ListenAddr: "127.0.0.1:0",
		SkipAuth:   skipAuth,
	}, logger)
}

func TestHealthCheck(t *testing.T) {
	srv := newTestServer(t, true)

	req := httptest.NewRequest("GET", "/_health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", w.Body.String())
	}
}

func TestLandingPage(t *testing.T) {
	srv := newTestServer(t, true)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "KP-Gruuk") {
		t.Fatal("expected landing page to contain 'KP-Gruuk'")
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected text/html content-type, got %q", ct)
	}
}

func TestTunnelConnectAuthRequired(t *testing.T) {
	srv := newTestServer(t, false) // auth enabled

	req := httptest.NewRequest("GET", "/_ws/connect", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}
}

func TestTunnelConnectSkipAuthMissingEmail(t *testing.T) {
	srv := newTestServer(t, true)

	req := httptest.NewRequest("GET", "/_ws/connect", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without email in skip-auth mode, got %d", w.Code)
	}
}

func TestTunnelConnectAuthNotConfigured(t *testing.T) {
	srv := newTestServer(t, false)
	// ValidateToken is intentionally nil

	req := httptest.NewRequest("GET", "/_ws/connect", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when ValidateToken not set, got %d", w.Code)
	}
}

func TestTunnelConnectInvalidToken(t *testing.T) {
	srv := newTestServer(t, false)
	srv.ValidateToken = func(token string) (string, error) {
		return "", &tokenError{"invalid"}
	}

	req := httptest.NewRequest("GET", "/_ws/connect", nil)
	req.Header.Set("Authorization", "Bearer badtoken")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", w.Code)
	}
}

// tokenError is a simple error for testing.
type tokenError struct{ msg string }

func (e *tokenError) Error() string { return e.msg }

func TestSubdomainProxyNoTunnel(t *testing.T) {
	srv := newTestServer(t, true)

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Host = "alice.test.local"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for unregistered subdomain, got %d", w.Code)
	}
}

func TestExtractSubdomain(t *testing.T) {
	srv := newTestServer(t, true)

	cases := []struct {
		host string
		want string
	}{
		{"alice.test.local", "alice"},
		{"alice.test.local:8080", "alice"},
		{"test.local", ""},
		{"other.domain.com", ""},
		{"", ""},
		{"two.levels.test.local", ""},
	}

	for _, tc := range cases {
		got := srv.extractSubdomain(tc.host)
		if got != tc.want {
			t.Errorf("extractSubdomain(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}
