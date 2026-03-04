package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/unco3/vibeproxy/internal/config"
	"github.com/unco3/vibeproxy/internal/daemon"
	"github.com/unco3/vibeproxy/internal/proxy"
	"github.com/unco3/vibeproxy/internal/secret"

	"github.com/spf13/cobra"
)

func init() {
	startCmd.Flags().Bool("foreground", false, "Run in foreground (used internally for daemon mode)")
	rootCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the VibeProxy server",
	RunE: func(cmd *cobra.Command, args []string) error {
		fg, _ := cmd.Flags().GetBool("foreground")

		if !fg {
			return daemonize()
		}
		return runForeground()
	},
}

func daemonize() error {
	// Clean up stale PID file from a previous crash
	daemon.CleanStalePID()

	if pid, running := daemon.IsRunning(); running {
		return fmt.Errorf("vibeproxy already running (pid %d)", pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	child := exec.Command(exe, "start", "--foreground")
	child.Dir, _ = os.Getwd()
	child.Env = os.Environ()

	// Detach from parent
	child.Stdout = nil
	child.Stderr = nil
	child.Stdin = nil
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := child.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	fmt.Printf("VibeProxy started (pid %d)\n", child.Process.Pid)
	return nil
}

func runForeground() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}

	secrets, err := secret.New(cfg.SecretBackend)
	if err != nil {
		return err
	}

	srv, err := proxy.NewServer(cfg, secrets)
	if err != nil {
		return err
	}

	if err := daemon.WritePID(); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer daemon.RemovePID()

	// Graceful shutdown on signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case sig := <-sigCh:
		slog.Info("shutting down", "signal", sig.String())
		return srv.Shutdown(5 * time.Second)
	case err := <-errCh:
		return err
	}
}
