// Package workspace describes what a workspace consists of: the projects
// of a folder (LocalDirSource) or of a GitLab group (GroupSource), and the
// Scope built from them — the rows of the Projects tab and the unique
// projects the batches ask about.
package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
)

// Project is one row of the workspace: the GitLab side and the local one.
type Project struct {
	Ref gitlab.ProjectRef // empty FullPath for foreign and remote-less folders
	// Local is the folder; nil for projects of a group. A folder without
	// .git has Local.Kind == gitremote.KindNoGit.
	Local *gitremote.LocalProject
	// Foreign marks a folder whose remote is not on the instance (or that
	// has no usable remote): shown grey.
	Foreign bool
	// Info is what the source already knows about the project (group
	// projects come with metadata); nil for folders.
	Info *gitlab.Project
}

// ProjectSource lists the projects of a workspace.
type ProjectSource interface {
	// Name is the label of the header and the breadcrumbs: "~/src",
	// "group: platform".
	Name() string
	Projects(ctx context.Context) ([]Project, error)
}

// LocalDirSource is the sub-folders of Dir (one level).
type LocalDirSource struct {
	Dir      string
	Instance gitremote.Instance
}

func (s LocalDirSource) Name() string { return HomeRel(s.Dir) }

// Projects scans the folder. Folders without .git are returned too (with
// Local.Kind == KindNoGit) for the "skipped" summary.
func (s LocalDirSource) Projects(context.Context) ([]Project, error) {
	entries, err := gitremote.Scan(s.Dir, s.Instance)
	if err != nil {
		return nil, err
	}
	out := make([]Project, len(entries))
	for i := range entries {
		e := &entries[i]
		out[i] = Project{Local: e, Foreign: e.Kind != gitremote.KindGitLab}
		if e.Kind == gitremote.KindGitLab {
			out[i].Ref = gitlab.ProjectRef{FullPath: e.ProjectPath}
		}
	}
	return out, nil
}

// GroupSource is the projects of a GitLab group and its subgroups: at most
// Limit non-archived ones, the most active first.
type GroupSource struct {
	Group  gitlab.Group
	Limit  int
	Client gitlab.Client
}

func (s GroupSource) Name() string { return "group: " + s.Group.FullPath }

// Projects returns the group's projects; the total number of projects of
// the group is reported by ProjectsTotal.
func (s GroupSource) Projects(ctx context.Context) ([]Project, error) {
	ps, _, err := s.ProjectsTotal(ctx)
	return ps, err
}

// ProjectsTotal is Projects with the total number of the group's projects
// ("showing 100 of 342").
func (s GroupSource) ProjectsTotal(ctx context.Context) ([]Project, int, error) {
	list, total, err := s.Client.Projects().ListGroupProjects(ctx, s.Group.ID, s.Limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Project, len(list))
	for i := range list {
		info := list[i]
		out[i] = Project{Ref: info.ProjectRef, Info: &info}
	}
	return out, total, nil
}

// Scope is a resolved workspace.
type Scope struct {
	Source  string    // ProjectSource.Name()
	Rows    []Project // folders with .git and group projects, in source order
	Skipped []string  // names of folders without .git
	Total   int       // all projects of a group (≥ number of rows); 0 for folders; -1: unknown, more than the rows

	refs    []gitlab.ProjectRef
	primary map[string]*gitremote.LocalProject
}

// NewScope builds the scope from the source's projects: rows, the skipped
// folders and the unique GitLab projects (two folders of one project give
// two rows but one project; the first folder is the project's primary one).
func NewScope(source string, ps []Project) Scope {
	s := Scope{Source: source, primary: map[string]*gitremote.LocalProject{}}
	seen := map[string]bool{}
	for _, p := range ps {
		if p.Local != nil && p.Local.Kind == gitremote.KindNoGit {
			s.Skipped = append(s.Skipped, p.Local.Name)
			continue
		}
		s.Rows = append(s.Rows, p)
		if p.Foreign || p.Ref.FullPath == "" {
			continue
		}
		if !seen[p.Ref.FullPath] {
			seen[p.Ref.FullPath] = true
			s.refs = append(s.refs, p.Ref)
			if p.Local != nil {
				s.primary[p.Ref.FullPath] = p.Local
			}
		}
	}
	return s
}

// Refs are the unique GitLab projects of the workspace.
func (s Scope) Refs() []gitlab.ProjectRef { return s.refs }

// GitLabCount is the number of unique GitLab projects.
func (s Scope) GitLabCount() int { return len(s.refs) }

// Primary returns the first folder of project path, nil if it has none.
func (s Scope) Primary(path string) *gitremote.LocalProject { return s.primary[path] }

// Local reports whether the workspace has local folders (a folder source).
func (s Scope) Local() bool {
	for _, r := range s.Rows {
		if r.Local != nil {
			return true
		}
	}
	return len(s.Skipped) > 0
}

// Repos returns the repositories of every row with a local folder,
// GitLab and foreign ones alike (fetch and pull touch both).
func (s Scope) Repos() []*gitremote.Repo {
	var out []*gitremote.Repo
	for _, r := range s.Rows {
		if r.Local != nil && r.Local.Repo != nil {
			out = append(out, r.Local.Repo)
		}
	}
	return out
}

// HomeRel shortens /Users/me/src to ~/src.
func HomeRel(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, dir); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + rel
	}
	return dir
}
