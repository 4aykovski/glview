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
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/joblog"
	"github.com/4aykovski/glview/internal/ui/screens/pipeline"
	"github.com/4aykovski/glview/internal/ui/screens/render"
	"github.com/4aykovski/glview/internal/ui/screens/runpipeline"
)

const (
	finalTTL       = 10 * time.Minute // finished pipelines never change
	pipePreviewLns = 2                // lines inside the preview frame
)

type (
	pipesLoadedMsg struct {
		tok     common.Token
		cursor  string
		refresh bool
		page    gitlab.Page[gitlab.PipelineSummary]
		err     error
	}
	stagesLoadedMsg struct {
		tok    common.Token
		id     int64
		stages []gitlab.StageSummary
		err    error
	}
	pipeActionMsg struct {
		retry bool
		id    int64
		err   error
	}
	pipeJobMsg struct {
		pipe gitlab.PipelineSummary
		job  *gitlab.Job
		err  error
	}
)

// pipelinesTab is the pipeline list of the project.
type pipelinesTab struct {
	sh   *shared
	gen  common.Gen
	stG  common.Gen
	poll common.Gen
	tbl  *table.Model
	fl   *filterline.Model

	filter gitlab.PipelineFilter
	cancel context.CancelFunc
	stCan  context.CancelFunc

	items    []gitlab.PipelineSummary
	firstLen int
	next     string
	total    int
	stages   map[int64][]gitlab.StageSummary
	stFor    int64

	loaded, loading, moreLoading, moreErr bool
	err                                   error
	updated                               time.Time
	active                                bool
	w, h                                  int
}

func newPipelinesTab(sh *shared) *pipelinesTab {
	t := &pipelinesTab{sh: sh, gen: common.NewGen(), stG: common.NewGen(), poll: common.NewGen(), total: -1,
		stages: map[int64][]gitlab.StageSummary{}}
	t.tbl = table.New(sh.env.Styles)
	t.fl = filterline.New(sh.env.Styles, t.filterKeys())
	t.rebuild()
	return t
}

func (t *pipelinesTab) env() *app.Env { return t.sh.env }

func (t *pipelinesTab) filterKeys() []filterline.Key {
	m := t.env().Msg.Project
	return []filterline.Key{
		{Name: "ref", Desc: m.KeyRef, Source: t.sh.refsSource()},
		{Name: "status", Desc: m.KeyStatus, Values: []string{"created", "waiting_for_resource", "preparing", "pending", "running", "success", "failed", "canceled", "skipped", "manual", "scheduled"}},
		{Name: "source", Desc: m.KeySource, Values: []string{"push", "web", "trigger", "schedule", "api", "external", "pipeline", "chat", "webide", "merge_request_event", "external_pull_request_event", "parent_pipeline", "ondemand_dast_scan", "ondemand_dast_validation"}},
		{Name: "user", Desc: m.KeyUser, Values: []string{"@me"}, Source: t.sh.membersSource()},
	}
}

func (t *pipelinesTab) UpdatedAt() time.Time { return t.updated }

func (t *pipelinesTab) Count() int {
	if !t.loaded {
		return -1
	}
	return t.total
}

func (t *pipelinesTab) Status() string {
	m := t.env().Msg.Project
	interval := t.env().PollInterval(t.hasActive()).Round(time.Second).String()
	running := 0
	for _, p := range t.items {
		if p.Status.Active() {
			running++
		}
	}
	if running > 0 {
		return m.StatusLive(running, interval)
	}
	return m.StatusPoll(interval)
}
func (t *pipelinesTab) CapturesInput() bool { return t.fl.Focused() }

// Items returns the loaded pipelines (for tests).
func (t *pipelinesTab) Items() []gitlab.PipelineSummary { return t.items }

func (t *pipelinesTab) SetSize(w, h int) {
	t.w, t.h = w, h
	t.fl.SetWidth(w - 2)
	t.layout()
}

func (t *pipelinesTab) filterHeight() int {
	if t.fl.Focused() {
		return t.fl.Height()
	}
	return 1
}

func (t *pipelinesTab) previewShown() bool { return t.w >= previewMinW && t.h >= pipePreviewLns+2+8 }

func (t *pipelinesTab) layout() {
	body := t.h - t.filterHeight()
	if t.previewShown() {
		body -= pipePreviewLns + 2
	}
	t.tbl.SetSize(t.w, max(1, body))
}

func (t *pipelinesTab) rebuild() {
	env := t.env()
	s, m := env.Styles, env.Msg.MR
	t.tbl.SetStyles(env.Styles)
	t.fl.SetStyles(env.Styles)
	t.tbl.SetColumns([]table.Column{
		{Title: m.ColStatus, Min: 10, Max: 14, Priority: 100},
		{Title: m.ColID, Min: 7, Priority: 90},
		{Title: m.ColRef, Min: 12, Max: 28, Priority: 80},
		{Title: m.ColSource, Min: 8, Max: 20, Priority: 30},
		{Title: m.ColCommit, Min: 16, Flex: true, Priority: 70},
		{Title: m.ColUser, Min: 6, Max: 14, Priority: 40},
		{Title: m.ColDur, Min: 5, Priority: 20, AlignRight: true},
		{Title: m.ColAgo, Min: 4, Priority: 60, AlignRight: true},
	})
	rows := make([]table.Row, len(t.items))
	for i, p := range t.items {
		rows[i] = table.Row{ID: fmt.Sprint(p.ID), Cells: []string{
			s.Status(p.Status), s.Accent.Render(fmt.Sprintf("#%d", p.ID)), render.PipelineRef(p.Ref), p.Source, render.PipelineCommit(p), render.PipelineUser(p),
			render.Duration(p.Duration), render.Ago(env.Lang, env.Now(), p.CreatedAt),
		}}
	}
	t.tbl.SetRows(rows)
	t.tbl.SetEmpty(t.emptyText())
	t.setTail()
	t.layout()
}

func (t *pipelinesTab) setTail() {
	m := t.env().Msg.Project
	switch {
	case t.err != nil, t.moreErr:
		t.tbl.SetTail(table.TailError, m.LoadFailed)
	case t.loading && len(t.items) == 0:
		t.tbl.SetTail(table.TailLoading, m.Loading)
	case t.moreLoading:
		t.tbl.SetTail(table.TailLoading, m.LoadingMore)
	default:
		t.tbl.SetTail(table.TailNone, "")
	}
}

func (t *pipelinesTab) emptyText() string {
	m := t.env().Msg.Pipe
	f := t.filter
	var q []string
	if f.Ref != "" {
		q = append(q, m.QualRef(f.Ref))
	}
	if f.Status != "" {
		q = append(q, m.QualStatus(f.Status))
	}
	if f.Source != "" {
		q = append(q, m.QualSource(f.Source))
	}
	if f.Username != "" {
		if f.Username == t.env().User.Username {
			q = append(q, m.QualMine)
		} else {
			q = append(q, m.QualUser(f.Username))
		}
	}
	text := m.EmptyBase
	if len(q) > 0 {
		text += m.EmptyWhere + strings.Join(q, "; ")
	}
	return "\n  " + text + "\n  " + t.env().Styles.Muted.Render(m.EmptyHint)
}

// ---- loading ----

func (t *pipelinesTab) key(cursor string) string {
	return fmt.Sprintf("pipes:%s:%s:%s", t.sh.ref.FullPath, t.filter.Key(), cursor)
}

func (t *pipelinesTab) prefix() string {
	return fmt.Sprintf("pipes:%s:%s:", t.sh.ref.FullPath, t.filter.Key())
}

func (t *pipelinesTab) fetch(tok common.Token, cursor string, force, refresh bool) tea.Cmd {
	env := t.env()
	client, ref, f, limit, key := env.Client, t.sh.ref, t.filter, env.Settings.PageSize, t.key(cursor)
	if t.cancel != nil && cursor == "" {
		t.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	if cursor == "" {
		t.cancel = cancel
	}
	return func() tea.Msg {
		defer cancel()
		page, _, err := common.Fetch(ctx, env.Cache, key, listTTL, force, func(ctx context.Context) (gitlab.Page[gitlab.PipelineSummary], error) {
			return client.Pipelines().List(ctx, ref, f, cursor, limit)
		})
		return pipesLoadedMsg{tok: tok, cursor: cursor, refresh: refresh, page: page, err: err}
	}
}

func (t *pipelinesTab) loadFirst(reset, force bool) tea.Cmd {
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

func (t *pipelinesTab) loadMore() tea.Cmd {
	if t.next == "" || t.moreLoading || t.loading {
		return nil
	}
	t.moreLoading, t.moreErr = true, false
	t.setTail()
	return t.fetch(t.gen.Current(), t.next, false, false)
}

func (t *pipelinesTab) Refresh() tea.Cmd {
	if t.moreErr && len(t.items) > 0 {
		t.moreLoading, t.moreErr = true, false
		t.setTail()
		return t.fetch(t.gen.Current(), t.next, true, false)
	}
	return tea.Batch(t.loadFirst(false, true), t.loadStages(true))
}

// loadStages requests the stage progress of the selected pipeline, dropping
// a request still in flight for the previous selection.
func (t *pipelinesTab) loadStages(force bool) tea.Cmd {
	p := t.selected()
	if p == nil || p.IID == 0 || !t.previewShown() {
		return nil
	}
	if t.stCan != nil {
		t.stCan()
	}
	tok := t.stG.Next()
	t.stFor = p.ID
	env, ref, iid, id := t.env(), t.sh.ref, p.IID, p.ID
	key := fmt.Sprintf("stages:%s:%d", ref.FullPath, iid)
	ttl := time.Duration(0) // running pipelines are not cached
	if p.Status.Final() {
		ttl = finalTTL
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	t.stCan = cancel
	return func() tea.Msg {
		defer cancel()
		st, _, err := common.Fetch(ctx, env.Cache, key, ttl, force, func(ctx context.Context) ([]gitlab.StageSummary, error) {
			return env.Client.Pipelines().Stages(ctx, ref, iid)
		})
		return stagesLoadedMsg{tok: tok, id: id, stages: st, err: err}
	}
}

func (t *pipelinesTab) hasActive() bool {
	for _, p := range t.items {
		if p.Status.Active() {
			return true
		}
	}
	return false
}

func (t *pipelinesTab) schedulePoll() tea.Cmd {
	if !t.active || t.env().AuthLost {
		t.poll.Next()
		return nil
	}
	return common.PollAfter(t.env().PollInterval(t.hasActive()), t.poll.Next())
}

func (t *pipelinesTab) Activate() tea.Cmd {
	t.active = true
	switch {
	case !t.loaded && !t.loading:
		return t.loadFirst(true, false)
	case t.loaded && t.env().Now().Sub(t.updated) >= t.env().PollInterval(t.hasActive()) && !t.env().AuthLost:
		return t.loadFirst(false, true)
	}
	return t.schedulePoll()
}

func (t *pipelinesTab) Deactivate() {
	t.active = false
	t.poll.Next()
}

// ---- update ----

func (t *pipelinesTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	env := t.env()
	if cmd := t.fl.HandleMsg(msg); cmd != nil {
		t.layout()
		return t, cmd
	}
	switch msg := msg.(type) {
	case filterline.SuggestionsMsg:
		t.layout()
		return t, nil
	case pipesLoadedMsg:
		return t, t.onLoaded(msg)
	case stagesLoadedMsg:
		if t.stG.Valid(msg.tok) && msg.err == nil {
			t.stages[msg.id] = msg.stages
		}
	case pipeActionMsg:
		if !t.active {
			return t, nil
		}
		if env.Cache != nil {
			env.Cache.Invalidate("pipes:" + t.sh.ref.FullPath + ":")
			env.Cache.Invalidate("stages:" + t.sh.ref.FullPath + ":")
			env.Cache.Invalidate(fmt.Sprintf("pipe:%s:%d", t.sh.ref.Key(), msg.id))
		}
		if msg.err != nil {
			return t, env.Fail(msg.err)
		}
		toast := env.Msg.Pipe.ToastCanceled(msg.id)
		if msg.retry {
			toast = env.Msg.Pipe.ToastRetried(msg.id)
		}
		return t, tea.Batch(app.Toast(common.ToastSuccess, toast, ""), t.loadFirst(false, true))
	case pipeJobMsg:
		if !t.active {
			return t, nil
		}
		if msg.err != nil {
			return t, env.Fail(msg.err)
		}
		if msg.job == nil {
			return t, app.Toast(common.ToastInfo, env.Msg.Pipe.NoLogJobs, "")
		}
		return t, app.Push(joblog.New(env, t.sh.ref, msg.pipe.ID, *msg.job))

	case common.PollTickMsg:
		if !t.poll.Valid(msg.Token) || !t.active || env.AuthLost {
			return t, nil
		}
		return t, tea.Batch(t.loadFirst(false, true), t.loadStages(true))
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

func (t *pipelinesTab) onLoaded(msg pipesLoadedMsg) tea.Cmd {
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
		t.items = appendUniquePipes(t.items, page.Items)
		t.next = page.NextCursor
	case msg.refresh && t.loaded && len(t.items) > t.firstLen:
		tail := t.items[t.firstLen:]
		t.items = appendUniquePipes(append([]gitlab.PipelineSummary(nil), page.Items...), tail)
		t.firstLen = len(page.Items)
	default:
		t.items = append([]gitlab.PipelineSummary(nil), page.Items...)
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
	if msg.cursor == "" {
		cmds = append(cmds, t.loadStages(false))
	}
	if t.tbl.NearEnd(nearEnd) {
		cmds = append(cmds, t.loadMore())
	}
	return tea.Batch(cmds...)
}

func appendUniquePipes(dst, src []gitlab.PipelineSummary) []gitlab.PipelineSummary {
	seen := make(map[int64]bool, len(dst))
	for _, p := range dst {
		seen[p.ID] = true
	}
	for _, p := range src {
		if !seen[p.ID] {
			dst = append(dst, p)
		}
	}
	return dst
}

func (t *pipelinesTab) selected() *gitlab.PipelineSummary {
	id := t.tbl.CursorID()
	for i := range t.items {
		if fmt.Sprint(t.items[i].ID) == id {
			return &t.items[i]
		}
	}
	return nil
}

func (t *pipelinesTab) onKey(k tea.KeyPressMsg) tea.Cmd {
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
		cmds := []tea.Cmd{t.loadStages(false)}
		if t.tbl.NearEnd(nearEnd) {
			cmds = append(cmds, t.loadMore())
		}
		return tea.Batch(cmds...)
	}
	p := t.selected()
	switch k.String() {
	case "/":
		t.fl.SetKeys(t.filterKeys())
		t.fl.SetCurrent(t.filterValues())
		cmd := t.fl.Open(t.filterText())
		t.layout()
		return cmd
	case "m":
		if t.filter.Username == env.User.Username {
			t.filter.Username = ""
		} else {
			t.filter.Username = env.User.Username
		}
		return t.loadFirst(true, false)
	case "o":
		return app.OpenURL(env, t.listURL())
	case "s":
		cycle := []string{"", "running", "failed", "success", "manual"}
		next := 0
		for i, st := range cycle {
			if st == t.filter.Status {
				next = (i + 1) % len(cycle)
			}
		}
		t.filter.Status = cycle[next]
		return t.loadFirst(true, false)
	case "backspace":
		t.filter = gitlab.PipelineFilter{}
		return t.loadFirst(true, false)
	case "n":
		return t.runForm()
	}
	if p == nil {
		return nil
	}
	switch k.String() {
	case "O":
		if p.WebURL != "" {
			return app.OpenURL(env, p.WebURL)
		}
	case "y":
		if p.WebURL != "" {
			return app.Copy(env, p.WebURL, render.ShortURL(p.WebURL))
		}
	case "enter":
		return app.Push(pipeline.New(env, t.sh.ref, p.ID, p))
	case "R":
		m := env.Msg.Pipe
		return app.OpenOverlay(app.NewConfirm(env, m.RetryTitle(p.ID), m.RetryBody, t.mutate(*p, true)))
	case "c":
		m := env.Msg.Pipe
		return app.OpenOverlay(app.NewConfirm(env, m.CancelTitle(p.ID), m.CancelBody, t.mutate(*p, false)))
	case "l":
		return t.findLogJob(*p)
	}
	return nil
}

func (t *pipelinesTab) mutate(p gitlab.PipelineSummary, retry bool) tea.Cmd {
	client, ref := t.env().Client, t.sh.ref
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		var err error
		if retry {
			err = client.Pipelines().Retry(ctx, ref, p.ID)
		} else {
			err = client.Pipelines().Cancel(ctx, ref, p.ID)
		}
		return pipeActionMsg{retry: retry, id: p.ID, err: err}
	}
}

// findLogJob looks for the first failed job of the pipeline, else the first
// running one; the job comes with its earlier attempts for the log. The
// pipeline is always read anew (the list and its status may be behind) and
// a final one is cached under the key of the Pipeline screen.
func (t *pipelinesTab) findLogJob(p gitlab.PipelineSummary) tea.Cmd {
	env, ref := t.env(), t.sh.ref
	key := fmt.Sprintf("pipe:%s:%d", ref.Key(), p.ID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		pl, err := env.Client.Pipelines().Get(ctx, ref, p.ID)
		if err != nil {
			return pipeJobMsg{pipe: p, err: err}
		}
		if env.Cache != nil && pl.Status.Final() {
			env.Cache.Set(key, pl)
		}
		return pipeJobMsg{pipe: pl.PipelineSummary, job: pickLogJob(pl.Jobs)}
	}
}

func pickLogJob(jobs []gitlab.Job) *gitlab.Job {
	for _, want := range []gitlab.PipelineStatus{gitlab.StatusFailed, gitlab.StatusRunning} {
		for i := range jobs {
			if jobs[i].Status == want {
				return &jobs[i]
			}
		}
	}
	return nil
}

// ---- filter ----

func (t *pipelinesTab) filterValues() map[string]string {
	v := map[string]string{}
	f := t.filter
	for k, val := range map[string]string{"ref": f.Ref, "status": f.Status, "source": f.Source, "user": f.Username} {
		if val != "" {
			v[k] = val
		}
	}
	return v
}

func (t *pipelinesTab) filterText() string {
	return filterline.Format(t.filterKeys(), t.filterValues(), "")
}

func (t *pipelinesTab) applyFilter() tea.Cmd {
	vals, free, err := t.fl.Parsed()
	if err != nil {
		return nil
	}
	if free != "" { // pipelines have no free-text search: treat it as a ref
		if vals["ref"] == "" {
			vals["ref"] = free
		}
	}
	user := vals["user"]
	if user == "@me" {
		user = t.env().User.Username
	}
	t.filter = gitlab.PipelineFilter{Ref: vals["ref"], Status: vals["status"], Source: vals["source"], Username: user}
	return t.loadFirst(true, false)
}

// ---- view ----

func (t *pipelinesTab) filterLine() string {
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
	right := ""
	if t.loaded {
		right = env.Msg.Project.Counter(len(t.items), t.total, false)
	}
	return common.FilterBar(s, pairs, "", right, t.w)
}

func (t *pipelinesTab) filterError(err error) string {
	m := t.env().Msg.Project
	var pe *filterline.ParseError
	if errors.As(err, &pe) {
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

func (t *pipelinesTab) View() string {
	parts := []string{t.filterLine(), t.tbl.View()}
	if t.previewShown() {
		parts = append(parts, t.preview())
	}
	return strings.Join(parts, "\n")
}

// preview shows "#id ref · status duration" and the stage chain.
func (t *pipelinesTab) preview() string {
	env := t.env()
	s, m := env.Styles, env.Msg
	frame := func(lines ...string) string {
		for i := range lines {
			lines[i] = " " + render.Clip(lines[i], t.w-4)
		}
		return common.Box(s, strings.Join(lines, "\n"), t.w, pipePreviewLns+2, common.BoxNormal)
	}
	p := t.selected()
	if p == nil {
		return frame("", "")
	}
	head := s.PreviewTitle.Render(fmt.Sprintf("#%d", p.ID)) + " " + render.PipelineRef(p.Ref) + s.Muted.Render(" · ") + s.Status(p.Status)
	if p.Duration > 0 {
		head += " " + s.Muted.Render(render.Duration(p.Duration))
	}
	chain := s.Muted.Render(m.Pipe.PrevStagesLoading)
	if st, ok := t.stages[p.ID]; ok {
		if len(st) == 0 {
			chain = s.Muted.Render(m.Pipe.PrevNoStages)
		} else {
			chain = render.StageChain(s, st)
		}
	}
	return frame(head, chain)
}

func (t *pipelinesTab) KeyMap() common.KeyMap {
	k := t.env().Msg.Keys
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", k.Open, common.PriorityHigh),
		common.NewBinding([]string{"/"}, "", k.Filter, common.PriorityNormal),
		common.NewBinding([]string{"l"}, "", k.Log, common.PriorityNormal),
		common.NewBinding([]string{"n"}, "", k.NewPipeline, common.PriorityNormal),
		common.NewBinding([]string{"R"}, "", k.Retry, common.PriorityNormal),
		common.NewBinding([]string{"c"}, "", k.Cancel, common.PriorityNormal),
		common.NewBinding([]string{"s"}, "", k.Status, common.PriorityNormal),
		common.NewBinding([]string{"backspace"}, "⌫", k.Reset, common.PriorityLow),
		common.NewBinding([]string{"m"}, "", k.Me, common.PriorityNormal),
		common.NewBinding([]string{"o"}, "", k.BrowserList, common.PriorityLow),
		common.NewBinding([]string{"O"}, "", k.BrowserOne, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
	}}
}

// listURL is the project's pipelines page in the web UI with the current
// filters (ref, status, source, username).
func (t *pipelinesTab) listURL() string {
	base := t.sh.webURL
	if base == "" {
		base = strings.TrimRight(t.env().Host, "/") + "/" + t.sh.ref.FullPath
	}
	q := url.Values{}
	f := t.filter
	for k, v := range map[string]string{"ref": f.Ref, "status": f.Status, "source": f.Source, "username": f.Username} {
		if v != "" {
			q.Set(k, v)
		}
	}
	u := strings.TrimRight(base, "/") + "/-/pipelines"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// runForm opens the "run pipeline" overlay with the local branch (or the
// default branch) preselected.
func (t *pipelinesTab) runForm() tea.Cmd {
	local, initial := "", t.sh.defaultBranch
	if r := t.sh.repo; r != nil && !r.Detached && r.Branch != "" {
		local, initial = r.Branch, r.Branch
	}
	f := runpipeline.New(t.env(), t.sh.ref, initial, local)
	return tea.Sequence(app.OpenOverlay(f), f.Init())
}
