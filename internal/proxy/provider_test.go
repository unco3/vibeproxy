package proxy

import (
	"net/http"
	"testing"
)

func TestHeaderProviderBearer(t *testing.T) {
	p := &HeaderProvider{Header: "Authorization", Scheme: "Bearer"}

	r, _ := http.NewRequest("POST", "/v1/chat", nil)
	r.Header.Set("Authorization", "Bearer vp-local-abc123")

	got := p.ExtractToken(r)
	if got != "vp-local-abc123" {
		t.Errorf("got %q, want %q", got, "vp-local-abc123")
	}
}

func TestHeaderProviderRaw(t *testing.T) {
	p := &HeaderProvider{Header: "x-api-key", Scheme: ""}

	r, _ := http.NewRequest("POST", "/v1/messages", nil)
	r.Header.Set("x-api-key", "vp-local-def456")

	got := p.ExtractToken(r)
	if got != "vp-local-def456" {
		t.Errorf("got %q, want %q", got, "vp-local-def456")
	}
}

func TestHeaderProviderEmpty(t *testing.T) {
	p := &HeaderProvider{Header: "Authorization", Scheme: "Bearer"}

	r, _ := http.NewRequest("POST", "/v1/chat", nil)
	if got := p.ExtractToken(r); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHeaderProviderWrongScheme(t *testing.T) {
	p := &HeaderProvider{Header: "Authorization", Scheme: "Bearer"}

	r, _ := http.NewRequest("POST", "/v1/chat", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	if got := p.ExtractToken(r); got != "" {
		t.Errorf("expected empty for wrong scheme, got %q", got)
	}
}

func TestHeaderProviderCustomHeader(t *testing.T) {
	p := &HeaderProvider{Header: "X-Custom-Key", Scheme: "Token"}

	r, _ := http.NewRequest("POST", "/api/v1/complete", nil)
	r.Header.Set("X-Custom-Key", "Token vp-local-custom999")

	got := p.ExtractToken(r)
	if got != "vp-local-custom999" {
		t.Errorf("got %q, want %q", got, "vp-local-custom999")
	}
}

func TestInjectKeyBearer(t *testing.T) {
	p := &HeaderProvider{Header: "Authorization", Scheme: "Bearer"}

	r, _ := http.NewRequest("POST", "/v1/chat", nil)
	p.InjectKey(r, "sk-real-key")

	if got := r.Header.Get("Authorization"); got != "Bearer sk-real-key" {
		t.Errorf("got %q, want %q", got, "Bearer sk-real-key")
	}
}

func TestInjectKeyRaw(t *testing.T) {
	p := &HeaderProvider{Header: "x-api-key", Scheme: ""}

	r, _ := http.NewRequest("POST", "/v1/messages", nil)
	p.InjectKey(r, "sk-ant-real")

	if got := r.Header.Get("x-api-key"); got != "sk-ant-real" {
		t.Errorf("got %q, want %q", got, "sk-ant-real")
	}
}
