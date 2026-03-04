package policy

import "testing"

func TestWhitelist(t *testing.T) {
	w := NewWhitelist(map[string][]string{
		"openai": {"/v1/chat/completions", "/v1/embeddings"},
	})

	tests := []struct {
		svc, path string
		want      bool
	}{
		{"openai", "/v1/chat/completions", true},
		{"openai", "/v1/embeddings", true},
		{"openai", "/v1/files", false},
		{"openai", "/v2/chat/completions", false},
		{"unknown", "/v1/chat/completions", false},
		// Path traversal should be blocked by path.Clean
		{"openai", "/v1/chat/completions/../files", false},
		{"openai", "/v1/chat/completions/../../v1/files", false},
		// Normalized path still allowed
		{"openai", "/v1/chat/../chat/completions", true},
	}

	for _, tt := range tests {
		if got := w.Allowed(tt.svc, tt.path); got != tt.want {
			t.Errorf("Allowed(%q, %q) = %v, want %v", tt.svc, tt.path, got, tt.want)
		}
	}
}
