// Package mrdetail is the merge request screen: title block, status chips
// and the Overview / Discussions / Pipelines tabs.
package mrdetail

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/mractions"
	"github.com/4aykovski/glview/internal/ui/screens/pipeline"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	dataTTL        = 30 * time.Second
	requestTimeout = 15 * time.Second
	wideW          = 100 // the side column moves below the text under this width
	sideW          = 34
	tabOverview    = 0
	tabDiscussions = 1
	tabPipelines   = 2
)

type (
	mrLoadedMsg struct {
		tok common.Token
		mr  gitlab.MergeRequest
		err error
	}
	discLoadedMsg struct {
		tok   common.Token
		discs []gitlab.Discussion
		err   error
	}
	recheckedMsg struct {
		tok    common.Token
		status string
		err    error
	}
	pipesLoadedMsg struct {
		tok   common.Token
		pipes []gitlab.PipelineSummary
		err   error
	}
)

// Screen is the MR detail screen.
type Screen struct {
	env *app.Env
	ref gitlab.ProjectRef
	iid int64
	mr  gitlab.MergeRequest

	discs       []gitlab.Discussion
	discLoaded  bool
	pipes       []gitlab.PipelineSummary
	pipesLoaded bool

	mrGen, discGen, pipeGen, poll common.Gen
	recheckGen                    common.Gen

	// Merge status recheck: GitLab computes detailed_merge_status lazily, so
	// an "unchecked" MR is nudged once per iid@sha and polled fast for a while.
	rechecked map[string]bool
	recheckAt time.Time
	tbl       *table.Model
	act       *mractions.Controller

	tab     int
	scroll  int
	updated time.Time
	active  bool
	w, h    int

	md map[string]string // rendered markdown by width|lang|text

	// Discussions tab: the current thread and the folds the user toggled
	// (overriding the default rule); both are keyed by discussion ID so
	// they survive reloads.
	curThread string
	fold      map[string]bool
}

// New opens MR iid of project ref; initial (from a list row) is shown
// until the details arrive.
func New(env *app.Env, ref gitlab.ProjectRef, repo *gitremote.Repo, initial gitlab.MergeRequest) *Screen {
	s := &Screen{
		env: env, ref: ref, iid: initial.IID, mr: initial,
		mrGen: common.NewGen(), discGen: common.NewGen(), pipeGen: common.NewGen(), poll: common.NewGen(), recheckGen: common.NewGen(), rechecked: map[string]bool{},
		tbl: table.New(env.Styles), md: map[string]string{}, fold: map[string]bool{},
	}
	s.act = mractions.New(env, func() gitlab.ProjectRef { return s.ref }, func() *gitremote.Repo { return repo })
	s.rebuildTable()
	return s
}

func (s *Screen) Init() tea.Cmd { return nil }

func (s *Screen) Title() string { return fmt.Sprintf("%s !%d", s.ref.FullPath, s.iid) }

func (s *Screen) SetSize(w, h int) {
	s.w, s.h = w, h
	s.tbl.SetSize(w, max(1, h-s.headerLines()))
}

// MR returns the merge request as currently known (for tests and actions).
func (s *Screen) MR() gitlab.MergeRequest { return s.mr }

func (s *Screen) KeyMap() common.KeyMap {
	k := s.env.Msg.Keys
	short := []common.Binding{
		common.NewBinding([]string{"tab"}, "tab", k.NextTab, common.PriorityHigh),
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", k.Open, common.PriorityNormal),
	}
	if s.tab == tabDiscussions {
		short = append(short,
			common.NewBinding([]string{"J", "K"}, "J/K", k.Threads, common.PriorityNormal),
			common.NewBinding([]string{"space"}, "space", k.Fold, common.PriorityNormal),
		)
	}
	short = append(short, s.act.Bindings()...)
	short = append(short,
		common.NewBinding([]string{"r"}, "", k.Refresh, common.PriorityNormal),
		common.NewBinding([]string{"o"}, "", k.Browser, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
		common.NewBinding([]string{"esc"}, "", k.Back, common.PriorityLow),
	)
	return common.KeyMap{Short: short}
}

// ---- loading ----

func (s *Screen) key(kind string) string { return fmt.Sprintf("%s:%s:%d", kind, s.ref.FullPath, s.iid) }

func (s *Screen) loadMR(force bool) tea.Cmd {
	tok := s.mrGen.Next()
	env, ref, iid, key := s.env, s.ref, s.iid, s.key("mr")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		mr, _, err := common.Fetch(ctx, env.Cache, key, dataTTL, force, func(ctx context.Context) (gitlab.MergeRequest, error) {
			return env.Client.MergeRequests().Get(ctx, ref, iid)
		})
		return mrLoadedMsg{tok: tok, mr: mr, err: err}
	}
}

func (s *Screen) loadDiscussions(force bool) tea.Cmd {
	tok := s.discGen.Next()
	env, ref, iid, key := s.env, s.ref, s.iid, s.key("mrdisc")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		d, _, err := common.Fetch(ctx, env.Cache, key, dataTTL, force, func(ctx context.Context) ([]gitlab.Discussion, error) {
			return env.Client.MergeRequests().Discussions(ctx, ref, iid)
		})
		return discLoadedMsg{tok: tok, discs: d, err: err}
	}
}

func (s *Screen) loadPipelines(force bool) tea.Cmd {
	tok := s.pipeGen.Next()
	env, ref, iid, key := s.env, s.ref, s.iid, s.key("mrpipes")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		p, _, err := common.Fetch(ctx, env.Cache, key, dataTTL, force, func(ctx context.Context) ([]gitlab.PipelineSummary, error) {
			return env.Client.MergeRequests().Pipelines(ctx, ref, iid)
		})
		return pipesLoadedMsg{tok: tok, pipes: p, err: err}
	}
}

func (s *Screen) loadAll(force bool) tea.Cmd {
	return tea.Batch(s.loadMR(force), s.loadDiscussions(force), s.loadPipelines(force))
}

// recheckWindow is how long after a recheck request the MR is polled fast
// while its status is still being computed.
const recheckWindow = 60 * time.Second

// statusPending reports whether GitLab is still computing the merge status.
func statusPending(status string) bool {
	return status == "unchecked" || status == "checking" || status == "preparing"
}

// pollDelay is 3s while the pipeline runs, a rebase is in progress or the
// merge status is being rechecked (up to a minute), else 30s.
func (s *Screen) pollDelay() time.Duration {
	active := s.mr.RebaseInProgress || (s.mr.HeadPipeline != nil && s.mr.HeadPipeline.Status.Active())
	if !s.recheckAt.IsZero() && statusPending(s.mr.MergeStatus) && s.env.Now().Sub(s.recheckAt) < recheckWindow {
		active = true
	}
	return s.env.PollInterval(active)
}

// maybeRecheck asks GitLab to recompute the status of an open "unchecked" MR,
// once per iid@sha. Failures only go to the debug log.
func (s *Screen) maybeRecheck() tea.Cmd {
	if s.mr.State != gitlab.MRStateOpened || s.mr.MergeStatus != "unchecked" {
		return nil
	}
	key := fmt.Sprintf("%d@%s", s.iid, s.mr.SHA)
	if s.rechecked[key] {
		return nil
	}
	s.rechecked[key] = true
	s.recheckAt = s.env.Now()
	tok := s.recheckGen.Next()
	env, ref, iid := s.env, s.ref, s.iid
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		st, err := env.Client.MergeRequests().RecheckMergeStatus(ctx, ref, iid)
		return recheckedMsg{tok: tok, status: st, err: err}
	}
}

func (s *Screen) schedulePoll() tea.Cmd {
	if !s.active || s.env.AuthLost {
		s.poll.Next()
		return nil
	}
	return common.PollAfter(s.pollDelay(), s.poll.Next())
}

// ---- update ----

func (s *Screen) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	env := s.env
	if res, ok := s.act.Update(msg, s.active); ok {
		if res.Started {
			s.mr.RebaseInProgress = true
		}
		if res.Outcome != mractions.RebaseNone {
			s.mr.RebaseInProgress = false
		}
		if res.Reload {
			return s, tea.Batch(res.Cmd, s.loadMR(true))
		}
		return s, res.Cmd
	}
	switch msg := msg.(type) {
	case app.ActivatedMsg:
		s.active = true
		if s.updated.IsZero() {
			return s, s.loadAll(false)
		}
		if env.Now().Sub(s.updated) >= s.pollDelay() && !env.AuthLost {
			return s, s.loadMR(true)
		}
		return s, s.schedulePoll()
	case app.DeactivatedMsg:
		s.active = false
		s.poll.Next()
		s.act.Stop()

	case mrLoadedMsg:
		if !s.mrGen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			return s, tea.Batch(env.Fail(msg.err), s.schedulePoll())
		}
		s.mr = msg.mr
		s.updated = env.Now()
		recheck := s.maybeRecheck()
		return s, tea.Batch(recheck, s.schedulePoll())
	case recheckedMsg:
		if !s.recheckGen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			debuglog.L().Debug("merge status recheck failed", "project", s.ref.FullPath, "iid", s.iid, "error", msg.err)
			return s, nil
		}
		if s.mr.MergeStatus == "unchecked" && msg.status != "" {
			s.mr.MergeStatus = msg.status
		}
		return s, nil
	case discLoadedMsg:
		if !s.discGen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			return s, env.Fail(msg.err)
		}
		s.discs, s.discLoaded = msg.discs, true
	case pipesLoadedMsg:
		if !s.pipeGen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			return s, env.Fail(msg.err)
		}
		s.pipes, s.pipesLoaded = msg.pipes, true
		s.rebuildTable()

	case common.PollTickMsg:
		if !s.poll.Valid(msg.Token) || !s.active || env.AuthLost {
			return s, nil
		}
		return s, s.loadMR(true)
	case app.CacheRevalidatedMsg:
		switch {
		case msg.Key == s.key("mr"):
			return s, s.loadMR(false)
		case msg.Key == s.key("mrdisc"):
			return s, s.loadDiscussions(false)
		case msg.Key == s.key("mrpipes"):
			return s, s.loadPipelines(false)
		}
	case app.AuthLostMsg:
		s.poll.Next()
	case app.AuthRestoredMsg:
		if s.active {
			return s, s.loadAll(true)
		}
	case app.FocusChangedMsg:
		if msg.Focused && s.active && !s.updated.IsZero() && env.Now().Sub(s.updated) >= s.pollDelay() {
			return s, s.loadMR(true)
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		s.md = map[string]string{}
		s.tbl.SetStyles(env.Styles)
		s.rebuildTable()

	case tea.KeyPressMsg:
		return s, s.onKey(msg)
	}
	return s, nil
}

func (s *Screen) onKey(k tea.KeyPressMsg) tea.Cmd {
	if cmd, ok := s.act.Key(k.String(), s.mr); ok {
		return cmd
	}
	switch k.String() {
	case "tab":
		s.setTab(s.tab + 1)
		return nil
	case "shift+tab":
		s.setTab(s.tab + 2)
		return nil
	case "r":
		return s.loadAll(true)
	case "o":
		return app.OpenURL(s.env, s.link())
	case "y":
		return app.Copy(s.env, s.link(), render.ShortURL(s.link()))
	}
	if s.tab == tabPipelines {
		if k.String() == "enter" {
			if p := s.selectedPipeline(); p != nil {
				return app.Push(pipeline.New(s.env, s.ref, p.ID, p))
			}
			return nil
		}
		s.tbl.HandleKey(k)
		return nil
	}
	if s.tab == tabDiscussions {
		s.threadKey(k.String())
		return nil
	}
	s.scrollKey(k.String())
	return nil
}

func (s *Screen) selectedPipeline() *gitlab.PipelineSummary {
	id := s.tbl.CursorID()
	for i := range s.pipes {
		if fmt.Sprint(s.pipes[i].ID) == id {
			return &s.pipes[i]
		}
	}
	return nil
}

func (s *Screen) setTab(i int) {
	s.tab = i % 3
	s.scroll = 0
	s.curThread = ""
}

func (s *Screen) scrollKey(k string) {
	page := max(1, s.bodyHeight()-1)
	switch k {
	case "j", "down":
		s.scroll++
	case "k", "up":
		s.scroll--
	case "pgdown", "ctrl+d", "space": // space pages everywhere but on Discussions (threadKey)
		s.scroll += page
	case "pgup", "ctrl+u":
		s.scroll -= page
	case "g", "home":
		s.scroll = 0
	case "G", "end":
		s.scroll = 1 << 20
	}
}

func (s *Screen) rebuildTable() {
	env := s.env
	m := env.Msg.MR
	st := env.Styles
	s.tbl.SetStyles(st)
	s.tbl.SetColumns([]table.Column{
		{Title: m.ColStatus, Min: 10, Max: 14, Priority: 100},
		{Title: m.ColID, Min: 7, Priority: 90},
		{Title: m.ColRef, Min: 12, Max: 28, Priority: 80},
		{Title: m.ColSource, Min: 8, Max: 20, Priority: 30},
		{Title: m.ColCommit, Min: 16, Flex: true, Priority: 70},
		{Title: m.ColUser, Min: 6, Max: 14, Priority: 40},
		{Title: m.ColDur, Min: 5, Priority: 20, AlignRight: true},
		{Title: m.ColAgo, Min: 4, Priority: 60, AlignRight: true},
	})
	rows := make([]table.Row, len(s.pipes))
	for i, p := range s.pipes {
		rows[i] = table.Row{ID: fmt.Sprint(p.ID), Cells: []string{
			st.Status(p.Status), st.Accent.Render(fmt.Sprintf("#%d", p.ID)), render.PipelineRef(p.Ref), p.Source, render.PipelineCommit(p), render.PipelineUser(p),
			render.Duration(p.Duration), render.Ago(env.Lang, env.Now(), p.CreatedAt),
		}}
	}
	s.tbl.SetRows(rows)
	s.tbl.SetEmpty("\n  " + m.EmptyPipelines)
	if !s.pipesLoaded {
		s.tbl.SetTail(table.TailLoading, m.Loading)
	} else {
		s.tbl.SetTail(table.TailNone, "")
	}
}

// ---- view ----

// boxedChips reports whether chips get a frame (three lines); on short
// terminals they are plain one-line text to leave room for the body.
func (s *Screen) boxedChips() bool { return s.h >= 30 }

// headerLines is the number of lines above the body: header, title, meta,
// chips, tab bar and its rule.
func (s *Screen) headerLines() int {
	if s.boxedChips() {
		return 8
	}
	return 6
}

func (s *Screen) bodyHeight() int { return max(1, s.h-s.headerLines()) }

func (s *Screen) View() string {
	env := s.env
	st, m := env.Styles, env.Msg
	mr := s.mr

	crumbs := st.Muted.Render(s.ref.FullPath+" › "+m.MR.Crumb+" › ") + st.Accent.Render(fmt.Sprintf("!%d", s.iid))
	head := common.Header(st, crumbs, env.HeaderRight(s.updated), s.w)

	title := mr.Title
	if mr.Draft && !strings.HasPrefix(title, "Draft:") {
		title = "Draft: " + title
	}
	meta := []string{mr.Author.Username, mr.SourceBranch + " → " + mr.TargetBranch}
	switch {
	case mr.State == gitlab.MRStateMerged && !mr.MergedAt.IsZero():
		by := ""
		if mr.MergedBy != nil {
			by = mr.MergedBy.Username
		}
		meta = append(meta, m.MR.Merged(by, render.Ago(env.Lang, env.Now(), mr.MergedAt)))
	case mr.State == gitlab.MRStateClosed && !mr.ClosedAt.IsZero():
		meta = append(meta, m.MR.Closed(render.Ago(env.Lang, env.Now(), mr.ClosedAt)))
	case !mr.CreatedAt.IsZero():
		meta = append(meta, m.MR.Opened(render.Ago(env.Lang, env.Now(), mr.CreatedAt)))
	}
	metaLine := st.Muted.Render(strings.Join(meta, " · "))
	if d := mr.Diff; d != nil {
		metaLine += st.Muted.Render(" · ") + st.DiffAdd.Render(fmt.Sprintf("+%d", d.Additions)) + " " + st.DiffDel.Render(fmt.Sprintf("−%d", d.Deletions)) + " " + st.Muted.Render(m.Plural.Files(d.Files))
	}

	lines := []string{head, render.Clip(st.Bold.Render(title), s.w), render.Clip(metaLine, s.w)}
	if s.boxedChips() {
		lines = append(lines, s.chipRow())
	} else {
		lines = append(lines, render.Clip(s.chips(), s.w))
	}
	lines = append(lines, s.tabBar(), common.Rule(st, s.w))

	var body string
	switch s.tab {
	case tabOverview:
		body = s.window(s.overview())
	case tabDiscussions:
		body = s.window(s.discussionsView())
	default:
		body = s.tbl.View()
	}
	return strings.Join(lines, "\n") + "\n" + body
}

// StatusHint is the footer status: how often the MR is refreshed.
func (s *Screen) StatusHint() string {
	return s.env.Msg.Project.StatusPoll(s.pollDelay().Round(time.Second).String())
}

type chipSpec struct {
	text string
	role common.ChipRole
}

func mergeRole(role render.MergeRole) common.ChipRole {
	switch role {
	case render.RoleOK:
		return common.ChipSuccess
	case render.RoleRunning:
		return common.ChipRunning
	case render.RoleWarn:
		return common.ChipWarn
	case render.RoleBad:
		return common.ChipError
	}
	return common.ChipNeutral
}

func pipelineRole(p gitlab.PipelineStatus) common.ChipRole {
	switch {
	case p == gitlab.StatusSuccess:
		return common.ChipSuccess
	case p == gitlab.StatusFailed:
		return common.ChipError
	case p.Active():
		return common.ChipRunning
	}
	return common.ChipNeutral
}

// chipSpecs lists the status chips: merge status (with its caption on wide
// terminals), approvals, pipeline, unresolved threads and a running rebase.
func (s *Screen) chipSpecs() []chipSpec {
	m := s.env.Msg
	mr := s.mr
	var out []chipSpec
	status, statusRole := render.MRStatusChip(&mr)
	if lbl := m.MR.StatusLabel(mr.MergeStatus); lbl != "" && s.w >= 110 && mr.State == gitlab.MRStateOpened {
		status += " · " + lbl
	}
	out = append(out, chipSpec{status, mergeRole(statusRole)})
	have := len(mr.Approvals.ApprovedBy)
	role := common.ChipNeutral
	glyph := "○"
	if have > 0 {
		glyph = "✓"
	}
	if mr.Approvals.Required > 0 && have >= mr.Approvals.Required {
		role = common.ChipSuccess
	}
	out = append(out, chipSpec{glyph + " " + m.MR.ChipApprovals(have, mr.Approvals.Required), role})
	if p := mr.HeadPipeline; p != nil {
		text := common.Glyph(p.Status) + " pipeline"
		if p.ID != 0 {
			text += fmt.Sprintf(" #%d", p.ID)
		}
		if p.Status.Final() && p.Duration > 0 {
			text += " · " + render.Duration(p.Duration)
		} else {
			text += " · " + string(p.Status)
		}
		out = append(out, chipSpec{text, pipelineRole(p.Status)})
	}
	if mr.RebaseInProgress {
		out = append(out, chipSpec{m.MR.RebaseChip, common.ChipRunning})
	}
	n := max(mr.Discussions.Unresolved, 0)
	trole := common.ChipNeutral
	if n > 0 {
		trole = common.ChipWarn
	}
	out = append(out, chipSpec{m.MR.ChipThreads(n), trole})
	return out
}

func (s *Screen) chipRow() string {
	var chips []string
	for _, c := range s.chipSpecs() {
		chips = append(chips, common.Chip(s.env.Styles, c.text, c.role))
	}
	row := common.ChipRow(chips...)
	// Clip every line to the terminal width.
	ls := strings.Split(row, "\n")
	for i := range ls {
		ls[i] = render.Clip(ls[i], s.w)
	}
	return strings.Join(ls, "\n")
}

// chips is the one-line variant of the chip row for short terminals.
func (s *Screen) chips() string {
	st := s.env.Styles
	var parts []string
	for _, c := range s.chipSpecs() {
		style := st.Muted
		switch c.role {
		case common.ChipSuccess:
			style = st.Success
		case common.ChipWarn:
			style = st.Warning
		case common.ChipError:
			style = st.Error
		case common.ChipRunning:
			style = st.StatusStyle(gitlab.StatusRunning)
		}
		parts = append(parts, style.Render(c.text))
	}
	return strings.Join(parts, "  ")
}

func (s *Screen) tabBar() string {
	st, m := s.env.Styles, s.env.Msg.MR
	discs := ""
	if s.discLoaded {
		discs = fmt.Sprintf(" %d", len(userDiscussions(s.discs)))
	}
	pipes := ""
	if s.pipesLoaded {
		pipes = fmt.Sprintf(" %d", len(s.pipes))
	}
	names := []string{m.TabOverview, m.TabDiscussions + discs, m.TabPipelines + pipes}
	parts := make([]string, len(names))
	for i, n := range names {
		if i == s.tab {
			parts[i] = st.TabActive.Render(n)
		} else {
			parts[i] = st.Tab.Render(n)
		}
	}
	return strings.Join(parts, " ")
}

// window cuts the visible part of lines at the scroll offset.
func (s *Screen) window(lines []string) string {
	h := s.bodyHeight()
	s.scroll = max(0, min(s.scroll, len(lines)-h))
	end := min(len(lines), s.scroll+h)
	return strings.Join(lines[s.scroll:end], "\n")
}

func (s *Screen) markdown(text string, width int) []string {
	key := fmt.Sprintf("%d|%s|%s", width, s.env.Lang, text)
	out, ok := s.md[key]
	if !ok {
		out = common.Markdown(s.env.Styles, text, width)
		s.md[key] = out
	}
	return strings.Split(out, "\n")
}

func (s *Screen) overview() []string {
	m := s.env.Msg.MR
	st := s.env.Styles
	wide := s.w >= wideW
	textW := s.w - 2
	if wide {
		textW = s.w - sideW - 3
	}
	var left []string
	if strings.TrimSpace(s.mr.Description) == "" {
		left = []string{st.Muted.Render(m.None)}
	} else {
		left = s.markdown(s.mr.Description, textW)
	}
	right := s.side()
	if !wide {
		return append(append(left, ""), right...)
	}
	l := lipgloss.NewStyle().Width(textW).Render(strings.Join(left, "\n"))
	r := lipgloss.NewStyle().Width(sideW).Render(strings.Join(right, "\n"))
	rule := common.VRule(st, max(s.bodyHeight(), lipgloss.Height(l), lipgloss.Height(r)))
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, " ", l, " ", rule, " ", r), "\n")
}

func (s *Screen) side() []string {
	st, m := s.env.Styles, s.env.Msg.MR
	mr := s.mr
	head := func(t string) string { return st.Muted.Render(t) }
	out := []string{head(m.SecApprovals)}
	if len(mr.Approvals.ApprovedBy) == 0 {
		out = append(out, st.Muted.Render(m.None))
	}
	for _, u := range mr.Approvals.ApprovedBy {
		out = append(out, st.Success.Render("✓ ")+u.Username)
	}
	if mr.Approvals.Left > 0 {
		out = append(out, st.Muted.Render(m.NeedApprovals(mr.Approvals.Left)))
	}
	out = append(out, "", head(m.SecLabels))
	if len(mr.Labels) == 0 {
		out = append(out, st.Muted.Render(m.None))
	} else {
		out = append(out, strings.Join(mr.Labels, ", "))
	}
	if p := mr.HeadPipeline; p == nil {
		out = append(out, "", head(m.SecPipeline), st.Muted.Render(m.None))
	} else {
		out = append(out, "", head(m.SecPipeline+fmt.Sprintf(" #%d", p.ID)))
		for _, stg := range p.Stages {
			name := st.StatusGlyph(stg.Status) + " " + stg.Name
			if stg.Status.Active() && stg.Total > 0 {
				name += fmt.Sprintf(" %d/%d", stg.Done, stg.Total)
			}
			dur := ""
			switch {
			case stg.Duration > 0:
				dur = render.Duration(stg.Duration)
			case stg.Status == gitlab.StatusManual:
				dur = "manual"
			}
			gap := max(1, sideW-ansi.StringWidth(name)-len(dur))
			out = append(out, name+strings.Repeat(" ", gap)+st.Muted.Render(dur))
		}
	}
	out = append(out, "", head(m.SecLastThread))
	if d := lastThread(s.discs); d == nil {
		out = append(out, st.Muted.Render(m.None))
	} else {
		n := d.Notes[len(d.Notes)-1]
		out = append(out, st.Bold.Render(n.Author.Username), render.FirstLine(n.Body))
	}
	return out
}

// userDiscussions drops discussions that only hold system notes.
func userDiscussions(ds []gitlab.Discussion) []gitlab.Discussion {
	var out []gitlab.Discussion
	for _, d := range ds {
		var notes []gitlab.Note
		for _, n := range d.Notes {
			if !n.System {
				notes = append(notes, n)
			}
		}
		if len(notes) > 0 {
			d.Notes = notes
			out = append(out, d)
		}
	}
	return out
}

func lastThread(ds []gitlab.Discussion) *gitlab.Discussion {
	us := userDiscussions(ds)
	if len(us) == 0 {
		return nil
	}
	return &us[len(us)-1]
}
