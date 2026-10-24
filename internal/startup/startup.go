// Package startup decides how glview starts: which mode (project or
// workspace), which host and token, and what the target project is.
package startup

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/gitremote"
)

type Mode int

const (
	ModeProject Mode = iota
	ModeWorkspace
)

// Inputs are everything Resolve looks at.
type Inputs struct {
	Config      config.Config
	ProjectFlag string // -p value: "group/project" or a project URL
	Cwd         string
	Getenv      func(string) string
	Glab        config.TokenProvider // nil when glab is unavailable
}

// Plan is the outcome of Resolve.
type Plan struct {
	Mode Mode

	HostURL     string
	HostSource  config.Source
	Token       string
	TokenSource config.Source

	// Project mode.
	Repo        *gitremote.Repo   // the local repository; nil for -p
	Remote      *gitremote.Remote // parsed remote; nil for -p or unparsable remote
	ProjectPath string            // from -p, or filled by Target once the host is known

	// Workspace mode.
	WorkspaceDir string
}

// Resolve determines mode, host and token. A missing host or token is not
// an error: the caller shows the setup screen.
func Resolve(ctx context.Context, in Inputs) (Plan, error) {
	if in.Getenv == nil {
		in.Getenv = func(string) string { return "" }
	}
	var p Plan
	var hostIn config.HostInput

	switch {
	case in.ProjectFlag != "":
		p.Mode = ModeProject
		hostURL, path, err := ParseProjectFlag(in.ProjectFlag, in.Config.Host)
		if err != nil {
			return p, err
		}
		hostIn.FlagURL = hostURL
		p.ProjectPath = path
	default:
		repo, err := gitremote.Find(in.Cwd)
		switch {
		case err == nil:
			p.Mode = ModeProject
			p.Repo = &repo
			if repo.RemoteURL != "" {
				if r, err := gitremote.ParseURL(repo.RemoteURL); err == nil {
					p.Remote = &r
					if !knownForeignHosts[r.Host] {
						hostIn.RemoteHost = r.Host
					}
				}
			}
		case errors.Is(err, gitremote.ErrNotRepo):
			p.Mode = ModeWorkspace
			p.WorkspaceDir = in.Cwd
		default:
			return p, err
		}
	}

	hostURL, src, err := config.ResolveHost(in.Config, hostIn, in.Getenv)
	if err != nil {
		return p, err
	}
	p.HostURL, p.HostSource = hostURL, src
	if hostURL != "" {
		p.Token, p.TokenSource = config.ResolveToken(ctx, in.Config, hostURL, in.Getenv, in.Glab)
	}
	return p, nil
}

// knownForeignHosts are never GitLab instances, so their remotes are not
// used to guess the GitLab host.
var knownForeignHosts = map[string]bool{
	"github.com": true, "bitbucket.org": true, "codeberg.org": true, "gitea.com": true,
	"dev.azure.com": true, "ssh.dev.azure.com": true, "git.sr.ht": true,
}

// Target is the project-mode outcome once the instance is known.
type Target struct {
	ProjectPath string // empty when the remote is not on the instance
	Reason      TargetReason
}

type TargetReason int

const (
	TargetOK TargetReason = iota
	// TargetForeign: the remote points to another host.
	TargetForeign
	// TargetNoRemote: no usable remote in the repository.
	TargetNoRemote
)

// Target matches the plan's remote against the instance.
func (p Plan) Target(hostURL string, aliases []string) (Target, error) {
	if p.ProjectPath != "" {
		return Target{ProjectPath: p.ProjectPath}, nil
	}
	if p.Remote == nil {
		return Target{Reason: TargetNoRemote}, nil
	}
	inst, err := gitremote.NewInstance(hostURL, aliases)
	if err != nil {
		return Target{}, err
	}
	path, ok := inst.ProjectPath(*p.Remote)
	if !ok {
		return Target{Reason: TargetForeign}, nil
	}
	return Target{ProjectPath: path}, nil
}

// ParseProjectFlag accepts "group/sub/project" or a project URL such as
// https://gitlab.example.com/group/project or .../group/project/-/merge_requests.
// For a URL it returns the host URL too; when cfgHost (with a relative URL
// root) prefixes the URL, the prefix is kept as part of the host.
func ParseProjectFlag(flag, cfgHost string) (hostURL, path string, err error) {
	flag = strings.TrimSpace(flag)
	if !strings.Contains(flag, "://") {
		path = strings.Trim(flag, "/")
		path = strings.TrimSuffix(path, ".git")
		if !strings.Contains(path, "/") {
			return "", "", fmt.Errorf("-p: expected group/project, got %q", flag)
		}
		return "", path, nil
	}

	if cfgHost != "" {
		if base, err := config.NormalizeHost(cfgHost); err == nil && strings.HasPrefix(flag, base+"/") {
			return base, cleanProjectPath(strings.TrimPrefix(flag, base+"/")), nil
		}
	}
	u, err := url.Parse(flag)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("-p: invalid URL %q", flag)
	}
	path = cleanProjectPath(u.Path)
	if !strings.Contains(path, "/") {
		return "", "", fmt.Errorf("-p: no project path in %q", flag)
	}
	return u.Scheme + "://" + u.Host, path, nil
}

func cleanProjectPath(p string) string {
	p = strings.Trim(p, "/")
	if i := strings.Index(p, "/-/"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSuffix(p, "/-")
	return strings.TrimSuffix(p, ".git")
}

// ParentDir is the folder "w" opens as a workspace from a foreign repo.
func (p Plan) ParentDir() string {
	if p.Repo != nil {
		return filepath.Dir(p.Repo.Root)
	}
	return p.WorkspaceDir
}
