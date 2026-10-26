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
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/mrlinks"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
	"github.com/4aykovski/glview/internal/ui/common/grouped"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/comment"
	"github.com/4aykovski/glview/internal/ui/screens/linkslist"
	"github.com/4aykovski/glview/internal/ui/screens/mractions"
	"github.com/4aykovski/glview/internal/ui/screens/mrdetail"
	"github.com/4aykovski/glview/internal/ui/screens/newmr"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

var mrStates = []gitlab.MRState{gitlab.MRStateOpened, gitlab.MRStateMerged, gitlab.MRStateClosed, gitlab.MRStateAll}

// mrsTab is the MRs tab: the first per_project MRs of every expanded
// project.
type mrsTab struct {
	*groupBase[gitlab.MergeRequest]
	filter gitlab.MRFilter // State, Search, Mine
	act    *mractions.Controller
	actRef gitlab.ProjectRef // project of the selected line (actions)

	linksGen common.Gen // the L or A request in flight
}

func defaultMRFilter() gitlab.MRFilter {
	return gitlab.MRFilter{State: gitlab.MRStateOpened, Scope: gitlab.MRScopeAll}
}

func newMRsTab(sh *shared) *mrsTab {
	t := &mrsTab{filter: defaultMRFilter(), linksGen: common.NewGen()}
	t.groupBase = newGroupBase[gitlab.MergeRequest](sh, "mrs", mrFilterKeys(sh.env))
	t.narrow = t.narrowed
	t.fetch = func(ctx context.Context, ps []gitlab.ProjectRef) <-chan gitlab.BatchResult[gitlab.MergeRequest] {
		return sh.env.Client.Workspace().MergeRequests(ctx, ps, t.filter, perProject(sh.env))
	}
	t.act = mractions.New(sh.env, func() gitlab.ProjectRef { return t.actRef },
		func() *gitremote.Repo { return sh.primaryRepo(t.actRef.FullPath) })
	t.Rebuild()
	return t
}

// perProject is workspace.per_project.
func perProject(env *app.Env) int {
	if n := env.Settings.PerProject; n > 0 {
		return n
	}
	return app.DefaultPerProject
}

// narrowed reports a filter that narrows the list: text, "me" or a state
// other than opened.
func (t *mrsTab) narrowed() bool {
	f := t.filter
	return f.State != gitlab.MRStateOpened || f.Search != "" || f.Mine
}

func (t *mrsTab) filterKeys() []filterline.Key { return mrFilterKeys(t.env()) }

func mrFilterKeys(env *app.Env) []filterline.Key {
	m := env.Msg.Project
	return []filterline.Key{{Name: "state", Desc: m.KeyState, Values: []string{"opened", "merged", "closed", "all"}}}
}

// Count is the number of open MRs from the overview (of the projects
// passing P); in filter mode it is the number of records found.
func (t *mrsTab) Count() int {
	if t.narrowed() {
		return t.count()
	}
	n, known := 0, false
	for _, p := range t.sh.order {
		if ov, ok := t.sh.overview(p); ok && t.passP(p) {
			n, known = n+ov.OpenMRs, true
		}
	}
	if !known {
		return -1
	}
	return n
}

func (t *mrsTab) Status() string { return t.env().Msg.WS.StatusGroups }

func (t *mrsTab) Activate() tea.Cmd { return t.activate() }

func (t *mrsTab) Deactivate() {
	t.deactivate()
	t.act.Stop()
}

func (t *mrsTab) Refresh() tea.Cmd { return t.refresh() }

func (t *mrsTab) Rebuild() {
	env := t.env()
	s, m := env.Styles, env.Msg
	t.tbl.SetStyles(s)
	t.fl.SetStyles(s)
	t.tbl.SetMoreText(m.WS.More)
	t.tbl.SetColumns([]table.Column{
		{Title: m.MR.ColIID, Min: 5, Priority: 100},
		{Title: m.MR.ColTitle, Min: 20, Flex: true, Priority: 90, Linked: true},
		{Title: m.MR.ColAuthor, Min: 8, Max: 16, Priority: 40},
		{Title: m.WS.ColYou, Min: 3, Max: 9, Priority: 30},
		{Title: m.MR.ColPipeline, Min: 8, Max: 12, Priority: 80},
		{Title: m.MR.ColAppr, Min: 4, Priority: 70, AlignRight: true},
		{Title: m.MR.ColUpd, Min: 4, Priority: 60, AlignRight: true},
	})
	t.setGroups(t.groups(t.row, t.summary, t.head, m.WS.EmptyMRs(string(t.filter.State), t.filter.Mine)))
	t.layout()
}

func (t *mrsTab) row(mr gitlab.MergeRequest) table.Row {
	env := t.env()
	s := env.Styles
	title := mr.Title
	if mr.Draft && !strings.HasPrefix(title, "Draft:") {
		title = "Draft: " + title
	}
	pipe := s.Muted.Render("—")
	if mr.HeadPipeline != nil {
		pipe = s.Status(mr.HeadPipeline.Status)
	}
	role := role(mr, env.User.Username)
	if role == "" {
		role = s.Muted.Render("—")
	}
	return table.Row{ID: fmt.Sprint(mr.IID), Cells: []string{
		s.Accent.Render(fmt.Sprintf("!%d", mr.IID)), title, mr.Author.Username, role, pipe,
		render.Approvals(s, mr.Approvals), render.Ago(env.Lang, env.Now(), mr.UpdatedAt),
	}, Links: mrlinks.Collect(mrlinks.Source{Title: mr.Title}, env.Settings.LinkRules)}
}

// role is the current user's role in the MR (the "You" column): author,
// assignee or reviewer. GitLab's own words, not translated.
func role(mr gitlab.MergeRequest, me string) string {
	is := func(u gitlab.User) bool { return u.Username == me }
	switch {
	case me == "":
		return ""
	case mr.Author.Username == me:
		return "author"
	case slices.ContainsFunc(mr.Assignees, is):
		return "assignee"
	case slices.ContainsFunc(mr.Reviewers, is):
		return "reviewer"
	}
	return ""
}

func (t *mrsTab) summary(path string, g gitlab.ProjectGroup[gitlab.MergeRequest]) string {
	m := t.env().Msg.WS
	parts := []string{m.GroupCount(max(g.Total, len(g.Items)), string(t.filter.State), t.filter.Mine)}
	if ov, ok := t.sh.overview(path); ok && ov.ReviewMRs > 0 && t.filter.State == gitlab.MRStateOpened && !t.filter.Mine {
		parts = append(parts, m.GroupWaiting(ov.ReviewMRs))
	}
	if g.Total > len(g.Items) {
		parts = append(parts, m.GroupShown(len(g.Items)))
	}
	return strings.Join(parts, " · ")
}

// head is the header of a collapsed group: the open MRs from the overview.
func (t *mrsTab) head(path string) string {
	m := t.env().Msg.WS
	ov, ok := t.sh.overview(path)
	switch {
	case !ok:
		return ""
	case ov.OpenMRs == 0:
		return m.EmptyMRs(string(gitlab.MRStateOpened), false)
	}
	parts := []string{m.GroupCount(ov.OpenMRs, string(gitlab.MRStateOpened), false)}
	if ov.ReviewMRs > 0 {
		parts = append(parts, m.GroupWaiting(ov.ReviewMRs))
	}
	return strings.Join(parts, " · ")
}

// toggle collapses or expands the group under the cursor.
func (t *mrsTab) toggle() tea.Cmd {
	path, cmd := t.groupBase.toggle()
	t.Rebuild()
	t.onToggled(path)
	return cmd
}

// selected returns the MR under the cursor (nil on headers and "more").
func (t *mrsTab) selected() *gitlab.MergeRequest {
	g, r, kind := t.tbl.Cursor()
	if kind != grouped.KindRow {
		return nil
	}
	items := t.data[g.Key].Items
	for i := range items {
		if fmt.Sprint(items[i].IID) == r.ID {
			return &items[i]
		}
	}
	return nil
}

// syncAct points the action controller at the selected line's project.
func (t *mrsTab) syncAct() {
	if path := t.cursorGroup(); path != "" {
		t.actRef = t.ref(path)
	}
}

func (t *mrsTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	env := t.env()
	if res, ok := t.act.Update(msg, t.active); ok {
		if res.Reload {
			return t, tea.Batch(res.Cmd, t.reload([]gitlab.ProjectRef{t.actRef}))
		}
		return t, res.Cmd
	}
	if cmd := t.fl.HandleMsg(msg); cmd != nil {
		t.layout()
		return t, cmd
	}
	switch msg := msg.(type) {
	case batchMsg[gitlab.MergeRequest]:
		ours, done, full, cmd := t.onBatch(msg)
		if !ours {
			return t, nil
		}
		t.Rebuild()
		if done && full {
			return t, t.schedulePoll()
		}
		return t, cmd
	case expandMsg:
		cmd := t.onExpand(msg)
		t.Rebuild()
		return t, cmd
	case linkslist.LoadedMsg:
		// The answer counts only for the request in flight and while the
		// cursor still stands on its MR.
		if mr := t.selected(); t.linksGen.Valid(msg.Tok) && mr != nil && mr.IID == msg.IID && t.cursorGroup() == msg.Ref.FullPath {
			return t, linkslist.Done(env, msg, *mr)
		}
	case newmr.CreatedMsg:
		t.updated = time.Time{} // the groups reload when the tab is shown again
	case pickedMsg:
		ours, cmd := t.onPicked(msg)
		if ours {
			t.Rebuild()
		}
		return t, cmd
	case common.PollTickMsg:
		if !t.poll.Valid(msg.Token) || !t.active || env.AuthLost {
			return t, nil
		}
		return t, t.refresh()
	case app.AuthLostMsg:
		t.poll.Next()
	case app.AuthRestoredMsg:
		if t.active {
			return t, t.load(false)
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		t.fl.SetKeys(t.filterKeys())
		t.Rebuild()
	case tea.KeyPressMsg:
		return t, t.onKey(msg)
	}
	return t, nil
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
	if t.handleKey(k) {
		return nil
	}
	path := t.cursorGroup()
	switch k.String() {
	case "space":
		return t.toggle()
	case "P":
		return t.openP()
	case "/":
		vals := map[string]string{}
		if t.filter.State != gitlab.MRStateOpened {
			vals["state"] = string(t.filter.State)
		}
		t.fl.SetKeys(t.filterKeys())
		t.fl.SetCurrent(map[string]string{"state": string(t.filter.State)})
		cmd := t.fl.Open(filterline.Format(t.filterKeys(), vals, t.filter.Search))
		t.layout()
		return cmd
	case "s":
		i := slices.Index(mrStates, t.filter.State)
		t.filter.State = mrStates[(i+1)%len(mrStates)]
		return t.refilter()
	case "m":
		t.filter.Mine = !t.filter.Mine
		return t.refilter()
	case "backspace":
		t.filter = defaultMRFilter()
		save := t.resetP()
		return tea.Batch(save, t.refilter())
	case "f":
		if path == "" {
			return nil
		}
		f := t.filter
		return t.sh.openProject(path, t.sh.primaryRepo(path), project.TabMRs, &f, nil, "")
	case "o":
		if path != "" {
			return app.OpenURL(env, t.listURL(path))
		}
	case "n":
		if path != "" && !t.archived(path) {
			ov, _ := t.sh.overview(path)
			return app.Push(newmr.New(env, newmr.Options{Project: t.ref(path), Repo: t.sh.primaryRepo(path), DefaultBranch: ov.DefaultBranch}))
		}
	}
	if _, _, kind := t.tbl.Cursor(); kind == grouped.KindHeader && k.String() == "enter" {
		return t.toggle()
	}
	mr := t.selected()
	if mr == nil {
		return nil
	}
	switch k.String() {
	case "enter":
		return app.Push(mrdetail.New(env, t.ref(path), t.sh.primaryRepo(path), *mr, t.archived(path)))
	case "d":
		return app.Push(mrdetail.NewOnTab(env, t.ref(path), t.sh.primaryRepo(path), *mr, t.archived(path), mrdetail.TabDiff))
	case "c":
		if !t.archived(path) {
			return comment.Open(env, comment.Target{Project: t.ref(path), Kind: comment.KindMR, IID: mr.IID, Title: mr.Title})
		}
	case "L":
		return linkslist.Load(env, t.linksGen.Next(), t.ref(path), mr.IID)
	case "A":
		return linkslist.LoadApprovers(env, t.linksGen.Next(), t.ref(path), mr.IID)
	case "a", "M", "b", "C":
		t.syncAct()
		cmd, _ := t.act.Key(k.String(), *mr)
		return cmd
	case "O":
		if mr.WebURL != "" {
			return app.OpenURL(env, mr.WebURL)
		}
	case "y":
		if mr.WebURL != "" {
			return app.Copy(env, mr.WebURL, render.ShortURL(mr.WebURL))
		}
	}
	return nil
}

func (t *mrsTab) refilter() tea.Cmd {
	t.refilterMode()
	cmd := t.load(true)
	t.Rebuild()
	return cmd
}

func (t *mrsTab) applyFilter() tea.Cmd {
	vals, free, err := t.fl.Parsed()
	if err != nil {
		return nil
	}
	t.filter.State = gitlab.MRStateOpened
	if v := vals["state"]; v != "" {
		t.filter.State = gitlab.MRState(v)
	}
	t.filter.Search = free
	return t.refilter()
}

// listURL is the project's MR list in the web UI with the tab filters
// (the "me" filter has no web equivalent).
func (t *mrsTab) listURL(path string) string {
	q := url.Values{}
	if t.filter.State != gitlab.MRStateOpened {
		q.Set("state", string(t.filter.State))
	}
	if t.filter.Search != "" {
		q.Set("search", t.filter.Search)
	}
	u := t.sh.projectURL(path) + "/-/merge_requests"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (t *mrsTab) View() string {
	env := t.env()
	s := env.Styles
	extra := ""
	if t.filter.Mine {
		extra = s.Accent.Render(env.Msg.WS.MeMRs)
	}
	if t.filter.Search != "" {
		extra = strings.TrimSpace(extra + " " + s.FilterValue.Render(t.filter.Search))
	}
	parts := []string{
		t.filterBar([][2]string{{"state", string(t.filter.State)}}, extra, t.filterErr(t.filterKeys())),
		t.tbl.View(),
	}
	if t.previewShown() {
		mr := t.selected()
		if mr == nil {
			parts = append(parts, t.frame("", "", ""))
		} else {
			parts = append(parts, t.frame(render.MRPreview(s, env.Msg, mr, env.Settings.LinkRules)...))
		}
	}
	return strings.Join(parts, "\n")
}

func (t *mrsTab) KeyMap() common.KeyMap {
	env := t.env()
	k, w := env.Msg.Keys, env.Msg.WS
	t.syncAct()
	short := []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"J", "K"}, "J/K", w.KeyGroups, common.PriorityNormal),
		common.NewBinding([]string{"space"}, "space", w.KeyCollapse, common.PriorityLow),
		common.NewBinding([]string{"enter"}, "", k.Open, common.PriorityHigh),
		common.NewBinding([]string{"d"}, "", env.Msg.Diff.KeyDiff, common.PriorityLow),
		common.NewBinding([]string{"f"}, "", w.KeyProject, common.PriorityHigh),
		common.NewBinding([]string{"m"}, "", k.Me, common.PriorityNormal),
	}
	if path := t.cursorGroup(); path != "" && !t.archived(path) {
		short = append(short, common.NewBinding([]string{"n"}, "", env.Msg.NewMR.KeyNew, common.PriorityLow))
	}
	short = append(short, t.act.Bindings()...)
	if t.selected() != nil { // not on a group header or "N more"
		c := common.NewBinding([]string{"c"}, "", k.Comment, common.PriorityLow)
		c.Hidden = t.archived(t.cursorGroup()) // an archived project takes no writes
		short = append(short, c, common.NewBinding([]string{"L"}, "", env.Msg.Links.Key, common.PriorityLinks),
			common.NewBinding([]string{"A"}, "", env.Msg.Approvers.Key, common.PriorityLinks))
	}
	short = append(short,
		common.NewBinding([]string{"/"}, "", k.Filter, common.PriorityNormal),
		common.NewBinding([]string{"s"}, "", k.State, common.PriorityNormal),
		common.NewBinding([]string{"P"}, "", w.KeyProjects, common.PriorityLow),
		common.NewBinding([]string{"backspace"}, "⌫", k.Reset, common.PriorityLow),
		common.NewBinding([]string{"o"}, "", k.BrowserList, common.PriorityLow),
		common.NewBinding([]string{"O"}, "", k.BrowserMR, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
	)
	return common.KeyMap{Short: short}
}
