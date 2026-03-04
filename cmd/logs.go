package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	logsCmd.Flags().IntP("tail", "n", 20, "Number of lines to show")
	rootCmd.AddCommand(logsCmd)
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show recent audit log entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		n, _ := cmd.Flags().GetInt("tail")
		logPath := filepath.Join(os.Getenv("HOME"), ".vibeproxy", "audit.log")

		data, err := os.ReadFile(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No audit log found. Start the proxy first.")
				return nil
			}
			return err
		}

		lines := splitLines(string(data))
		if len(lines) > n {
			lines = lines[len(lines)-n:]
		}
		for _, line := range lines {
			fmt.Println(line)
		}
		return nil
	},
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
