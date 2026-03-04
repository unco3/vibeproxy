package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	yaml := `services:
  openai:
    target: https://api.openai.com
    auth_header: Authorization
    auth_scheme: Bearer
    allowed_paths:
      - /v1/chat/completions
    rate_limit:
      requests_per_minute: 60
listen: 127.0.0.1:9090
`
	os.WriteFile(filepath.Join(dir, "vibeproxy.yaml"), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Listen != "127.0.0.1:9090" {
		t.Errorf("listen = %q, want 127.0.0.1:9090", cfg.Listen)
	}

	svc, ok := cfg.Services["openai"]
	if !ok {
		t.Fatal("missing openai service")
	}
	if svc.Target != "https://api.openai.com" {
		t.Errorf("target = %q", svc.Target)
	}
	if svc.RateLimit.RequestsPerMinute != 60 {
		t.Errorf("rate limit = %d", svc.RateLimit.RequestsPerMinute)
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestGatewayPathsDefault(t *testing.T) {
	dir := t.TempDir()
	yaml := `services: {}
gateway:
  enabled: true
  models:
    gpt-: openai
`
	os.WriteFile(filepath.Join(dir, "vibeproxy.yaml"), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Gateway.Enabled {
		t.Error("gateway should be enabled")
	}
	if len(cfg.Gateway.Paths) != 1 || cfg.Gateway.Paths[0] != "/v1/chat/completions" {
		t.Errorf("gateway.paths = %v, want [/v1/chat/completions]", cfg.Gateway.Paths)
	}
}

func TestGatewayPathsCustom(t *testing.T) {
	dir := t.TempDir()
	yaml := `services: {}
gateway:
  enabled: true
  paths:
    - /v1/chat/completions
    - /v1/embeddings
  models:
    gpt-: openai
`
	os.WriteFile(filepath.Join(dir, "vibeproxy.yaml"), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Gateway.Paths) != 2 {
		t.Errorf("expected 2 gateway paths, got %d: %v", len(cfg.Gateway.Paths), cfg.Gateway.Paths)
	}
}

func TestLoadSecretBackend(t *testing.T) {
	dir := t.TempDir()
	yaml := `services: {}
secret_backend: env
`
	os.WriteFile(filepath.Join(dir, "vibeproxy.yaml"), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecretBackend != "env" {
		t.Errorf("secret_backend = %q, want %q", cfg.SecretBackend, "env")
	}
}
