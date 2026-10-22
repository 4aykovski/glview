package workspace

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
	"github.com/4aykovski/glview/internal/ui/common/grouped"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/pipeactions"
	"github.com/4aykovski/glview/internal/ui/screens/pipeline"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const finalTTL = 10 * time.Minute // finished pipelines never change

var pipeStatuses = []string{"", "running", "failed", "success", "manual"}

type stagesMsg struct {
	tok    common.Token
	id     int64
	stages []gitlab.StageSummary
	err    error
}

// pipelinesTab is the Pipelines tab: the first per_project pipelines of
// every expanded project. While some are running, those projects are asked
// again every polling.active.
type pipelinesTab struct {
	*groupBase[gitlab.PipelineSummary]
	filter gitlab.PipelineFilter // Status, Ref, Username
	fast   common.Gen            // fast tick of running projects

	stG    common.Gen
	stCan  context.CancelFunc
	stages map[int64][]gitlab.StageSummary
}

func pipeFilterKeys(env *app.Env) []filterline.Key {
	m := env.Msg.Project
	return []filterline.Key{
		{Name: "status", Desc: m.KeyStatus, Values: []string{"created", "waiting_for_resource", "preparing", "pending", "running", "success", "failed", "canceled", "skipped", "manual", "scheduled"}},
		{Name: "ref", Desc: m.KeyRef},
	}
}

func newPipelinesTab(sh *shared) *pipelinesTab {
	t := &pipelinesTab{fast: common.NewGen(), stG: common.NewGen(), stages: map[int64][]gitlab.StageSummary{}}
	t.groupBase = newGroupBase[gitlab.PipelineSummary](sh, "pipelines", pipeFilterKeys(sh.env))
	t.narrow = t.narrowed
	t.fetch = func(ctx context.Context, ps []gitlab.ProjectRef) <-chan gitlab.BatchResult[gitlab.PipelineSummary] {
		return sh.env.Client.Workspace().Pipelines(ctx, ps, t.filter, perProject(sh.env))
	}
	t.Rebuild()
	return t
}

// Count is the number of pipelines found with a filter; collapsed groups
// have no count.
func (t *pipelinesTab) Count() int {
	if !t.narrowed() {
		return -1
	}
	return t.count()
}

// narrowed reports a filter that narrows the list: status, ref or "me".
func (t *pipelinesTab) narrowed() bool {
	f := t.filter
	return f.Status != "" || f.Ref != "" || f.Username != ""
}

// running lists the projects with a running or pending pipeline.
func (t *pipelinesTab) running() []gitlab.ProjectRef {
	var out []gitlab.ProjectRef
	for _, path := range t.sh.order {
		if slices.ContainsFunc(t.data[path].Items, func(p gitlab.PipelineSummary) bool { return p.Status.Active() }) {
			out = append(out, t.ref(path))
		}
	}
	return out
}

func (t *pipelinesTab) Status() string {
	env := t.env()
	n := 0
	for _, g := range t.data {
		for _, p := range g.Items {
			if p.Status.Active() {
				n++
			}
		}
	}
	if n > 0 {
		return env.Msg.Project.StatusLive(n, env.PollInterval(true).Round(time.Second).String())
	}
	return env.Msg.WS.StatusGroups
}

func (t *pipelinesTab) Activate() tea.Cmd { return tea.Batch(t.activate(), t.scheduleFast()) }

func (t *pipelinesTab) Deactivate() {
	t.deactivate()
	t.fast.Next()
	if t.stCan != nil {
		t.stCan()
	}
	t.stG.Next()
}

func (t *pipelinesTab) Refresh() tea.Cmd {
	t.fast.Next()
	return tea.Batch(t.refresh(), t.loadStages(true))
}

// scheduleFast plans the next fast tick while something is running.
func (t *pipelinesTab) scheduleFast() tea.Cmd {
	env := t.env()
	if !t.active || env.AuthLost || len(t.running()) == 0 {
		t.fast.Next()
		return nil
	}
	return common.PollAfter(env.PollInterval(true), t.fast.Next())
}

// FastMsg returns the fast tick that is due next (for tests).
func (t *pipelinesTab) FastMsg() tea.Msg { return common.PollTickMsg{Token: t.fast.Current()} }

func (t *pipelinesTab) Rebuild() {
	env := t.env()
	s, m := env.Styles, env.Msg
	t.tbl.SetStyles(s)
	t.fl.SetStyles(s)
	t.tbl.SetMoreText(m.WS.More)
	t.tbl.SetColumns([]table.Column{
		{Title: m.MR.ColStatus, Min: 10, Max: 14, Priority: 100},
		{Title: m.MR.ColID, Min: 7, Priority: 90},
		{Title: m.MR.ColRef, Min: 12, Max: 28, Priority: 80},
		{Title: m.MR.ColSource, Min: 8, Max: 20, Priority: 30},
		{Title: m.MR.ColCommit, Min: 16, Flex: true, Priority: 70},
		{Title: m.MR.ColDur, Min: 5, Priority: 20, AlignRight: true},
		{Title: m.MR.ColAgo, Min: 4, Priority: 60, AlignRight: true},
	})
	t.setGroups(t.groups(t.row, t.summary, t.head, m.WS.EmptyPipes(t.filter.Username != "")))
	t.layout()
}

func (t *pipelinesTab) row(p gitlab.PipelineSummary) table.Row {
	env := t.env()
	s := env.Styles
	return table.Row{ID: fmt.Sprint(p.ID), Cells: []string{
		s.Status(p.Status), s.Accent.Render(fmt.Sprintf("#%d", p.ID)), render.PipelineRef(p.Ref), p.Source,
		render.PipelineCommit(p), render.Duration(p.Duration), render.Ago(env.Lang, env.Now(), p.CreatedAt),
	}}
}

func (t *pipelinesTab) summary(_ string, g gitlab.ProjectGroup[gitlab.PipelineSummary]) string {
	return t.env().Msg.WS.GroupShown(len(g.Items))
}

// head is the header of a collapsed group: the default branch pipeline
// from the overview ("✓ main #91790 · 2h").
func (t *pipelinesTab) head(path string) string {
	env := t.env()
	s := env.Styles
	ov, ok := t.sh.overview(path)
	switch {
	case !ok:
		return ""
	case ov.DefaultPipeline != nil:
		p := ov.DefaultPipeline
		ref := ov.DefaultBranch
		if ref == "" {
			ref = p.Ref
		}
		text := s.StatusGlyph(p.Status) + " " + ref + " " + fmt.Sprintf("#%d", p.ID)
		if !p.CreatedAt.IsZero() {
			text += " · " + render.Ago(env.Lang, env.Now(), p.CreatedAt)
		}
		return text
	case ov.Phase >= 2:
		return env.Msg.WS.EmptyPipes(false)
	}
	return ""
}

// toggle collapses or expands the group under the cursor.
func (t *pipelinesTab) toggle() tea.Cmd {
	path, cmd := t.groupBase.toggle()
	t.Rebuild()
	t.onToggled(path)
	return tea.Batch(cmd, t.scheduleFast())
}

func (t *pipelinesTab) selected() *gitlab.PipelineSummary {
	g, r, kind := t.tbl.Cursor()
	if kind != grouped.KindRow {
		return nil
	}
	items := t.data[g.Key].Items
	for i := range items {
		if fmt.Sprint(items[i].ID) == r.ID {
			return &items[i]
		}
	}
	return nil
}

// loadStages asks the stage progress of the selected pipeline (preview).
func (t *pipelinesTab) loadStages(force bool) tea.Cmd {
	p := t.selected()
	if p == nil || p.IID == 0 || !t.previewShown() || !t.active {
		return nil
	}
	if _, ok := t.stages[p.ID]; ok && !force && p.Status.Final() {
		return nil
	}
	if t.stCan != nil {
		t.stCan()
	}
	tok := t.stG.Next()
	env, ref, iid, id := t.env(), t.ref(p.Project.FullPath), p.IID, p.ID
	key := fmt.Sprintf("stages:%s:%d", ref.FullPath, iid)
	ttl := time.Duration(0)
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
		return stagesMsg{tok: tok, id: id, stages: st, err: err}
	}
}

func (t *pipelinesTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	env := t.env()
	if cmd := t.fl.HandleMsg(msg); cmd != nil {
		t.layout()
		return t, cmd
	}
	switch msg := msg.(type) {
	case batchMsg[gitlab.PipelineSummary]:
		ours, done, full, cmd := t.onBatch(msg)
		if !ours {
			return t, nil
		}
		t.Rebuild()
		if done {
			cmds := []tea.Cmd{t.scheduleFast(), t.loadStages(false)}
			if full {
				cmds = append(cmds, t.schedulePoll())
			}
			return t, tea.Batch(cmds...)
		}
		return t, cmd
	case expandMsg:
		cmd := t.onExpand(msg)
		t.Rebuild()
		return t, cmd
	case pickedMsg:
		ours, cmd := t.onPicked(msg)
		if !ours {
			return t, nil
		}
		t.Rebuild()
		return t, tea.Batch(cmd, t.loadStages(false))
	case stagesMsg:
		if t.stG.Valid(msg.tok) && msg.err == nil {
			t.stages[msg.id] = msg.stages
		}
	case common.PollTickMsg:
		switch {
		case !t.active || env.AuthLost:
		case t.poll.Valid(msg.Token):
			t.fast.Next()
			return t, t.refresh()
		case t.fast.Valid(msg.Token):
			return t, t.reload(t.running())
		}
	case pipeactions.DoneMsg:
		if !t.active {
			return t, nil
		}
		cmd := pipeactions.Done(env, msg)
		if msg.Err != nil {
			return t, cmd
		}
		return t, tea.Batch(cmd, t.reload([]gitlab.ProjectRef{msg.Ref}))
	case pipeactions.LogJobMsg:
		if !t.active {
			return t, nil
		}
		return t, pipeactions.OpenLog(env, msg)
	case app.AuthLostMsg:
		t.poll.Next()
		t.fast.Next()
	case app.AuthRestoredMsg:
		if t.active {
			return t, t.load(false)
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		t.fl.SetKeys(pipeFilterKeys(env))
		t.Rebuild()
	case tea.KeyPressMsg:
		return t, t.onKey(msg)
	}
	return t, nil
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
	if t.handleKey(k) {
		return t.loadStages(false)
	}
	path := t.cursorGroup()
	switch k.String() {
	case "space":
		return t.toggle()
	case "P":
		return t.openP()
	case "/":
		vals := map[string]string{}
		if t.filter.Status != "" {
			vals["status"] = t.filter.Status
		}
		if t.filter.Ref != "" {
			vals["ref"] = t.filter.Ref
		}
		t.fl.SetKeys(pipeFilterKeys(env))
		t.fl.SetCurrent(vals)
		cmd := t.fl.Open(filterline.Format(pipeFilterKeys(env), vals, ""))
		t.layout()
		return cmd
	case "s":
		i := slices.Index(pipeStatuses, t.filter.Status)
		t.filter.Status = pipeStatuses[(i+1)%len(pipeStatuses)]
		return t.refilter()
	case "m":
		if t.filter.Username != "" {
			t.filter.Username = ""
		} else {
			t.filter.Username = env.User.Username
		}
		return t.refilter()
	case "backspace":
		t.filter = gitlab.PipelineFilter{}
		save := t.resetP()
		return tea.Batch(save, t.refilter())
	case "f":
		if path == "" {
			return nil
		}
		f := t.filter
		return t.sh.openProject(path, t.sh.primaryRepo(path), project.TabPipelines, nil, &f)
	case "o":
		if path != "" {
			return app.OpenURL(env, t.listURL(path))
		}
	case "n":
		if path != "" {
			ov, _ := t.sh.overview(path)
			return pipeactions.RunForm(env, t.ref(path), t.sh.primaryRepo(path), ov.DefaultBranch)
		}
	}
	if _, _, kind := t.tbl.Cursor(); kind == grouped.KindHeader && k.String() == "enter" {
		return t.toggle()
	}
	p := t.selected()
	if p == nil {
		return nil
	}
	ref := t.ref(path)
	switch k.String() {
	case "enter":
		return app.Push(pipeline.New(env, ref, p.ID, p))
	case "l":
		return pipeactions.FindLogJob(env, ref, *p)
	case "R":
		return pipeactions.Retry(env, ref, *p)
	case "c":
		return pipeactions.Cancel(env, ref, *p)
	case "O":
		if p.WebURL != "" {
			return app.OpenURL(env, p.WebURL)
		}
	case "y":
		if p.WebURL != "" {
			return app.Copy(env, p.WebURL, render.ShortURL(p.WebURL))
		}
	}
	return nil
}

func (t *pipelinesTab) refilter() tea.Cmd {
	t.fast.Next()
	t.refilterMode()
	cmd := t.load(true)
	t.Rebuild()
	return cmd
}

func (t *pipelinesTab) applyFilter() tea.Cmd {
	vals, free, err := t.fl.Parsed()
	if err != nil {
		return nil
	}
	ref := vals["ref"]
	if ref == "" {
		ref = free // pipelines have no text search: a bare word is a ref
	}
	t.filter.Status, t.filter.Ref = vals["status"], ref
	return t.refilter()
}

// listURL is the project's pipelines page with the tab filters.
func (t *pipelinesTab) listURL(path string) string {
	q := url.Values{}
	for k, v := range map[string]string{"status": t.filter.Status, "ref": t.filter.Ref, "username": t.filter.Username} {
		if v != "" {
			q.Set(k, v)
		}
	}
	u := t.sh.projectURL(path) + "/-/pipelines"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (t *pipelinesTab) View() string {
	env := t.env()
	s := env.Styles
	status, ref := t.filter.Status, t.filter.Ref
	if status == "" {
		status = "any"
	}
	if ref == "" {
		ref = "any"
	}
	extra := ""
	if t.filter.Username != "" {
		extra = s.Accent.Render(env.Msg.WS.MePipes)
	}
	parts := []string{
		t.filterBar([][2]string{{"status", status}, {"ref", ref}}, extra, t.filterErr(pipeFilterKeys(env))),
		t.tbl.View(),
	}
	if t.previewShown() {
		parts = append(parts, t.preview())
	}
	return strings.Join(parts, "\n")
}

// preview: "#id  project · ref · status duration" and the stage chain.
func (t *pipelinesTab) preview() string {
	env := t.env()
	s, m := env.Styles, env.Msg
	p := t.selected()
	if p == nil {
		return t.frame("", "", "")
	}
	head := s.PreviewTitle.Render(fmt.Sprintf("#%d", p.ID)) + "  " + p.Project.FullPath + s.Muted.Render(" · ") +
		render.PipelineRef(p.Ref) + s.Muted.Render("  · ") + s.Status(p.Status)
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
	return t.frame(head, chain, render.PipelineCommit(*p))
}

func (t *pipelinesTab) KeyMap() common.KeyMap {
	env := t.env()
	k, w := env.Msg.Keys, env.Msg.WS
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"J", "K"}, "J/K", w.KeyGroups, common.PriorityNormal),
		common.NewBinding([]string{"space"}, "space", w.KeyCollapse, common.PriorityLow),
		common.NewBinding([]string{"enter"}, "", k.Open, common.PriorityHigh),
		common.NewBinding([]string{"f"}, "", w.KeyProject, common.PriorityHigh),
		common.NewBinding([]string{"m"}, "", k.Me, common.PriorityNormal),
		common.NewBinding([]string{"l"}, "", k.Log, common.PriorityNormal),
		common.NewBinding([]string{"n"}, "", k.NewPipeline, common.PriorityNormal),
		common.NewBinding([]string{"R"}, "", k.Retry, common.PriorityNormal),
		common.NewBinding([]string{"c"}, "", k.Cancel, common.PriorityNormal),
		common.NewBinding([]string{"/"}, "", k.Filter, common.PriorityNormal),
		common.NewBinding([]string{"s"}, "", k.Status, common.PriorityNormal),
		common.NewBinding([]string{"P"}, "", w.KeyProjects, common.PriorityLow),
		common.NewBinding([]string{"backspace"}, "⌫", k.Reset, common.PriorityLow),
		common.NewBinding([]string{"o"}, "", k.BrowserList, common.PriorityLow),
		common.NewBinding([]string{"O"}, "", k.BrowserOne, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
	}}
}
