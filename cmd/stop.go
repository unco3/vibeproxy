package cmd

import (
	"fmt"

	"vibeproxy/internal/daemon"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(stopCmd)
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running VibeProxy server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Stop(); err != nil {
			return err
		}
		fmt.Println("VibeProxy stopped.")
		return nil
	},
}
