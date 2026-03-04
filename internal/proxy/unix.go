package proxy

import (
	"fmt"
	"log/slog"
	"net"
	"os"
)

func (s *Server) listenUnix() error {
	path := s.cfg.ListenUnix

	// Remove stale socket file if it exists
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing stale socket %s: %w", path, err)
		}
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("listening on unix socket %s: %w", path, err)
	}
	s.ln = ln

	// Set socket permissions (owner read/write only)
	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("setting socket permissions: %w", err)
	}

	slog.Info("VibeProxy listening", "unix", path)
	return s.httpSrv.Serve(ln)
}
