package secret

import (
	"fmt"
	"os"
	"strings"
)

// EnvProvider reads secrets from environment variables (read-only).
// Variable names follow the pattern VIBEPROXY_SECRET_{SERVICE} (upper-cased).
type EnvProvider struct{}

func (e *EnvProvider) Get(service string) (string, error) {
	key := envKey(service)
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("environment variable %s not set", key)
	}
	return val, nil
}

func (e *EnvProvider) Set(string, string) error {
	return fmt.Errorf("env provider is read-only")
}

func (e *EnvProvider) Delete(string) error {
	return fmt.Errorf("env provider is read-only")
}

func (e *EnvProvider) Name() string { return "env" }

func envKey(service string) string {
	return "VIBEPROXY_SECRET_" + strings.ToUpper(service)
}
