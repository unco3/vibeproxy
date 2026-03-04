package proxy

import (
	"net/http"
	"testing"
)

// ExtractToken is a convenience fallback. Primary extraction is via Provider.
// These tests ensure the legacy helper still works for common patterns.

func TestExtractTokenBearer(t *testing.T) {
	r, _ := http.NewRequest("POST", "/v1/chat", nil)
	r.Header.Set("Authorization", "Bearer vp-local-abc123")

	got := ExtractToken(r)
	if got != "vp-local-abc123" {
		t.Errorf("got %q, want %q", got, "vp-local-abc123")
	}
}

func TestExtractTokenXAPIKey(t *testing.T) {
	r, _ := http.NewRequest("POST", "/v1/messages", nil)
	r.Header.Set("x-api-key", "vp-local-def456")

	got := ExtractToken(r)
	if got != "vp-local-def456" {
		t.Errorf("got %q, want %q", got, "vp-local-def456")
	}
}

func TestExtractTokenEmpty(t *testing.T) {
	r, _ := http.NewRequest("POST", "/v1/chat", nil)
	if got := ExtractToken(r); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
