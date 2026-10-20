package app

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
)

// CatalogEntry is a project known without asking GitLab: the current
// folder's project, a workspace project, a starred or a recent one.
type CatalogEntry struct {
	Path     string
	Current  bool // the project of the folder gitlab-tui was started in
	Starred  bool
	OpenMRs  int       // -1 when unknown
	Activity time.Time // zero when unknown
	Repo     *gitremote.Repo
}

// ProjectCatalog is what the project picker (p) and the local part of the
// global search (F) read. It never makes network requests.
type ProjectCatalog interface {
	Current() *CatalogEntry
	// Workspace lists the projects of the Workspace at the bottom of the stack.
	Workspace() []CatalogEntry
	Starred() []CatalogEntry
	Recent() []CatalogEntry
	// LocalRepo returns a known local folder of project path, nil if none.
	LocalRepo(path string) *gitremote.Repo
}

// WorkspaceRoot is implemented by the Workspace screen. When it is the
// root of the stack, opening a project from search keeps it below the
// project, and the catalog lists its projects.
type WorkspaceRoot interface {
	Screen
	CatalogEntries() []CatalogEntry
	LocalRepo(path string) *gitremote.Repo
}

// Catalog implements ProjectCatalog over the disk snapshots (starred) and
// the state file (recent projects). Methods are safe on a nil *Catalog.
type Catalog struct {
	mu        sync.Mutex
	disk      *cache.Disk
	statePath string
	state     cache.State
	current   *CatalogEntry
	ws        WorkspaceRoot
	starred   []gitlab.Project
	starredAt time.Time
	stats     map[string]CatalogEntry
}

var _ ProjectCatalog = (*Catalog)(nil)

// NewCatalog loads the starred snapshot from disk (may be nil) and the
// recent projects from statePath ("" keeps them in memory only).
func NewCatalog(disk *cache.Disk, statePath string) *Catalog {
	c := &Catalog{disk: disk, statePath: statePath, stats: map[string]CatalogEntry{}}
	if statePath != "" {
		c.state = cache.LoadState(statePath)
	}
	if disk != nil {
		if ps, at, ok := disk.LoadStarred(); ok {
			c.starred, c.starredAt = ps, at
		}
	}
	return c
}

// SetCurrent records the project of the start folder.
func (c *Catalog) SetCurrent(path string, repo *gitremote.Repo) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = &CatalogEntry{Path: path, Current: true, OpenMRs: -1, Repo: repo}
}

// SetWorkspace registers the Workspace at the bottom of the stack (nil
// when the root is another screen).
func (c *Catalog) SetWorkspace(w WorkspaceRoot) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ws = w
}

// Note remembers what a workspace knows about projects (open MRs,
// activity) to show it next to starred and recent entries.
func (c *Catalog) Note(entries ...CatalogEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range entries {
		c.stats[e.Path] = e
	}
}

func (c *Catalog) Current() *CatalogEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return nil
	}
	e := c.withStats(*c.current)
	return &e
}

func (c *Catalog) Workspace() []CatalogEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	ws := c.ws
	c.mu.Unlock()
	if ws == nil {
		return nil
	}
	return ws.CatalogEntries()
}

func (c *Catalog) Starred() []CatalogEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CatalogEntry, 0, len(c.starred))
	for _, p := range c.starred {
		e := c.withStats(CatalogEntry{Path: p.FullPath, Starred: true, OpenMRs: -1, Activity: p.LastActivityAt})
		e.Starred = true
		out = append(out, e)
	}
	return out
}

func (c *Catalog) Recent() []CatalogEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CatalogEntry, 0, len(c.state.Recent))
	for _, p := range c.state.Recent {
		out = append(out, c.withStats(CatalogEntry{Path: p, OpenMRs: -1}))
	}
	return out
}

// withStats fills unknown fields from the notes and the starred list
// (the caller holds mu).
func (c *Catalog) withStats(e CatalogEntry) CatalogEntry {
	if s, ok := c.stats[e.Path]; ok {
		if e.OpenMRs < 0 {
			e.OpenMRs = s.OpenMRs
		}
		if e.Activity.IsZero() {
			e.Activity = s.Activity
		}
	}
	for _, p := range c.starred {
		if p.FullPath == e.Path {
			e.Starred = true
			if e.Activity.IsZero() {
				e.Activity = p.LastActivityAt
			}
		}
	}
	return e
}

func (c *Catalog) LocalRepo(path string) *gitremote.Repo {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	cur, ws := c.current, c.ws
	c.mu.Unlock()
	if cur != nil && cur.Path == path && cur.Repo != nil {
		return cur.Repo
	}
	if ws != nil {
		return ws.LocalRepo(path)
	}
	return nil
}

// Touch moves path to the front of the recent projects and saves the
// state file in the background.
func (c *Catalog) Touch(path string) tea.Cmd {
	if c == nil || path == "" {
		return nil
	}
	c.mu.Lock()
	c.state.Touch(path)
	st := cache.State{Recent: append([]string(nil), c.state.Recent...)}
	file := c.statePath
	c.mu.Unlock()
	if file == "" {
		return nil
	}
	return func() tea.Msg {
		if err := cache.SaveState(file, st); err != nil {
			debuglog.L().Debug("save state", "err", err)
		}
		return nil
	}
}

// StarredStale reports whether the starred snapshot is missing or older
// than an hour.
func (c *Catalog) StarredStale() bool {
	if c == nil || c.disk == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.disk.Fresh(c.starredAt, cache.StarredMaxAge)
}

type starredLoadedMsg struct {
	projects []gitlab.Project
	err      error
}

// refreshStarred fetches the starred projects in the background; a failure
// keeps the old snapshot and is only logged.
func (c *Catalog) refreshStarred(client gitlab.Client) tea.Cmd {
	if c == nil || client == nil || !c.StarredStale() {
		return nil
	}
	disk := c.disk
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		ps, err := client.Projects().Starred(ctx)
		if err == nil && disk != nil {
			if err := disk.SaveStarred(ps); err != nil {
				debuglog.L().Debug("save starred", "err", err)
			}
		}
		return starredLoadedMsg{projects: ps, err: err}
	}
}

func (c *Catalog) onStarred(msg starredLoadedMsg) {
	if c == nil {
		return
	}
	if msg.err != nil {
		debuglog.L().Debug("starred refresh failed", "err", msg.err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.starred, c.starredAt = msg.projects, time.Now()
}
