// Package gitremote detects the GitLab project of a local repository by
// reading .git files directly (git is never executed) and scans a folder
// for repositories in workspace mode.
package gitremote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotRepo means no .git was found from the start directory up to the root.
var ErrNotRepo = errors.New("not a git repository")

// ErrNoRemote means the repository has no usable remote (no origin and not
// exactly one remote).
var ErrNoRemote = errors.New("no usable remote")

// Repo is a local repository as seen on disk.
type Repo struct {
	Root      string // working tree root
	GitDir    string // per-worktree git dir (HEAD lives here)
	CommonDir string // shared git dir (config lives here); == GitDir for plain repos
	Remote    string // remote name, usually "origin"; empty if none usable
	RemoteURL string
	Branch    string // branch name, or short SHA when detached
	Detached  bool
}

// Find looks for .git starting at dir and walking up to the filesystem root.
func Find(dir string) (Repo, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Repo{}, err
	}
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return Open(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Repo{}, ErrNotRepo
		}
		dir = parent
	}
}

// Open reads the repository whose working tree root is root. The remote
// may be missing: then Remote and RemoteURL are empty and no error is
// returned, so callers can still show the folder.
func Open(root string) (Repo, error) {
	gitDir, err := resolveGitDir(filepath.Join(root, ".git"))
	if err != nil {
		return Repo{}, err
	}
	r := Repo{Root: root, GitDir: gitDir, CommonDir: commonDir(gitDir)}

	if f, err := os.Open(filepath.Join(r.CommonDir, "config")); err == nil {
		cfg := parseGitConfig(f)
		f.Close()
		r.Remote, r.RemoteURL = pickRemote(cfg)
	}
	if err := r.RefreshHEAD(); err != nil {
		return Repo{}, err
	}
	return r, nil
}

// RefreshHEAD re-reads HEAD (e.g. after a checkout outside the TUI).
func (r *Repo) RefreshHEAD() error {
	data, err := os.ReadFile(filepath.Join(r.GitDir, "HEAD"))
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	head := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(head, "ref:"); ok {
		r.Branch = strings.TrimPrefix(strings.TrimSpace(ref), "refs/heads/")
		r.Detached = false
		return nil
	}
	r.Detached = true
	r.Branch = head
	if len(head) > 8 {
		r.Branch = head[:8]
	}
	return nil
}

// resolveGitDir handles both a .git directory and a .git file containing
// "gitdir: <path>" (worktrees, submodules).
func resolveGitDir(dotGit string) (string, error) {
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotRepo, err)
	}
	if info.IsDir() {
		return dotGit, nil
	}
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	path, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", fmt.Errorf("%w: malformed .git file", ErrNotRepo)
	}
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(dotGit), path)
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("%w: gitdir %s: %v", ErrNotRepo, path, err)
	}
	return path, nil
}

// commonDir follows the "commondir" file of a linked worktree.
func commonDir(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return gitDir
	}
	p := strings.TrimSpace(string(data))
	if !filepath.IsAbs(p) {
		p = filepath.Join(gitDir, p)
	}
	return filepath.Clean(p)
}

// pickRemote prefers origin; otherwise uses the only remote if there is one.
func pickRemote(cfg gitConfig) (name, url string) {
	if u := cfg.get("remote.origin", "url"); u != "" {
		return "origin", u
	}
	if names := cfg.remotes(); len(names) == 1 {
		return names[0], cfg.get("remote."+names[0], "url")
	}
	return "", ""
}
