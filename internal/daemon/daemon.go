package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var pidDir = filepath.Join(os.Getenv("HOME"), ".vibeproxy")
var pidFile = filepath.Join(pidDir, "vibeproxy.pid")

// WritePID atomically writes the current process PID.
// Uses O_CREATE|O_EXCL to prevent races when multiple processes start simultaneously.
func WritePID() error {
	if err := os.MkdirAll(pidDir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(pidFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("PID file already exists — another instance may be running (use 'vibe stop' first)")
		}
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%d", os.Getpid())
	return err
}

func ReadPID() (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func RemovePID() {
	os.Remove(pidFile)
}

// CleanStalePID removes a PID file if the referenced process is no longer running.
// Returns true if a stale PID was cleaned up.
func CleanStalePID() bool {
	pid, err := ReadPID()
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		RemovePID()
		return true
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		slog.Info("cleaned stale PID file", "pid", pid)
		RemovePID()
		return true
	}
	return false
}

func IsRunning() (int, bool) {
	pid, err := ReadPID()
	if err != nil {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process no longer exists — clean up stale PID
		RemovePID()
		return 0, false
	}
	return pid, true
}

// Stop sends SIGTERM and waits for the process to exit.
func Stop() error {
	return StopWithTimeout(10 * time.Second)
}

// StopWithTimeout sends SIGTERM and waits up to timeout for the process to exit.
func StopWithTimeout(timeout time.Duration) error {
	pid, running := IsRunning()
	if !running {
		return fmt.Errorf("vibeproxy is not running")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to pid %d: %w", pid, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			RemovePID()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("process %d did not exit within %s", pid, timeout)
}
