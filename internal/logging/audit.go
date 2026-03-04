package logging

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	Service   string `json:"service"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	Duration  string `json:"duration"`
	Agent     string `json:"agent,omitempty"`
}

type AuditLogger struct {
	mu     sync.Mutex
	writer io.Writer
}

func NewAuditLogger() (*AuditLogger, error) {
	dir := filepath.Join(os.Getenv("HOME"), ".vibeproxy")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(filepath.Join(dir, "audit.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &AuditLogger{writer: f}, nil
}

// NewAuditLoggerWriter creates an AuditLogger that writes to w (useful for testing).
func NewAuditLoggerWriter(w io.Writer) *AuditLogger {
	return &AuditLogger{writer: w}
}

func (a *AuditLogger) Log(service, method, path string, status int, duration time.Duration, agent string) {
	entry := AuditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Service:   service,
		Method:    method,
		Path:      path,
		Status:    status,
		Duration:  duration.String(),
		Agent:     agent,
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	json.NewEncoder(a.writer).Encode(entry)
}
