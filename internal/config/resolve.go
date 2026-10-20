package config

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Source tells where a host or token came from. Shown in Help and used to
// pick the right hint on 401.
type Source string

const (
	SourceNone   Source = ""
	SourceFlag   Source = "flag"
	SourceConfig Source = "config"
	SourceEnv    Source = "env"
	SourceRemote Source = "remote"
	SourceGlab   Source = "glab"
)

// TokenProvider returns a token for a hostname (implemented by internal/glab).
// An empty token with a nil error means "no token for this host".
type TokenProvider interface {
	Token(ctx context.Context, hostname string) (string, error)
}

// HostInput collects the candidates for the GitLab host, highest priority first.
type HostInput struct {
	FlagURL    string // host part of a full project URL passed with -p
	RemoteHost string // hostname from the current repository's remote
}

// ResolveHost picks the instance URL: -p URL → config → $GITLAB_HOST → remote.
// It returns an empty URL and SourceNone when nothing is known (→ setup screen).
func ResolveHost(cfg Config, in HostInput, getenv func(string) string) (string, Source, error) {
	candidates := []struct {
		raw string
		src Source
	}{
		{in.FlagURL, SourceFlag},
		{cfg.Host, SourceConfig},
		{getenv("GITLAB_HOST"), SourceEnv},
		{in.RemoteHost, SourceRemote},
	}
	for _, c := range candidates {
		if strings.TrimSpace(c.raw) == "" {
			continue
		}
		u, err := NormalizeHost(c.raw)
		if err != nil {
			return "", c.src, fmt.Errorf("%s host %q: %w", c.src, c.raw, err)
		}
		return u, c.src, nil
	}
	return "", SourceNone, nil
}

// ResolveToken picks the token for hostURL: config → $GITLAB_TOKEN → glab.
// The config token only applies when the config host is empty or matches.
// glab may be nil (not installed or disabled).
func ResolveToken(ctx context.Context, cfg Config, hostURL string, getenv func(string) string, glab TokenProvider) (string, Source) {
	if cfg.Token != "" && (cfg.Host == "" || sameHost(cfg.Host, hostURL)) {
		return cfg.Token, SourceConfig
	}
	if t := strings.TrimSpace(getenv("GITLAB_TOKEN")); t != "" {
		return t, SourceEnv
	}
	if glab != nil && hostURL != "" {
		if t, err := glab.Token(ctx, Hostname(hostURL)); err == nil && t != "" {
			return t, SourceGlab
		}
	}
	return "", SourceNone
}

// NormalizeHost turns "gitlab.example.com", "https://gitlab.example.com/",
// or "https://example.com/gitlab" into a scheme-qualified URL without a
// trailing slash. The path prefix (relative URL root) is preserved.
func NormalizeHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment, u.User = "", "", nil
	return u.String(), nil
}

// Hostname returns the bare hostname (no scheme, port or path) of a host URL.
func Hostname(hostURL string) string {
	u, err := url.Parse(hostURL)
	if err != nil || u.Host == "" {
		return strings.ToLower(hostURL)
	}
	return strings.ToLower(u.Hostname())
}

func sameHost(a, b string) bool {
	na, err1 := NormalizeHost(a)
	nb, err2 := NormalizeHost(b)
	return err1 == nil && err2 == nil && na == nb
}
