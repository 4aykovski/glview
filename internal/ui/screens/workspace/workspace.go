// Package workspace is the root screen of workspace mode: the Projects tab
// (overview of every project of a folder or a group) and the MRs and
// Pipelines tabs grouped by project (SPEC §3.3).
package workspace

import (
	"context"
	"errors"
	"slices"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitexec"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/localgit"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/workspace"
)

const requestTimeout = 15 * time.Second

// Tabs of the workspace.
const (
	TabProjects = iota
	TabMRs
	TabPipelines
	TabTags
)

// shared is the state every tab reads: the scope and the project overview.
type shared struct {
	env    *app.Env
	source string
	group  *workspace.GroupSource // nil for a folder
	scope  workspace.Scope
	ready  bool  // the scope is known (a group loads it first)
	err    error // the group's project list failed to load (r retries)

	over    map[string]gitlab.ProjectOverview // by project path
	errs    map[string]error                  // per-project overview errors
	order   []string                          // project paths, most active first
	updated time.Time
	loading bool // an overview run is in flight
}

// overview returns the overview of path and whether it is known.
func (sh *shared) overview(path string) (gitlab.ProjectOverview, bool) {
	ov, ok := sh.over[path]
	return ov, ok
}

// noAccess reports a project the overview could not read (deleted, no
// access).
func (sh *shared) noAccess(path string) bool {
	return errors.Is(sh.errs[path], gitlab.ErrNotFound)
}

// reorder sorts the projects by activity; projects without overview go
// last, alphabetically. It runs only after a complete overview, so rows and
// groups do not jump while batches arrive.
func (sh *shared) reorder() {
	refs := sh.scope.Refs()
	order := make([]string, len(refs))
	for i, r := range refs {
		order[i] = r.FullPath
	}
	act := func(p string) time.Time {
		if ov, ok := sh.over[p]; ok {
			return ov.LastActivityAt
		}
		for _, r := range sh.scope.Rows {
			if r.Info != nil && r.Ref.FullPath == p {
				return r.Info.LastActivityAt
			}
		}
		return time.Time{}
	}
	sort.SliceStable(order, func(i, j int) bool {
		ai, aj := act(order[i]), act(order[j])
		switch {
		case ai.IsZero() != aj.IsZero():
			return !ai.IsZero()
		case ai.IsZero():
			return order[i] < order[j]
		}
		return ai.After(aj)
	})
	sh.order = order
}

// rank is the position of a project in the order (unknown last).
func (sh *shared) rank() map[string]int {
	r := make(map[string]int, len(sh.order))
	for i, p := range sh.order {
		r[p] = i
	}
	return r
}

// refsInOrder are the unique projects in display order.
func (sh *shared) refsInOrder() []gitlab.ProjectRef {
	out := make([]gitlab.ProjectRef, 0, len(sh.order))
	for _, p := range sh.order {
		ref := gitlab.ProjectRef{FullPath: p}
		if ov, ok := sh.over[p]; ok {
			ref.ID = ov.ID
		}
		out = append(out, ref)
	}
	return out
}

// projectURL is the project page in the web UI (the rule of the project
// screen, app.Env.ProjectURL).
func (sh *shared) projectURL(path string) string {
	return sh.env.ProjectURL(sh.over[path].WebURL, path)
}

// primaryRepo is the first local folder of project path, nil if none.
func (sh *shared) primaryRepo(path string) *gitremote.Repo {
	if lp := sh.scope.Primary(path); lp != nil {
		return lp.Repo
	}
	return nil
}

// openProject opens the project screen of path on tab with the given
// workspace tab filters.
func (sh *shared) openProject(path string, repo *gitremote.Repo, tab int, mf *gitlab.MRFilter, pf *gitlab.PipelineFilter) tea.Cmd {
	return app.Push(project.New(sh.env, project.Options{
		Path: path, Repo: repo, Tab: tab, MRFilter: mf, PipelineFilter: pf, Crumb: sh.source,
	}))
}

// tab is one tab of the workspace.
type tab interface {
	Update(msg tea.Msg) (tab, tea.Cmd)
	View() string
	SetSize(w, h int)
	KeyMap() common.KeyMap
	CapturesInput() bool
	Activate() tea.Cmd
	Deactivate()
	Refresh() tea.Cmd
	UpdatedAt() time.Time
	Count() int
	Status() string
	// Rebuild redraws after the overview changed.
	Rebuild()
}

type (
	scopeLoadedMsg struct {
		tok   common.Token
		rows  []workspace.Project
		total int
		err   error
	}
	// batchMsg carries one batch of a workspace request; next listens for
	// the following one. done is set once the channel is closed.
	batchMsg[T any] struct {
		tok  common.Token
		res  gitlab.BatchResult[T]
		done bool
		next tea.Cmd
	}
)

// listen turns a batch channel into a chain of commands (SPEC §6.3).
func listen[T any](tok common.Token, ch <-chan gitlab.BatchResult[T]) tea.Cmd {
	return func() tea.Msg {
		res, ok := <-ch
		if !ok {
			return batchMsg[T]{tok: tok, done: true}
		}
		return batchMsg[T]{tok: tok, res: res, next: listen(tok, ch)}
	}
}

// Screen is the workspace screen.
type Screen struct {
	sh     *shared
	gen    common.Gen // scope load (group workspace)
	ovGen  common.Gen // overview run
	poll   common.Gen // overview poll tick
	cancel context.CancelFunc
	force  bool // the running overview was forced (r)

	tabs   []tab
	cur    int
	active bool
	failed bool // the running overview already toasted an error
	gitOn  bool // ahead/behind counted (and fetch_on_start done) once
	w, h   int
}

var _ app.WorkspaceRoot = (*Screen)(nil)

// New creates the workspace of a folder from its scan results.
func New(env *app.Env, source string, scope workspace.Scope) *Screen {
	s := newScreen(env, source)
	s.setScope(scope)
	return s
}

// NewGroup creates the workspace of a GitLab group; its projects are
// loaded in Init.
func NewGroup(env *app.Env, g gitlab.Group) *Screen {
	limit := env.Settings.MaxGroupProjects
	if limit <= 0 {
		limit = app.DefaultMaxGroupProjects
	}
	src := &workspace.GroupSource{Group: g, Limit: limit, Client: env.Client}
	s := newScreen(env, src.Name())
	s.sh.group = src
	return s
}

func newScreen(env *app.Env, source string) *Screen {
	sh := &shared{env: env, source: source, over: map[string]gitlab.ProjectOverview{}, errs: map[string]error{}}
	s := &Screen{sh: sh, gen: common.NewGen(), ovGen: common.NewGen(), poll: common.NewGen()}
	s.tabs = []tab{newProjectsTab(sh), newMRsTab(sh), newPipelinesTab(sh), newPlaceholder(sh, TabTags)}
	return s
}

// setScope installs the scope and draws it from the disk snapshot.
func (s *Screen) setScope(scope workspace.Scope) {
	sh := s.sh
	sh.scope, sh.ready = scope, true
	if d := sh.env.Disk; d != nil {
		if snap, _, ok := d.LoadWorkspace(sh.env.Host, sh.source); ok {
			for _, ov := range snap.Projects {
				sh.over[ov.FullPath] = ov
			}
		}
	}
	sh.reorder()
	s.rebuild()
}

func (s *Screen) projects() *projectsTab { return s.tabs[TabProjects].(*projectsTab) }

func (s *Screen) rebuild() {
	for _, t := range s.tabs {
		t.Rebuild()
	}
}

// Scope returns the workspace scope (for tests).
func (s *Screen) Scope() workspace.Scope { return s.sh.scope }

// Current returns the index of the active tab (for tests).
func (s *Screen) Current() int { return s.cur }

func (s *Screen) Init() tea.Cmd {
	if s.sh.group == nil {
		return nil
	}
	tok := s.gen.Next()
	src := *s.sh.group
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		rows, total, err := src.ProjectsTotal(ctx)
		return scopeLoadedMsg{tok: tok, rows: rows, total: total, err: err}
	}
}

func (s *Screen) Title() string { return s.sh.source }

func (s *Screen) SetSize(w, h int) {
	s.w, s.h = w, h
	for _, t := range s.tabs {
		t.SetSize(w, max(0, h-3))
	}
}

// CapturesInput lets the active tab own the keyboard while a filter is open.
func (s *Screen) CapturesInput() bool { return s.tabs[s.cur].CapturesInput() }

// CatalogEntries lists the workspace projects for the project picker.
func (s *Screen) CatalogEntries() []app.CatalogEntry {
	sh := s.sh
	out := make([]app.CatalogEntry, 0, len(sh.order))
	for _, p := range sh.order {
		e := app.CatalogEntry{Path: p, OpenMRs: -1, Repo: sh.primaryRepo(p)}
		if ov, ok := sh.over[p]; ok {
			e.OpenMRs, e.Activity = ov.OpenMRs, ov.LastActivityAt
		}
		out = append(out, e)
	}
	return out
}

// LocalRepo returns the first local folder of project path.
func (s *Screen) LocalRepo(path string) *gitremote.Repo { return s.sh.primaryRepo(path) }

func (s *Screen) KeyMap() common.KeyMap {
	k := s.sh.env.Msg.Keys
	km := s.tabs[s.cur].KeyMap()
	short := append([]common.Binding{}, km.Short...)
	g := s.sh.env.Msg.Git
	fetch := common.NewBinding([]string{"u"}, "", g.KeyFetch, common.PriorityNormal)
	pull := common.NewBinding([]string{"U"}, "", g.KeyPull, common.PriorityLow)
	fetch.Hidden, pull.Hidden = !s.gitAvailable(), !s.gitAvailable()
	short = append(short, fetch, pull,
		common.NewBinding([]string{"1", "2", "3", "4"}, "1-4", k.Tabs, common.PriorityNormal),
		common.NewBinding([]string{"r"}, "", k.Refresh, common.PriorityNormal),
		common.NewBinding([]string{"q"}, "", k.Quit, common.PriorityLow),
	)
	return common.KeyMap{Short: short, Groups: km.Groups}
}

func (s *Screen) switchTo(i int) tea.Cmd {
	i = (i%len(s.tabs) + len(s.tabs)) % len(s.tabs)
	if i == s.cur {
		return nil
	}
	if s.active {
		s.tabs[s.cur].Deactivate()
	}
	prev := s.cur
	s.cur = i
	if !s.active {
		return nil
	}
	cmds := []tea.Cmd{s.tabs[s.cur].Activate()}
	switch {
	case i == TabProjects:
		cmds = append(cmds, s.overviewOnActivate())
	case prev == TabProjects:
		s.poll.Next() // the overview polls only while its tab is shown
	}
	return tea.Batch(cmds...)
}

// overviewOnActivate refreshes a stale overview or schedules the next poll.
func (s *Screen) overviewOnActivate() tea.Cmd {
	env := s.sh.env
	switch {
	case !s.sh.ready || s.sh.loading || env.AuthLost:
		return nil
	case s.sh.updated.IsZero() || env.Now().Sub(s.sh.updated) >= env.WorkspacePoll():
		return s.loadOverview(false)
	}
	return s.schedulePoll()
}

func (s *Screen) schedulePoll() tea.Cmd {
	env := s.sh.env
	if !s.active || s.cur != TabProjects || env.AuthLost {
		s.poll.Next()
		return nil
	}
	return common.PollAfter(env.WorkspacePoll(), s.poll.Next())
}

// loadOverview starts an overview run over every project of the scope.
func (s *Screen) loadOverview(force bool) tea.Cmd {
	sh := s.sh
	if !sh.ready || len(sh.scope.Refs()) == 0 {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	tok := s.ovGen.Next()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.sh.loading, s.failed, s.force = cancel, true, false, force
	roots := map[string]string{}
	for p, ov := range sh.over {
		if ov.DefaultBranch != "" {
			roots[p] = ov.DefaultBranch
		}
	}
	ch := sh.env.Client.Workspace().ProjectsOverview(ctx, sh.refsInOrder(), gitlab.OverviewOptions{RootRefs: roots, ForceTags: force})
	s.tabs[TabProjects].Rebuild()
	return listen(tok, ch)
}

func (s *Screen) stopOverview() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.sh.loading {
		s.ovGen.Next()
		s.sh.loading = false
	}
	s.poll.Next()
}

func (s *Screen) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	env := s.sh.env
	switch msg := msg.(type) {
	case app.ActivatedMsg:
		s.active = true
		return s, tea.Batch(s.overviewOnActivate(), s.tabs[s.cur].Activate(), s.gitOnStart())
	case app.DeactivatedMsg:
		s.active = false
		s.tabs[s.cur].Deactivate()
		s.stopOverview()
		return s, nil

	case scopeLoadedMsg:
		if !s.gen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			s.sh.err = msg.err
			s.tabs[TabProjects].Rebuild()
			return s, env.Fail(msg.err)
		}
		s.sh.err = nil
		scope := workspace.NewScope(s.sh.source, msg.rows)
		scope.Total = msg.total
		s.setScope(scope)
		if s.active {
			return s, tea.Batch(s.overviewOnActivate(), s.tabs[s.cur].Activate(), s.gitOnStart())
		}
		return s, nil

	case batchMsg[gitlab.ProjectOverview]:
		return s, s.onOverview(msg)

	case startMsg:
		if s.active {
			return s, msg.start()
		}
		return s, nil
	case localgit.ProgressMsg, localgit.DoneMsg, localgit.AheadBehindMsg:
		s.tabs[TabProjects].Rebuild()
		return s, nil

	case common.PollTickMsg:
		if s.poll.Valid(msg.Token) {
			if !s.active || s.cur != TabProjects || env.AuthLost {
				return s, nil
			}
			return s, s.loadOverview(false)
		}
	case app.AuthLostMsg:
		s.poll.Next()
	case app.AuthRestoredMsg:
		var cmd tea.Cmd
		if s.active {
			cmd = s.loadOverview(false)
		}
		return s, tea.Batch(cmd, s.broadcast(msg))

	case tea.KeyPressMsg:
		return s, s.key(msg)
	}
	return s, s.broadcast(msg)
}

// onOverview merges one overview batch; at the end of the run it fixes
// the order, saves the snapshot and schedules the next poll. Only the tabs
// whose data changed are rebuilt: the group tabs read from the overview
// just the order, "no access" and the review count. The MR of the local
// branch is asked only when the selected row or its branch changed.
func (s *Screen) onOverview(msg batchMsg[gitlab.ProjectOverview]) tea.Cmd {
	if !s.ovGen.Valid(msg.tok) {
		return nil
	}
	sh, env := s.sh, s.sh.env
	if msg.done {
		s.sh.loading = false
		s.cancel = nil
		sh.updated = env.Now()
		old := sh.order
		sh.reorder()
		if slices.Equal(old, sh.order) {
			s.projects().Rebuild()
		} else {
			s.rebuild()
		}
		s.saveSnapshot()
		env.Catalog.Note(s.CatalogEntries()...)
		return tea.Batch(s.schedulePoll(), s.projects().loadLocalMR())
	}
	var fail tea.Cmd
	groups := false // the group tabs show something that changed
	for _, g := range msg.res.Groups {
		path := g.Project.FullPath
		noAccess := sh.noAccess(path)
		switch {
		case g.Err != nil:
			sh.errs[path] = g.Err
			if errors.Is(g.Err, gitlab.ErrNotFound) {
				delete(sh.over, path)
			} else if !s.failed {
				s.failed = true
				fail = env.Fail(g.Err)
			}
		case len(g.Items) > 0:
			delete(sh.errs, path)
			old := sh.over[path]
			nw := mergeOverview(old, g.Items[0])
			groups = groups || old.ReviewMRs != nw.ReviewMRs
			sh.over[path] = nw
		}
		groups = groups || noAccess != sh.noAccess(path)
	}
	pt := s.projects()
	sel := pt.selection()
	pt.Rebuild()
	if groups {
		s.tabs[TabMRs].Rebuild()
		s.tabs[TabPipelines].Rebuild()
	}
	var local tea.Cmd
	if pt.selection() != sel {
		local = pt.loadLocalMR()
	}
	return tea.Batch(fail, msg.next, local)
}

// mergeOverview keeps what the old row knew and the new report does not
// carry yet (phase 2 and the tag arrive later than phase 1).
func mergeOverview(old, nw gitlab.ProjectOverview) gitlab.ProjectOverview {
	if nw.Phase < 2 && old.DefaultBranch == nw.DefaultBranch {
		nw.DefaultPipeline = old.DefaultPipeline
	}
	if !nw.TagKnown {
		nw.LastTag = old.LastTag
	}
	return nw
}

func (s *Screen) saveSnapshot() {
	sh := s.sh
	if sh.env.Disk == nil {
		return
	}
	rows := make([]gitlab.ProjectOverview, 0, len(sh.over))
	for _, p := range sh.order {
		if ov, ok := sh.over[p]; ok {
			rows = append(rows, ov)
		}
	}
	if err := sh.env.Disk.SaveWorkspace(sh.env.Host, sh.source, rows); err != nil {
		debuglog.L().Debug("save workspace snapshot", "err", err)
	}
}

func (s *Screen) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(s.tabs))
	for i, t := range s.tabs {
		s.tabs[i], cmds[i] = t.Update(msg)
	}
	return tea.Batch(cmds...)
}

func (s *Screen) key(k tea.KeyPressMsg) tea.Cmd {
	cur := s.tabs[s.cur]
	if !cur.CapturesInput() {
		switch str := k.String(); str {
		case "1", "2", "3", "4":
			return s.switchTo(int(str[0] - '1'))
		case "tab":
			return s.switchTo(s.cur + 1)
		case "shift+tab":
			return s.switchTo(s.cur - 1)
		case "u":
			return s.fetch()
		case "U":
			return s.pull()
		case "r":
			if !s.sh.ready {
				return s.retryScope()
			}
			s.refreshRepos()
			cmds := []tea.Cmd{cur.Refresh()}
			if s.cur == TabProjects {
				cmds = append(cmds, s.loadOverview(true))
			}
			return tea.Batch(cmds...)
		}
	}
	var cmd tea.Cmd
	s.tabs[s.cur], cmd = cur.Update(k)
	return cmd
}

// retryScope loads the group's project list again after it failed (r);
// while the first load is in flight r does nothing.
func (s *Screen) retryScope() tea.Cmd {
	if s.sh.group == nil || s.sh.err == nil {
		return nil
	}
	s.sh.err = nil
	s.tabs[TabProjects].Rebuild()
	return s.Init()
}

// refreshRepos re-reads HEAD of every local folder (a checkout outside).
func (s *Screen) refreshRepos() {
	for _, r := range s.sh.scope.Repos() {
		_ = r.RefreshHEAD()
	}
	s.tabs[TabProjects].Rebuild()
}

func (s *Screen) View() string {
	env := s.sh.env
	st, m := env.Styles, env.Msg
	sh := s.sh
	n := sh.scope.GitLabCount()
	count := m.Plural.GitLabProjects(n)
	if sh.group != nil {
		count = m.Plural.Projects(n)
	}
	ctx := sh.source + " " + st.Muted.Render("· "+m.Header.Workspace+" · "+count)
	switch {
	case sh.err != nil:
		ctx = sh.source + " " + st.Muted.Render("· "+m.Header.Workspace+" ·") + " " + st.Error.Render(m.Project.LoadFailed)
	case sh.scope.Total < 0:
		ctx += " " + st.Muted.Render("· "+m.WS.ShownOfMany(len(sh.scope.Rows)))
	case sh.scope.Total > len(sh.scope.Rows):
		ctx += " " + st.Muted.Render("· "+m.WS.ShownOf(len(sh.scope.Rows), sh.scope.Total))
	}
	updated := s.tabs[s.cur].UpdatedAt()
	header := common.Header(st, ctx, env.HeaderRight(updated), s.w)
	bar := common.Tabs(st, []common.Tab{
		{Key: "1", Label: m.Tabs.Projects, Count: s.tabs[TabProjects].Count()},
		{Key: "2", Label: m.Tabs.MRs, Count: s.tabs[TabMRs].Count()},
		{Key: "3", Label: m.Tabs.Pipelines, Count: s.tabs[TabPipelines].Count()},
		{Key: "4", Label: m.Tabs.Tags, Count: s.tabs[TabTags].Count()},
	}, s.cur)
	return header + "\n" + bar + "\n" + common.Rule(st, s.w) + "\n" + s.tabs[s.cur].View()
}

// StatusHint is shown in the footer when there is no toast.
func (s *Screen) StatusHint() string { return s.tabs[s.cur].Status() }

// PollMsg returns the overview poll tick that is due next (for tests).
func (s *Screen) PollMsg() tea.Msg { return common.PollTickMsg{Token: s.poll.Current()} }

// gitAvailable reports whether u/U work here: git found and local folders
// (a group workspace has none).
func (s *Screen) gitAvailable() bool {
	return s.sh.env.Git.Available() && s.sh.ready && s.sh.group == nil && len(s.sh.scope.Repos()) > 0
}

// gitOnStart counts ahead/behind of every folder once (locally) and, with
// git.fetch_on_start, fetches them in the background.
func (s *Screen) gitOnStart() tea.Cmd {
	if s.gitOn || !s.gitAvailable() {
		return nil
	}
	s.gitOn = true
	env := s.sh.env
	repos := s.sh.scope.Repos()
	if env.Settings.FetchOnStart && !env.Git.Busy() {
		return env.Git.Start(gitexec.OpFetch, repos)
	}
	return env.Git.CountAheadBehind(repos)
}

// fetch runs git fetch in every folder (u).
func (s *Screen) fetch() tea.Cmd {
	env := s.sh.env
	if !s.gitAvailable() {
		return nil
	}
	if env.Git.Busy() {
		return app.Toast(common.ToastInfo, env.Msg.Git.Busy(env.Git.Op().String()), "")
	}
	cmd := env.Git.Start(gitexec.OpFetch, s.sh.scope.Repos())
	s.tabs[TabProjects].Rebuild()
	return cmd
}

// pull asks for confirmation with the repositories that will be skipped,
// then pulls fast-forward only (U).
func (s *Screen) pull() tea.Cmd {
	env := s.sh.env
	if !s.gitAvailable() {
		return nil
	}
	if env.Git.Busy() {
		return app.Toast(common.ToastInfo, env.Msg.Git.Busy(env.Git.Op().String()), "")
	}
	start := func() tea.Cmd {
		cmd := env.Git.Start(gitexec.OpPullFF, s.sh.scope.Repos())
		s.tabs[TabProjects].Rebuild()
		return cmd
	}
	body := localgit.PullBody(env.Msg, s.sh.scope.Repos())
	return app.OpenOverlay(app.NewConfirm(env, env.Msg.Git.PullTitle, body, func() tea.Msg { return startMsg{start: start} }))
}

// startMsg runs a deferred start on the screen's goroutine (the tracker is
// not safe for concurrent use).
type startMsg struct{ start func() tea.Cmd }
