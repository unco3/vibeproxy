package token

import "testing"

func TestForService(t *testing.T) {
	got := ForService("openai")
	want := "vp-local-openai"
	if got != want {
		t.Errorf("ForService(\"openai\") = %q, want %q", got, want)
	}
}

func TestServiceFrom(t *testing.T) {
	tests := []struct {
		tok     string
		wantSvc string
		wantOK  bool
	}{
		{"vp-local-openai", "openai", true},
		{"vp-local-anthropic", "anthropic", true},
		{"vp-local-my-service-1", "my-service-1", true},
		{"vp-local-", "", false},
		{"sk-1234", "", false},
		{"", "", false},
		// Reject path traversal attempts
		{"vp-local-../../../etc/passwd", "", false},
		// Reject uppercase (strict lowercase-only)
		{"vp-local-OpenAI", "", false},
		// Reject special characters
		{"vp-local-svc name", "", false},
		{"vp-local-svc/path", "", false},
		{"vp-local-svc\ninjection", "", false},
	}
	for _, tt := range tests {
		svc, ok := ServiceFrom(tt.tok)
		if svc != tt.wantSvc || ok != tt.wantOK {
			t.Errorf("ServiceFrom(%q) = (%q, %v), want (%q, %v)", tt.tok, svc, ok, tt.wantSvc, tt.wantOK)
		}
	}
}

func TestIsVibeToken(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"vp-local-abc123", true},
		{"vp-local-", false},
		{"sk-1234", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsVibeToken(tt.input); got != tt.want {
			t.Errorf("IsVibeToken(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
