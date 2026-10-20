package project

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/mractions"
	"github.com/4aykovski/glview/internal/ui/screens/mrdetail"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	listTTL        = 30 * time.Second
	requestTimeout = 15 * time.Second
	nearEnd        = 5
	previewMinW    = 100
	previewLines   = 3 // lines inside the preview frame
)

type mrsLoadedMsg struct {
	tok     common.Token
	cursor  string
	refresh bool // merge with the loaded rows instead of replacing them
	page    gitlab.Page[gitlab.MergeRequest]
	err     error
}

// mrsTab is the MR list of the project.
type mrsTab struct {
	sh   *shared
	gen  common.Gen
	poll common.Gen
	tbl  *table.Model
	fl   *filterline.Model

	filter gitlab.MRFilter
	cancel context.CancelFunc
	act    *mractions.Controller

	items    []gitlab.MergeRequest
	firstLen int
	next     string
	total    int

	loaded, loading, moreLoading, moreErr bool
	err                                   error
	updated                               time.Time
	active                                bool
	w, h                                  int
}

func newMRsTab(sh *shared) *mrsTab {
	t := &mrsTab{sh: sh, gen: common.NewGen(), poll: common.NewGen(), total: -1,
		filter: gitlab.MRFilter{State: gitlab.MRStateOpened, Scope: gitlab.MRScopeAll}}
	t.tbl = table.New(sh.env.Styles)
	t.act = mractions.New(sh.env, func() gitlab.ProjectRef { return sh.ref }, func() *gitremote.Repo { return sh.repo })
	t.fl = filterline.New(sh.env.Styles, t.filterKeys())
	t.rebuild()
	return t
}

func (t *mrsTab) filterKeys() []filterline.Key {
	m := t.env().Msg.Project
	return []filterline.Key{
		{Name: "state", Desc: m.KeyState, Values: []string{"opened", "merged", "closed", "all"}},
		{Name: "scope", Desc: m.KeyScope, Values: []string{"all", "created_by_me", "assigned_to_me"}},
		{Name: "reviewer", Desc: m.KeyReviewer, Values: []string{"@me", "any"}, Source: t.sh.membersSource()},
		{Name: "draft", Desc: m.KeyDraft, Values: []string{"any", "yes", "no"}},
		{Name: "labels", Desc: m.KeyLabels, Multi: true, Source: t.sh.labelsSource()},
	}
}

func (t *mrsTab) env() *app.Env { return t.sh.env }

func (t *mrsTab) UpdatedAt() time.Time { return t.updated }

// Count is the number of MRs matching the filter, when known.
func (t *mrsTab) Count() int {
	if !t.loaded {
		return -1
	}
	return t.total
}

func (t *mrsTab) Status() string {
	m := t.env().Msg.Project
	return m.StatusPoll(t.env().PollInterval(t.hasActive()).Round(time.Second).String())
}
func (t *mrsTab) CapturesInput() bool { return t.fl.Focused() }

// Items returns the loaded merge requests (for tests).
func (t *mrsTab) Items() []gitlab.MergeRequest { return t.items }

func (t *mrsTab) SetSize(w, h int) {
	t.w, t.h = w, h
	t.fl.SetWidth(w - 2)
	t.layout()
}

// filterHeight is the lines taken above the table: the filter line, or the
// open input with its suggestion list and error.
func (t *mrsTab) filterHeight() int {
	if t.fl.Focused() {
		return t.fl.Height()
	}
	return 1
}

func (t *mrsTab) previewShown() bool { return t.w >= previewMinW && t.h >= previewLines+2+8 }

func (t *mrsTab) layout() {
	body := t.h - t.filterHeight()
	if t.previewShown() {
		body -= previewLines + 2
	}
	t.tbl.SetSize(t.w, max(1, body))
}

func (t *mrsTab) columns() []table.Column {
	m := t.env().Msg.MR
	return []table.Column{
		{Title: m.ColIID, Min: 5, Priority: 100},
		{Title: m.ColTitle, Min: 20, Flex: true, Priority: 90},
		{Title: m.ColAuthor, Min: 8, Max: 16, Priority: 40},
		{Title: m.ColBranch, Min: 18, Max: 36, Priority: 30},
		{Title: m.ColPipeline, Min: 8, Priority: 80},
		{Title: m.ColAppr, Min: 4, Priority: 70, AlignRight: true},
		{Title: m.ColThr, Min: 3, Priority: 20, AlignRight: true},
		{Title: m.ColUpd, Min: 4, Priority: 60, AlignRight: true},
	}
}

// rebuild recomputes strings that depend on language, theme or data.
func (t *mrsTab) rebuild() {
	env := t.env()
	t.tbl.SetStyles(env.Styles)
	t.fl.SetStyles(env.Styles)
	t.tbl.SetColumns(t.columns())
	t.tbl.SetEmpty(t.emptyText())
	rows := make([]table.Row, len(t.items))
	s, m := env.Styles, env.Msg
	for i, mr := range t.items {
		title := mr.Title
		if mr.Draft && !strings.HasPrefix(title, "Draft:") {
			title = "Draft: " + title
		}
		if mr.HasConflicts {
			title += " " + s.Warning.Render(m.MR.Conflicts)
		}
		rows[i] = table.Row{ID: fmt.Sprint(mr.IID), Cells: []string{
			s.Accent.Render(fmt.Sprintf("!%d", mr.IID)),
			title,
			mr.Author.Username,
			mr.SourceBranch + " → " + mr.TargetBranch,
			render.PipelineCell(s, mr.HeadPipeline),
			render.Approvals(s, mr.Approvals),
			render.Threads(s, mr.Discussions),
			render.Ago(env.Lang, env.Now(), mr.UpdatedAt),
		}}
	}
	t.tbl.SetRows(rows)
	t.setTail()
	t.layout()
}

func (t *mrsTab) setTail() {
	m := t.env().Msg.Project
	switch {
	case t.err != nil:
		t.tbl.SetTail(table.TailError, m.LoadFailed)
	case t.moreErr:
		t.tbl.SetTail(table.TailError, m.LoadFailed)
	case t.loading && len(t.items) == 0:
		t.tbl.SetTail(table.TailLoading, m.Loading)
	case t.moreLoading:
		t.tbl.SetTail(table.TailLoading, m.LoadingMore)
	default:
		t.tbl.SetTail(table.TailNone, "")
	}
}

func (t *mrsTab) emptyText() string {
	m := t.env().Msg.MR
	f := t.filter
	var q []string
	if f.Mine {
		q = append(q, m.QualMine)
	}
	switch f.Scope {
	case gitlab.MRScopeCreatedByMe:
		q = append(q, m.QualAuthorMe)
	case gitlab.MRScopeAssignedToMe:
		q = append(q, m.QualAssigneeMe)
	}
	switch f.Reviewer {
	case "":
	case "@me":
		q = append(q, m.QualReviewerMe)
	default:
		q = append(q, m.QualReviewer(f.Reviewer))
	}
	if f.Draft != nil {
		if *f.Draft {
			q = append(q, m.QualDraft)
		} else {
			q = append(q, m.QualNotDraft)
		}
	}
	if len(f.Labels) > 0 {
		q = append(q, m.QualLabels(strings.Join(f.Labels, ", ")))
	}
	if f.Search != "" {
		q = append(q, m.QualText(f.Search))
	}
	text := m.EmptyBase(string(f.State))
	if len(q) > 0 {
		text += m.EmptyWhere + strings.Join(q, "; ")
	}
	return "\n  " + text + "\n  " + t.env().Styles.Muted.Render(m.EmptyHint)
}

// ---- loading ----

func (t *mrsTab) key(cursor string) string {
	return fmt.Sprintf("mrs:%s:%s:%s", t.sh.ref.FullPath, t.filter.Key(), cursor)
}

func (t *mrsTab) prefix() string {
	return fmt.Sprintf("mrs:%s:%s:", t.sh.ref.FullPath, t.filter.Key())
}

// fetch builds the request command for one page.
func (t *mrsTab) fetch(tok common.Token, cursor string, force, refresh bool) tea.Cmd {
	env := t.env()
	client, ref, f, limit := env.Client, t.sh.ref, t.filter, env.Settings.PageSize
	key := t.key(cursor)
	if t.cancel != nil && cursor == "" {
		t.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	if cursor == "" {
		t.cancel = cancel
	}
	return func() tea.Msg {
		defer cancel()
		page, _, err := common.Fetch(ctx, env.Cache, key, listTTL, force, func(ctx context.Context) (gitlab.Page[gitlab.MergeRequest], error) {
			return client.MergeRequests().List(ctx, ref, f, cursor, limit)
		})
		return mrsLoadedMsg{tok: tok, cursor: cursor, refresh: refresh, page: page, err: err}
	}
}

// loadFirst requests the first page. reset drops the rows (filter change);
// otherwise the result is merged into the loaded rows.
func (t *mrsTab) loadFirst(reset, force bool) tea.Cmd {
	tok := t.gen.Next()
	t.moreLoading, t.moreErr, t.err = false, false, nil
	if reset {
		t.items, t.firstLen, t.next, t.total, t.loaded = nil, 0, "", -1, false
		t.loading = true
		t.rebuild()
	} else {
		t.loading = !t.loaded
		t.setTail()
	}
	return t.fetch(tok, "", force, !reset)
}

func (t *mrsTab) loadMore() tea.Cmd {
	if t.next == "" || t.moreLoading || t.loading {
		return nil
	}
	t.moreLoading, t.moreErr = true, false
	t.setTail()
	return t.fetch(t.gen.Current(), t.next, false, false)
}

func (t *mrsTab) Refresh() tea.Cmd {
	if t.moreErr && len(t.items) > 0 {
		t.moreLoading, t.moreErr = true, false
		t.setTail()
		return t.fetch(t.gen.Current(), t.next, true, false)
	}
	return t.loadFirst(false, true)
}

func (t *mrsTab) hasActive() bool {
	for _, mr := range t.items {
		if mr.HeadPipeline != nil && mr.HeadPipeline.Status.Active() {
			return true
		}
	}
	return false
}

func (t *mrsTab) schedulePoll() tea.Cmd {
	if !t.active || t.env().AuthLost {
		t.poll.Next()
		return nil
	}
	return common.PollAfter(t.env().PollInterval(t.hasActive()), t.poll.Next())
}

func (t *mrsTab) Activate() tea.Cmd {
	t.active = true
	switch {
	case !t.loaded && !t.loading:
		return t.loadFirst(true, false)
	case t.loaded && t.env().Now().Sub(t.updated) >= t.env().PollInterval(t.hasActive()) && !t.env().AuthLost:
		return t.loadFirst(false, true)
	}
	return t.schedulePoll()
}

func (t *mrsTab) Deactivate() {
	t.active = false
	t.act.Stop()
	t.poll.Next()
}

// ---- update ----

func (t *mrsTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	env := t.env()
	if res, ok := t.act.Update(msg, t.active); ok {
		if res.Reload {
			return t, tea.Batch(res.Cmd, t.loadFirst(false, true))
		}
		return t, res.Cmd
	}
	if cmd := t.fl.HandleMsg(msg); cmd != nil {
		t.layout()
		return t, cmd
	}
	switch msg := msg.(type) {
	case filterline.SuggestionsMsg:
		t.layout()
		return t, nil
	case mrsLoadedMsg:
		return t, t.onLoaded(msg)

	case common.PollTickMsg:
		if !t.poll.Valid(msg.Token) || !t.active || env.AuthLost {
			return t, nil
		}
		return t, t.loadFirst(false, true)

	case app.CacheRevalidatedMsg:
		if strings.HasPrefix(msg.Key, t.prefix()) && t.loaded {
			return t, t.loadFirst(false, false)
		}
	case app.AuthLostMsg:
		t.poll.Next()
	case app.AuthRestoredMsg:
		if t.active {
			return t, t.loadFirst(false, true)
		}
	case app.FocusChangedMsg:
		if msg.Focused && t.active && t.loaded && env.Now().Sub(t.updated) >= env.PollInterval(t.hasActive()) {
			return t, t.loadFirst(false, true)
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		t.fl.SetKeys(t.filterKeys())
		t.rebuild()

	case tea.KeyPressMsg:
		return t, t.onKey(msg)
	}
	return t, nil
}

func (t *mrsTab) onLoaded(msg mrsLoadedMsg) tea.Cmd {
	if !t.gen.Valid(msg.tok) {
		return nil
	}
	env := t.env()
	if msg.err != nil {
		if msg.cursor != "" {
			t.moreLoading, t.moreErr = false, true
			t.setTail()
			return env.Fail(msg.err)
		}
		t.loading = false
		if len(t.items) == 0 {
			t.err = msg.err
		}
		t.setTail()
		return tea.Batch(env.Fail(msg.err), t.schedulePoll())
	}
	page := msg.page
	switch {
	case msg.cursor != "":
		t.moreLoading = false
		t.items = appendUnique(t.items, page.Items)
		t.next = page.NextCursor
	case msg.refresh && t.loaded && len(t.items) > t.firstLen:
		tail := t.items[t.firstLen:]
		t.items = appendUnique(append([]gitlab.MergeRequest(nil), page.Items...), tail)
		t.firstLen = len(page.Items)
	default:
		t.items = append([]gitlab.MergeRequest(nil), page.Items...)
		t.firstLen = len(page.Items)
		t.next = page.NextCursor
	}
	if msg.cursor == "" {
		t.total = page.Total
		t.loaded, t.loading = true, false
		t.updated = env.Now()
	}
	t.rebuild()
	cmds := []tea.Cmd{t.schedulePoll()}
	if t.tbl.NearEnd(nearEnd) {
		cmds = append(cmds, t.loadMore())
	}
	return tea.Batch(cmds...)
}

func appendUnique(dst, src []gitlab.MergeRequest) []gitlab.MergeRequest {
	seen := make(map[int64]bool, len(dst))
	for _, m := range dst {
		seen[m.IID] = true
	}
	for _, m := range src {
		if !seen[m.IID] {
			dst = append(dst, m)
		}
	}
	return dst
}

func (t *mrsTab) selected() *gitlab.MergeRequest {
	id := t.tbl.CursorID()
	for i := range t.items {
		if fmt.Sprint(t.items[i].IID) == id {
			return &t.items[i]
		}
	}
	return nil
}

func (t *mrsTab) onKey(k tea.KeyPressMsg) tea.Cmd {
	env := t.env()
	if t.fl.Focused() {
		res, cmd := t.fl.Update(k)
		t.layout()
		if res == filterline.Applied {
			return tea.Batch(cmd, t.applyFilter())
		}
		return cmd
	}
	if t.tbl.HandleKey(k) {
		if t.tbl.NearEnd(nearEnd) {
			return t.loadMore()
		}
		return nil
	}
	switch k.String() {
	case "/":
		t.fl.SetKeys(t.filterKeys())
		t.fl.SetCurrent(t.filterValues())
		cmd := t.fl.Open(t.filterText())
		t.layout()
		return cmd
	case "m":
		t.filter.Mine = !t.filter.Mine
		return t.loadFirst(true, false)
	case "s":
		states := []gitlab.MRState{gitlab.MRStateOpened, gitlab.MRStateMerged, gitlab.MRStateClosed, gitlab.MRStateAll}
		next := 0
		for i, st := range states {
			if st == t.filter.State {
				next = (i + 1) % len(states)
			}
		}
		t.filter.State = states[next]
		return t.loadFirst(true, false)
	case "backspace":
		t.filter = gitlab.MRFilter{State: gitlab.MRStateOpened, Scope: gitlab.MRScopeAll}
		return t.loadFirst(true, false)
	case "a", "M", "b", "C":
		if mr := t.selected(); mr != nil {
			cmd, _ := t.act.Key(k.String(), *mr)
			return cmd
		}
	case "enter":
		if mr := t.selected(); mr != nil {
			return app.Push(mrdetail.New(env, t.sh.ref, t.sh.repo, *mr))
		}
	case "o":
		return app.OpenURL(env, t.listURL())
	case "O":
		if mr := t.selected(); mr != nil && mr.WebURL != "" {
			return app.OpenURL(env, mr.WebURL)
		}
	case "y":
		if mr := t.selected(); mr != nil && mr.WebURL != "" {
			return app.Copy(env, mr.WebURL, render.ShortURL(mr.WebURL))
		}
	}
	return nil
}

// ---- filter ----

func defaultFilter() map[string]string {
	return map[string]string{"state": "opened", "scope": "all", "reviewer": "any", "draft": "any"}
}

// filterValues returns the current key:value set, defaults included.
func (t *mrsTab) filterValues() map[string]string {
	v := defaultFilter()
	f := t.filter
	v["state"] = string(f.State)
	if f.Scope != "" {
		v["scope"] = string(f.Scope)
	}
	if f.Reviewer != "" {
		v["reviewer"] = f.Reviewer
	}
	if f.Draft != nil {
		v["draft"] = map[bool]string{true: "yes", false: "no"}[*f.Draft]
	}
	if len(f.Labels) > 0 {
		v["labels"] = strings.Join(f.Labels, ",")
	}
	return v
}

// filterText is the starting text of the input: the non-default pairs.
func (t *mrsTab) filterText() string {
	vals := t.filterValues()
	def := defaultFilter()
	for k, d := range def {
		if vals[k] == d {
			delete(vals, k)
		}
	}
	return filterline.Format(t.filterKeys(), vals, t.filter.Search)
}

func (t *mrsTab) applyFilter() tea.Cmd {
	vals, free, err := t.fl.Parsed()
	if err != nil {
		return nil
	}
	f := gitlab.MRFilter{State: gitlab.MRStateOpened, Scope: gitlab.MRScopeAll, Search: free, Mine: t.filter.Mine}
	if v := vals["state"]; v != "" {
		f.State = gitlab.MRState(v)
	}
	if v := vals["scope"]; v != "" {
		f.Scope = gitlab.MRScope(v)
	}
	if v := vals["reviewer"]; v != "" && v != "any" {
		f.Reviewer = v
	}
	switch vals["draft"] {
	case "yes":
		y := true
		f.Draft = &y
	case "no":
		n := false
		f.Draft = &n
	}
	if v := vals["labels"]; v != "" {
		f.Labels = strings.Split(v, ",")
	}
	t.filter = f
	return t.loadFirst(true, false)
}

// ---- view ----

func (t *mrsTab) filterLine() string {
	env := t.env()
	s := env.Styles
	if t.fl.Focused() {
		return filterInputView(t.fl, s, env.Msg.Project.FilterHint, t.filterError)
	}
	vals := t.filterValues()
	var pairs [][2]string
	for _, k := range t.filterKeys() {
		v, ok := vals[k.Name]
		if !ok {
			v = "any"
		}
		pairs = append(pairs, [2]string{k.Name, v})
	}
	extra := ""
	if t.filter.Mine {
		extra = s.Accent.Render("me")
	}
	if t.filter.Search != "" {
		extra += " " + s.FilterValue.Render(t.filter.Search)
	}
	right := ""
	if t.loaded {
		right = env.Msg.Project.Counter(len(t.items), t.total, t.filter.Mine && t.next != "")
	}
	return common.FilterBar(s, pairs, strings.TrimSpace(extra), right, t.w)
}

func (t *mrsTab) filterError(err error) string {
	m := t.env().Msg.Project
	var pe *filterline.ParseError
	if asParseError(err, &pe) {
		if pe.Kind == filterline.UnknownKey {
			var names []string
			for _, k := range t.filterKeys() {
				names = append(names, k.Name)
			}
			return m.FilterUnknown(pe.Key, strings.Join(names, ", "))
		}
		return m.FilterBad(pe.Key, pe.Value, strings.Join(pe.Allowed, ", "))
	}
	return err.Error()
}

func (t *mrsTab) View() string {
	parts := []string{t.filterLine(), t.tbl.View()}
	if t.previewShown() {
		parts = append(parts, t.preview())
	}
	return strings.Join(parts, "\n")
}

// preview is the dense three-line summary of the selected MR:
// title; status · approvals · pipeline · diff; first line of the description.
func (t *mrsTab) preview() string {
	env := t.env()
	s, m := env.Styles, env.Msg
	frame := func(lines ...string) string {
		inner := t.w - 4
		for i := range lines {
			lines[i] = " " + render.Clip(lines[i], inner)
		}
		return common.Box(s, strings.Join(lines, "\n"), t.w, previewLines+2, common.BoxNormal)
	}
	mr := t.selected()
	if mr == nil {
		return frame("", "", "")
	}
	dot := s.Muted.Render(" · ")
	line2 := []string{render.MergeChip(s, mr)}
	if lbl := m.MR.StatusLabel(mr.MergeStatus); lbl != "" && mr.State == gitlab.MRStateOpened {
		line2[0] += " " + s.Muted.Render(lbl)
	}
	if len(mr.Approvals.ApprovedBy) > 0 {
		line2 = append(line2, render.Approvals(s, mr.Approvals)+" "+s.Muted.Render("("+render.Names(mr.Approvals.ApprovedBy)+")"))
	} else {
		line2 = append(line2, s.Muted.Render(m.MR.PrevNoApprovals))
	}
	if p := mr.HeadPipeline; p != nil {
		line2 = append(line2, m.MR.PrevPipeline+" "+s.Accent.Render(fmt.Sprintf("#%d", p.ID))+" "+s.StatusGlyph(p.Status)+" "+s.Muted.Render(render.Duration(p.Duration)))
	}
	if d := mr.Diff; d != nil {
		line2 = append(line2, s.DiffAdd.Render(fmt.Sprintf("+%d", d.Additions))+" "+s.DiffDel.Render(fmt.Sprintf("−%d", d.Deletions))+" "+s.Muted.Render(m.Plural.Files(d.Files)))
	}
	desc := render.FirstLine(mr.Description)
	if desc == "" {
		desc = s.Muted.Render(m.MR.PrevNoDesc)
	}
	return frame(
		s.Accent.Render(fmt.Sprintf("!%d", mr.IID))+"  "+s.PreviewTitle.Render(mr.Title),
		strings.Join(line2, dot),
		desc,
	)
}

func (t *mrsTab) KeyMap() common.KeyMap {
	k := t.env().Msg.Keys
	short := []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", k.Open, common.PriorityHigh),
		common.NewBinding([]string{"/"}, "", k.Filter, common.PriorityNormal),
	}
	short = append(short, t.act.Bindings()...)
	short = append(short,
		common.NewBinding([]string{"s"}, "", k.State, common.PriorityNormal),
		common.NewBinding([]string{"backspace"}, "⌫", k.Reset, common.PriorityLow),
		common.NewBinding([]string{"m"}, "", k.Me, common.PriorityNormal),
		common.NewBinding([]string{"o"}, "", k.BrowserList, common.PriorityLow),
		common.NewBinding([]string{"O"}, "", k.BrowserMR, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
	)
	return common.KeyMap{Short: short}
}

// listURL is the project's merge requests page in the web UI with the
// current filters. Defaults are omitted; the "me" filter (m) has no web
// equivalent and is never included. Parameters that need the own login are
// dropped while it is unknown.
func (t *mrsTab) listURL() string {
	me := t.env().User.Username
	set := func(q url.Values, k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	q := url.Values{}
	f := t.filter
	if f.State != "" && f.State != gitlab.MRStateOpened {
		q.Set("state", string(f.State))
	}
	switch f.Scope {
	case gitlab.MRScopeCreatedByMe:
		set(q, "author_username", me)
	case gitlab.MRScopeAssignedToMe:
		set(q, "assignee_username", me)
	}
	if f.Reviewer == "@me" {
		set(q, "reviewer_username", me)
	} else {
		set(q, "reviewer_username", f.Reviewer)
	}
	if f.Draft != nil {
		q.Set("draft", map[bool]string{true: "yes", false: "no"}[*f.Draft])
	}
	for _, l := range f.Labels {
		q.Add("label_name[]", l)
	}
	set(q, "search", f.Search)
	u := t.sh.projectURL() + "/-/merge_requests"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func asParseError(err error, target **filterline.ParseError) bool { return errors.As(err, target) }

// filterInputView renders the open filter input: the input line with a
// hint (or the parse error below the list).
func filterInputView(fl *filterline.Model, s *common.Styles, hint string, errText func(error) string) string {
	lines := strings.Split(fl.View(), "\n")
	if err := fl.Err(); err != nil {
		return strings.Join(lines, "\n") + "\n" + s.Error.Render(errText(err))
	}
	lines[0] += "  " + s.Muted.Render(hint)
	return strings.Join(lines, "\n")
}
