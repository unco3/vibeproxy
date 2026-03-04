package cmd

import (
	"fmt"

	"vibeproxy/internal/daemon"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show VibeProxy server status",
	Run: func(cmd *cobra.Command, args []string) {
		if pid, running := daemon.IsRunning(); running {
			fmt.Printf("VibeProxy is running (pid %d)\n", pid)
		} else {
			fmt.Println("VibeProxy is not running.")
		}
	},
}
