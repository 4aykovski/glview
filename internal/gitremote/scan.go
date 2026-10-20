package gitremote

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Kind classifies a workspace sub-folder.
type Kind int

const (
	// KindGitLab is a repository whose remote points to the instance.
	KindGitLab Kind = iota
	// KindForeign is a repository whose remote points elsewhere (GitHub,
	// another GitLab).
	KindForeign
	// KindNoRemote is a repository without a usable remote.
	KindNoRemote
	// KindNoGit is a folder without .git (skipped in the UI, counted in the summary).
	KindNoGit
)

// LocalProject is one sub-folder of a workspace.
type LocalProject struct {
	Dir         string // absolute path
	Name        string // folder name
	Kind        Kind
	Repo        *Repo   // nil for KindNoGit
	Remote      *Remote // nil when the remote URL is missing or unparsable
	ProjectPath string  // GitLab project path for KindGitLab
}

// Scan inspects the direct sub-folders of dir (one level). Hidden folders
// are skipped; symlinks to directories are followed. Results are sorted by
// folder name.
func Scan(dir string, inst Instance) ([]LocalProject, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []LocalProject
	)
	sem := make(chan struct{}, 8)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path) // follows symlinks
		if err != nil || !info.IsDir() {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			p := Classify(path, inst)
			mu.Lock()
			out = append(out, p)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Classify inspects a single folder.
func Classify(path string, inst Instance) LocalProject {
	p := LocalProject{Dir: path, Name: filepath.Base(path), Kind: KindNoGit}
	if _, err := os.Lstat(filepath.Join(path, ".git")); err != nil {
		return p
	}
	repo, err := Open(path)
	if err != nil {
		return p
	}
	p.Repo = &repo
	p.Kind = KindNoRemote
	if repo.RemoteURL == "" {
		return p
	}
	r, err := ParseURL(repo.RemoteURL)
	if err != nil {
		p.Kind = KindForeign
		return p
	}
	p.Remote = &r
	if path, ok := inst.ProjectPath(r); ok {
		p.Kind = KindGitLab
		p.ProjectPath = path
		return p
	}
	p.Kind = KindForeign
	return p
}
