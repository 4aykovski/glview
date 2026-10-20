// Package glab integrates the optional glab CLI: discovery, reading the
// token for a host, and (later) exec actions. glab is never used as a Go
// library and its output is never parsed in place of the GitLab API.
package glab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// DefaultTimeout bounds every non-interactive glab call.
const DefaultTimeout = 3 * time.Second

// Glab is a discovered glab binary.
type Glab struct {
	Path    string
	Timeout time.Duration
}

// Detect finds glab. path overrides the PATH lookup; disabled turns the
// integration off. It returns nil when glab is unavailable.
func Detect(path string, disabled bool) *Glab {
	if disabled {
		return nil
	}
	if path == "" {
		path = "glab"
	}
	full, err := exec.LookPath(path)
	if err != nil {
		return nil
	}
	return &Glab{Path: full, Timeout: DefaultTimeout}
}

// Token returns glab's token for hostname. It first asks glab for the value
// stored in its config, then falls back to `auth status --show-token`
// (needed when the token lives in the OS keyring). Empty result with nil
// error means glab has no token for the host.
func (g *Glab) Token(ctx context.Context, hostname string) (string, error) {
	out, err := g.run(ctx, "config", "get", "token", "--host", hostname)
	if err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			return t, nil
		}
	} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "", err
	}

	out, err = g.run(ctx, "auth", "status", "--hostname", hostname, "--show-token", "--output", "json")
	if err != nil {
		return "", err
	}
	return findToken(out), nil
}

// LoggedIn reports whether glab is authenticated against hostname.
func (g *Glab) LoggedIn(ctx context.Context, hostname string) bool {
	_, err := g.run(ctx, "auth", "status", "--hostname", hostname)
	return err == nil
}

func (g *Glab) run(ctx context.Context, args ...string) ([]byte, error) {
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, g.Path, args...)
	cmd.Stdin = nil
	cmd.WaitDelay = 500 * time.Millisecond
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// findToken extracts the token from `glab auth status --output json`. The
// exact schema is not documented, so it searches for the first non-empty
// string value under a key named "token" at any depth.
func findToken(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	return walkToken(v)
}

func walkToken(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := slices.Sorted(maps.Keys(t))
		for _, k := range keys {
			val := t[k]
			if strings.EqualFold(k, "token") {
				if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
		for _, k := range keys {
			if s := walkToken(t[k]); s != "" {
				return s
			}
		}
	case []any:
		for _, val := range t {
			if s := walkToken(val); s != "" {
				return s
			}
		}
	}
	return ""
}
