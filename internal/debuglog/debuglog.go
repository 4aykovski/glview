// Package debuglog writes an optional debug log with secrets redacted.
//
// Logging is disabled unless Open is called (the --debug flag). Every line
// passes through a redactor that masks registered secrets (tokens) and
// anything that looks like a GitLab token.
package debuglog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const mask = "[REDACTED]"

// gitlabTokenRe matches well-known GitLab token prefixes (PAT, OAuth, deploy,
// runner, CI job tokens and friends).
var gitlabTokenRe = regexp.MustCompile(`\b(glpat|gloas|gldt|glrt|glcbt|glptt|glft|glimt|glagent|glsoat|glffct)-[A-Za-z0-9_.\-]{8,}`)

var (
	mu      sync.RWMutex
	secrets []string
	logger  = slog.New(slog.DiscardHandler)
)

// RegisterSecret makes the redactor mask s in all subsequent log output.
// Empty and very short values are ignored to avoid masking common words.
func RegisterSecret(s string) {
	s = strings.TrimSpace(s)
	if len(s) < 8 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range secrets {
		if existing == s {
			return
		}
	}
	secrets = append(secrets, s)
}

// Redact returns s with registered secrets and token-like strings masked.
func Redact(s string) string {
	mu.RLock()
	for _, sec := range secrets {
		s = strings.ReplaceAll(s, sec, mask)
	}
	mu.RUnlock()
	return gitlabTokenRe.ReplaceAllString(s, mask)
}

// Open starts logging to path (directories are created) and returns a closer.
func Open(path string) (io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open debug log: %w", err)
	}
	h := slog.NewTextHandler(&redactingWriter{w: f}, &slog.HandlerOptions{Level: slog.LevelDebug})
	mu.Lock()
	logger = slog.New(h)
	mu.Unlock()
	return f, nil
}

// L returns the current logger (a no-op logger unless Open was called).
func L() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

type redactingWriter struct{ w io.Writer }

func (r *redactingWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(r.w, Redact(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}
