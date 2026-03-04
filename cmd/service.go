package cmd

import (
	"fmt"
	"os"
	"runtime"

	"vibeproxy/internal/daemon"

	"github.com/spf13/cobra"
)

func init() {
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	rootCmd.AddCommand(serviceCmd)
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage VibeProxy as an OS-level service",
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install VibeProxy as a system service (auto-start on boot, auto-restart on crash)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("service install is currently supported on macOS only (launchd)")
		}

		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		if err := daemon.InstallLaunchd(dir); err != nil {
			return err
		}
		fmt.Println("VibeProxy installed as launchd service.")
		fmt.Println("  - Auto-starts on login")
		fmt.Println("  - Auto-restarts on crash")
		fmt.Println("  - Logs: ~/.vibeproxy/stdout.log, ~/.vibeproxy/stderr.log")
		return nil
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove VibeProxy from system services",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("service uninstall is currently supported on macOS only (launchd)")
		}

		if err := daemon.UninstallLaunchd(); err != nil {
			return err
		}
		fmt.Println("VibeProxy removed from launchd services.")
		return nil
	},
}
