package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"vibeproxy/internal/secret"

	"github.com/spf13/cobra"
)

func init() {
	authCmd.AddCommand(authLoginCmd)
}

var authLoginCmd = &cobra.Command{
	Use:   "login <provider>",
	Short: "Store an API key in the OS keychain",
	Long:  "Reads the API key from stdin and stores it in the OS keychain under the given provider name (e.g. openai, anthropic).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := args[0]

		fmt.Printf("Enter API key for %s: ", provider)
		reader := bufio.NewReader(os.Stdin)
		apiKey, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		apiKey = strings.TrimSpace(apiKey)

		if apiKey == "" {
			return fmt.Errorf("API key cannot be empty")
		}

		secrets, err := secret.New("")
		if err != nil {
			return err
		}

		if err := secrets.Set(provider, apiKey); err != nil {
			return fmt.Errorf("storing key: %w", err)
		}

		fmt.Printf("API key for %s stored via %s provider.\n", provider, secrets.Name())
		return nil
	},
}
