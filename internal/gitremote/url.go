package gitremote

import (
	"fmt"
	"net/url"
	"strings"
)

// Remote is a parsed remote URL.
type Remote struct {
	Host string // lower-case hostname without port (or an SSH alias)
	Path string // repository path without leading "/" and trailing ".git"
}

// ParseURL parses git remote URLs:
//
//	git@host:group/sub/proj.git
//	ssh://git@host[:port]/group/sub/proj.git
//	https://host[:port][/prefix]/group/sub/proj[.git]
//	http://…, git://…
func ParseURL(raw string) (Remote, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Remote{}, fmt.Errorf("empty remote URL")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Remote{}, err
		}
		switch u.Scheme {
		case "ssh", "git+ssh", "http", "https", "git":
		default:
			return Remote{}, fmt.Errorf("unsupported remote scheme %q", u.Scheme)
		}
		if u.Hostname() == "" {
			return Remote{}, fmt.Errorf("remote URL without host: %q", raw)
		}
		return Remote{Host: strings.ToLower(u.Hostname()), Path: cleanPath(u.Path)}, nil
	}

	// scp-like syntax: [user@]host:path. A colon before any slash is required.
	colon := strings.IndexByte(raw, ':')
	slash := strings.IndexByte(raw, '/')
	if colon <= 0 || (slash >= 0 && slash < colon) {
		return Remote{}, fmt.Errorf("unsupported remote URL %q", raw)
	}
	host := raw[:colon]
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		host = host[at+1:]
	}
	return Remote{Host: strings.ToLower(host), Path: cleanPath(raw[colon+1:])}, nil
}

func cleanPath(p string) string {
	p = strings.Trim(p, "/")
	p = strings.TrimSuffix(p, ".git")
	return strings.Trim(p, "/")
}

// Instance describes the configured GitLab instance for matching remotes.
type Instance struct {
	Host       string   // lower-case hostname
	PathPrefix string   // relative URL root without slashes, e.g. "gitlab"
	Aliases    []string // SSH aliases that mean this host
}

// NewInstance builds an Instance from the host URL in the config.
func NewInstance(hostURL string, aliases []string) (Instance, error) {
	u, err := url.Parse(hostURL)
	if err != nil || u.Hostname() == "" {
		return Instance{}, fmt.Errorf("invalid instance URL %q", hostURL)
	}
	inst := Instance{
		Host:       strings.ToLower(u.Hostname()),
		PathPrefix: strings.Trim(u.Path, "/"),
	}
	for _, a := range aliases {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			inst.Aliases = append(inst.Aliases, a)
		}
	}
	return inst, nil
}

// Owns reports whether the remote host belongs to this instance. Ports are
// ignored because SSH and HTTP ports usually differ on self-hosted setups.
func (i Instance) Owns(r Remote) bool {
	if r.Host == i.Host {
		return true
	}
	for _, a := range i.Aliases {
		if r.Host == a {
			return true
		}
	}
	return false
}

// ProjectPath returns the GitLab project path for a remote of this instance,
// stripping the relative URL root when present (HTTP remotes include it,
// SSH remotes do not).
func (i Instance) ProjectPath(r Remote) (string, bool) {
	if !i.Owns(r) || r.Path == "" {
		return "", false
	}
	p := r.Path
	if i.PathPrefix != "" {
		if rest, ok := strings.CutPrefix(p, i.PathPrefix+"/"); ok {
			p = rest
		}
	}
	if !strings.Contains(p, "/") {
		return "", false // GitLab projects always live in a namespace
	}
	return p, true
}
