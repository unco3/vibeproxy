package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "vibe",
	Short: "VibeProxy — local proxy that keeps API keys out of LLM context",
	Long:  "VibeProxy intercepts API requests from AI agents, swapping dummy tokens for real API keys stored in your OS keychain.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		jsonLog, _ := cmd.Flags().GetBool("json-log")
		debug, _ := cmd.Flags().GetBool("debug")

		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}

		opts := &slog.HandlerOptions{Level: level}

		var handler slog.Handler
		if jsonLog {
			handler = slog.NewJSONHandler(os.Stderr, opts)
		} else {
			handler = slog.NewTextHandler(os.Stderr, opts)
		}
		slog.SetDefault(slog.New(handler))
	},
}

func init() {
	rootCmd.PersistentFlags().Bool("json-log", false, "Output structured JSON logs")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug-level logging")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
