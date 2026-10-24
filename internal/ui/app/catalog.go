package app

import (
	"context"
	"slices"
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
	Current  bool // the project of the folder glview was started in
	Starred  bool
	OpenMRs  int       // -1 when unknown
	Activity time.Time // zero when unknown
	Repo     *gitremote.Repo
}

// ProjectCatalog is what the project picker (p) and the local part of the
// global search (F) read. It never makes network requests.
type ProjectCatalog interface {
	Current() *CatalogEntry
	// Workspace lists the projects of the topmost Workspace of the stack.
	Workspace() []CatalogEntry
	Starred() []CatalogEntry
	Recent() []CatalogEntry
	// LocalRepo returns a known local folder of project path, nil if none.
	LocalRepo(path string) *gitremote.Repo
}

// WorkspaceRoot is implemented by the Workspace screen. Opening a project
// from search puts it above the topmost Workspace of the stack, and the
// catalog lists that Workspace's projects.
type WorkspaceRoot interface {
	Screen
	CatalogEntries() []CatalogEntry
	LocalRepo(path string) *gitremote.Repo
}

// Catalog implements ProjectCatalog over the disk snapshots (starred) and
// the state file (recent projects), both kept per host: the catalog shows
// the projects of the current host only. Methods are safe on a nil
// *Catalog.
type Catalog struct {
	mu        sync.Mutex
	disk      *cache.Disk
	statePath string
	state     cache.State
	host      func() string // the current host (it is known after connecting)
	current   *CatalogEntry
	ws        WorkspaceRoot
	starred   map[string]*starredSnap // by host, read from disk on first use
	stats     map[string]CatalogEntry
	now       func() time.Time

	// Saves run off the UI goroutine; version orders them so an older
	// copy of the state never overwrites a newer one.
	version uint64
	saveMu  sync.Mutex
	saved   uint64
}

// starredSnap is the starred list of one host.
type starredSnap struct {
	projects []gitlab.Project
	at       time.Time
}

var _ ProjectCatalog = (*Catalog)(nil)

// NewCatalog reads the recent projects from statePath ("" keeps them in
// memory only); starred snapshots come from disk (may be nil). host returns
// the current GitLab host (nil: one unnamed host).
func NewCatalog(disk *cache.Disk, statePath string, host func() string) *Catalog {
	c := &Catalog{disk: disk, statePath: statePath, host: host,
		starred: map[string]*starredSnap{}, stats: map[string]CatalogEntry{}, now: time.Now}
	if statePath != "" {
		c.state = cache.LoadState(statePath)
	}
	return c
}

// curHost is the current host (the caller holds mu).
func (c *Catalog) curHost() string {
	if c.host == nil {
		return ""
	}
	return c.host()
}

// starredOf returns the starred snapshot of host, reading it from disk
// the first time (the caller holds mu).
func (c *Catalog) starredOf(host string) *starredSnap {
	if s, ok := c.starred[host]; ok {
		return s
	}
	s := &starredSnap{}
	if c.disk != nil {
		if ps, at, ok := c.disk.LoadStarred(host); ok {
			s.projects, s.at = ps, at
		}
	}
	c.starred[host] = s
	return s
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

// SetWorkspace registers the topmost Workspace of the stack (nil when
// there is none).
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
	starred := c.starredOf(c.curHost()).projects
	out := make([]CatalogEntry, 0, len(starred))
	for _, p := range starred {
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
	recent := c.state.Recent(c.curHost())
	out := make([]CatalogEntry, 0, len(recent))
	for _, p := range recent {
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
	for _, p := range c.starredOf(c.curHost()).projects {
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
	defer c.mu.Unlock()
	c.state.Touch(c.curHost(), path)
	return c.saveLocked()
}

// Expanded returns the expanded groups of tab ("mrs", "pipelines") of
// workspace source on the current host.
func (c *Catalog) Expanded(source, tab string) []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.state.Expanded(c.curHost(), source, tab)...)
}

// SetExpanded remembers the expanded groups of tab of workspace source and
// saves the state file in the background.
func (c *Catalog) SetExpanded(source, tab string, paths []string) tea.Cmd {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.SetExpanded(c.curHost(), source, tab, paths, c.now())
	return c.saveLocked()
}

// Selected returns the projects marked in the P filter of tab ("mrs",
// "pipelines") of workspace source on the current host.
func (c *Catalog) Selected(source, tab string) []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.state.Selected(c.curHost(), source, tab)...)
}

// SetSelected remembers the projects marked in the P filter of tab of
// workspace source (an empty list erases them) and saves the state file in
// the background. An unchanged list writes nothing.
func (c *Catalog) SetSelected(source, tab string, paths []string) tea.Cmd {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if slices.Equal(c.state.Selected(c.curHost(), source, tab), paths) {
		return nil
	}
	c.state.SetSelected(c.curHost(), source, tab, paths, c.now())
	return c.saveLocked()
}

// OpenWorkspace marks workspace source opened now (the oldest workspaces
// are forgotten first). Without a record nothing is written.
func (c *Catalog) OpenWorkspace(source string) tea.Cmd {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.state.TouchWorkspace(c.curHost(), source, c.now()) {
		return nil
	}
	return c.saveLocked()
}

// saveLocked returns a command writing a copy of the state (the caller
// holds mu).
func (c *Catalog) saveLocked() tea.Cmd {
	if c.statePath == "" {
		return nil
	}
	c.version++
	st, file, v := c.state.Clone(), c.statePath, c.version
	return func() tea.Msg {
		c.saveMu.Lock()
		defer c.saveMu.Unlock()
		if v <= c.saved {
			return nil // a newer state is already on disk
		}
		if err := cache.SaveState(file, st); err != nil {
			debuglog.L().Debug("save state", "err", err)
			return nil
		}
		c.saved = v
		return nil
	}
}

// StarredStale reports whether the starred snapshot of the current host is
// missing or older than an hour.
func (c *Catalog) StarredStale() bool {
	if c == nil || c.disk == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.disk.Fresh(c.starredOf(c.curHost()).at, cache.StarredMaxAge)
}

type starredLoadedMsg struct {
	host     string
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
	c.mu.Lock()
	host := c.curHost()
	c.mu.Unlock()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		ps, err := client.Projects().Starred(ctx)
		if err == nil && disk != nil {
			if err := disk.SaveStarred(host, ps); err != nil {
				debuglog.L().Debug("save starred", "err", err)
			}
		}
		return starredLoadedMsg{host: host, projects: ps, err: err}
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
	c.starred[msg.host] = &starredSnap{projects: msg.projects, at: time.Now()}
}
