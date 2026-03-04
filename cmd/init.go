package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unco3/vibeproxy/internal/config"
	"github.com/unco3/vibeproxy/internal/token"

	"github.com/spf13/cobra"
)

const defaultYAML = `services:
  openai:
    target: https://api.openai.com
    auth_header: Authorization
    auth_scheme: Bearer
    allowed_paths:
      - /v1/chat/completions
      - /v1/embeddings
      - /v1/responses
    rate_limit:
      requests_per_minute: 60

  anthropic:
    target: https://api.anthropic.com
    auth_header: x-api-key
    auth_scheme: ""
    allowed_paths:
      - /v1/messages
    rate_limit:
      requests_per_minute: 60

listen: 127.0.0.1:8080

timeouts:
  read_seconds: 30
  write_seconds: 120
  upstream_seconds: 90

cors:
  enabled: false
  allowed_origins:
    - http://localhost:3000

# gateway:
#   enabled: true
#   paths:
#     - /v1/chat/completions
#     - /v1/embeddings
#   models:
#     gpt-: openai
#     o1-: openai
#     o3-: openai
#     claude-: anthropic
`

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize vibeproxy.yaml and .env in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		// Write vibeproxy.yaml if not exists
		yamlPath := filepath.Join(dir, "vibeproxy.yaml")
		if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
			if err := os.WriteFile(yamlPath, []byte(defaultYAML), 0644); err != nil {
				return fmt.Errorf("writing vibeproxy.yaml: %w", err)
			}
			fmt.Println("Created vibeproxy.yaml")
		} else {
			fmt.Println("vibeproxy.yaml already exists, skipping")
		}

		// Update .gitignore
		gitignorePath := filepath.Join(dir, ".gitignore")
		if err := ensureGitignoreEntries(gitignorePath, []string{".env"}); err != nil {
			return fmt.Errorf("updating .gitignore: %w", err)
		}

		// Load config to generate .env from services
		cfg, err := config.Load(dir)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Write .env
		envPath := filepath.Join(dir, ".env")
		if _, err := os.Stat(envPath); os.IsNotExist(err) {
			if err := writeEnvFromConfig(envPath, cfg); err != nil {
				return fmt.Errorf("writing .env: %w", err)
			}
			fmt.Println("Created .env (agent reference)")
		}

		fmt.Println("\nDone! Next steps:")
		fmt.Println("  1. vibe auth login openai     # store your real API key")
		fmt.Println("  2. vibe start                  # start the proxy")
		return nil
	},
}

func ensureGitignoreEntries(path string, entries []string) error {
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(existing, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(existing) > 0 && !strings.HasSuffix(existing, "\n") {
		fmt.Fprintln(f)
	}
	for _, entry := range toAdd {
		fmt.Fprintln(f, entry)
	}
	fmt.Println("Updated .gitignore")
	return nil
}

// writeEnvFromConfig generates .env directly from config services using deterministic tokens.
func writeEnvFromConfig(envPath string, cfg *config.Config) error {
	var lines []string
	lines = append(lines, "# VibeProxy — point agents at localhost")
	lines = append(lines, fmt.Sprintf("VIBEPROXY_URL=http://%s", cfg.Listen))
	lines = append(lines, "")

	// Sort service names for deterministic output
	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		envVar := strings.ToUpper(name) + "_API_KEY"
		lines = append(lines, fmt.Sprintf("%s=%s", envVar, token.ForService(name)))
	}
	lines = append(lines, "")

	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0600)
}
