package secret

import (
	"testing"
)

func TestEnvProvider_Get(t *testing.T) {
	t.Setenv("VIBEPROXY_SECRET_OPENAI", "sk-test-key")

	p := &EnvProvider{}
	got, err := p.Get("openai")
	if err != nil {
		t.Fatal(err)
	}
	if got != "sk-test-key" {
		t.Errorf("got %q, want %q", got, "sk-test-key")
	}
}

func TestEnvProvider_GetMissing(t *testing.T) {
	p := &EnvProvider{}
	_, err := p.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestEnvProvider_ReadOnly(t *testing.T) {
	p := &EnvProvider{}
	if err := p.Set("openai", "key"); err == nil {
		t.Error("expected error on Set")
	}
	if err := p.Delete("openai"); err == nil {
		t.Error("expected error on Delete")
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		backend  string
		wantName string
		wantErr  bool
	}{
		{"", "keychain", false},
		{"keychain", "keychain", false},
		{"env", "env", false},
		{"unknown", "", true},
	}
	for _, tt := range tests {
		p, err := New(tt.backend)
		if tt.wantErr {
			if err == nil {
				t.Errorf("New(%q) should error", tt.backend)
			}
			continue
		}
		if err != nil {
			t.Errorf("New(%q) error: %v", tt.backend, err)
			continue
		}
		if p.Name() != tt.wantName {
			t.Errorf("New(%q).Name() = %q, want %q", tt.backend, p.Name(), tt.wantName)
		}
	}
}
