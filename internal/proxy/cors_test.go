package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unco3/vibeproxy/internal/config"
)

func TestCORSDisabled(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner, config.CORSConfig{Enabled: false})

	req := httptest.NewRequest("OPTIONS", "/v1/chat", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should not be set when disabled")
	}
}

func TestCORSPreflightAllowed(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called for preflight")
	})
	handler := corsMiddleware(inner, config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	req := httptest.NewRequest("OPTIONS", "/v1/chat", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("allow-origin = %q, want %q", got, "http://localhost:3000")
	}
}

func TestCORSPreflightRejected(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner, config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	req := httptest.NewRequest("OPTIONS", "/v1/chat", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("should not set CORS headers for disallowed origin")
	}
}

func TestCORSWildcardRejected(t *testing.T) {
	// Wildcard "*" should NOT match any origin — it was removed as a security measure.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner, config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
	})

	req := httptest.NewRequest("POST", "/v1/chat", nil)
	req.Header.Set("Origin", "http://anything.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("wildcard should not match, but got allow-origin = %q", got)
	}
}

func TestCORSNormalRequestPassesThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner, config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	req := httptest.NewRequest("POST", "/v1/chat", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler should be called for non-preflight requests")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("allow-origin = %q", got)
	}
}
