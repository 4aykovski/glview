package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitexec"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/localgit"
	"github.com/4aykovski/glview/internal/ui/screens/render"
	"github.com/4aykovski/glview/internal/workspace"
)

const (
	previewMinW  = 100
	projPrevLns  = 4 // lines inside the preview frame of the Projects tab
	localMRTTL   = 30 * time.Second
	minTableRows = 8
)

type localMRMsg struct {
	tok common.Token
	key string
	mr  *gitlab.MergeRequest
	err error
}

// projectsTab is the overview of every folder (or group project).
type projectsTab struct {
	sh  *shared
	tbl *table.Model
	fl  *filterline.Model

	text string // local text filter
	rows []workspace.Project

	lmGen   common.Gen
	localMR map[string]*gitlab.MergeRequest // by path@branch; nil value: no open MR
	lmFor   string                          // key of the request in flight
	lmForce bool                            // the next request bypasses the cache (r)
	lmCan   context.CancelFunc

	moved  bool // the user moved the cursor: keep it on its row
	active bool
	w, h   int
}

func newProjectsTab(sh *shared) *projectsTab {
	t := &projectsTab{sh: sh, lmGen: common.NewGen(), localMR: map[string]*gitlab.MergeRequest{}}
	t.tbl = table.New(sh.env.Styles)
	t.fl = filterline.New(sh.env.Styles, nil)
	return t
}

func (t *projectsTab) env() *app.Env { return t.sh.env }

func (t *projectsTab) UpdatedAt() time.Time { return t.sh.updated }
func (t *projectsTab) CapturesInput() bool  { return t.fl.Focused() }

// Count is the number of GitLab projects.
func (t *projectsTab) Count() int {
	if !t.sh.ready {
		return -1
	}
	return t.sh.scope.GitLabCount()
}

func (t *projectsTab) Status() string {
	return t.env().Msg.Project.StatusPoll(t.env().WorkspacePoll().Round(time.Second).String())
}

func (t *projectsTab) SetSize(w, h int) {
	t.w, t.h = w, h
	t.fl.SetWidth(w - 2)
	t.layout()
}

func (t *projectsTab) previewShown() bool {
	return t.w >= previewMinW && t.h >= projPrevLns+2+minTableRows+t.extraLines()
}

// empty reports a folder workspace without GitLab projects.
func (t *projectsTab) empty() bool {
	return t.sh.ready && t.sh.group == nil && t.sh.scope.GitLabCount() == 0
}

// extraLines is the text around the table: the empty state above it and
// the "skipped without .git" line below.
func (t *projectsTab) extraLines() int {
	n := 0
	if t.empty() {
		n += 4
	}
	if len(t.sh.scope.Skipped) > 0 {
		n++
	}
	return n
}

func (t *projectsTab) filterHeight() int {
	if t.fl.Focused() {
		return t.fl.Height()
	}
	return 1
}

func (t *projectsTab) layout() {
	body := t.h - t.filterHeight() - t.extraLines()
	if t.previewShown() {
		body -= projPrevLns + 2
	}
	t.tbl.SetSize(t.w, max(1, body))
}

func (t *projectsTab) Activate() tea.Cmd {
	t.active = true
	return t.loadLocalMR()
}

func (t *projectsTab) Deactivate() {
	t.active = false
	if t.lmCan != nil {
		t.lmCan()
	}
	t.lmGen.Next()
	t.lmFor = ""
}

func (t *projectsTab) Refresh() tea.Cmd {
	t.localMR = map[string]*gitlab.MergeRequest{}
	t.lmFor, t.lmForce = "", true
	return t.loadLocalMR()
}

// Rebuild recomputes the rows from the scope and the overview.
func (t *projectsTab) Rebuild() {
	env := t.env()
	s, m := env.Styles, env.Msg.WS
	t.tbl.SetStyles(s)
	t.fl.SetStyles(s)
	t.tbl.SetColumns([]table.Column{
		{Title: m.ColFolder, Min: 10, Max: 24, Priority: 70},
		{Title: m.ColGitLab, Min: 22, Flex: true, Priority: 100},
		{Title: m.ColBranch, Min: 12, Max: 24, Priority: 60},
		{Title: m.ColAheadBehind, Min: 3, Max: 8, Priority: 40},
		{Title: m.ColMRs, Min: 4, Max: 7, Priority: 90},
		{Title: m.ColDefault, Min: 10, Max: 14, Priority: 80},
		{Title: m.ColTag, Min: 4, Max: 14, Priority: 30},
		{Title: m.ColActivity, Min: 3, Priority: 50, AlignRight: true},
	})
	t.rows = t.sortedRows()
	rows := make([]table.Row, 0, len(t.rows))
	for _, r := range t.rows {
		rows = append(rows, t.row(r))
	}
	t.tbl.SetRows(rows)
	if !t.moved && len(rows) > 0 {
		t.tbl.SetCursorID(rows[0].ID) // follow the order until the user moves
	}
	t.tbl.SetEmpty("")
	switch {
	case t.sh.err != nil:
		t.tbl.SetTail(table.TailError, env.Msg.Project.LoadFailed)
	case !t.sh.ready:
		t.tbl.SetTail(table.TailLoading, env.Msg.Project.Loading)
	default:
		t.tbl.SetTail(table.TailNone, "")
	}
	t.layout()
}

// sortedRows orders the rows by project activity (the overview order),
// foreign folders last, and applies the text filter.
func (t *projectsTab) sortedRows() []workspace.Project {
	rank := t.sh.rank()
	rows := make([]workspace.Project, 0, len(t.sh.scope.Rows))
	q := strings.ToLower(t.text)
	for _, r := range t.sh.scope.Rows {
		if q != "" && !strings.Contains(strings.ToLower(t.filterText(r)), q) {
			continue
		}
		rows = append(rows, r)
	}
	key := func(r workspace.Project) (int, int, string) {
		name := r.Ref.FullPath
		if r.Local != nil {
			name = r.Local.Name
		}
		if r.Foreign {
			return 1, 0, name
		}
		pos, ok := rank[r.Ref.FullPath]
		if !ok {
			pos = len(rank)
		}
		return 0, pos, name
	}
	sort.SliceStable(rows, func(i, j int) bool {
		gi, pi, ni := key(rows[i])
		gj, pj, nj := key(rows[j])
		if gi != gj {
			return gi < gj
		}
		if pi != pj {
			return pi < pj
		}
		return ni < nj
	})
	return rows
}

// filterText is what the text filter matches: the folder name and the
// project path (SPEC §3.3), never the absolute path of the folder.
func (t *projectsTab) filterText(r workspace.Project) string {
	text := t.remoteText(r)
	if r.Local != nil {
		text = filepath.Base(r.Local.Dir) + " " + text
	}
	return text
}

// rowID identifies a row: the folder, or the project path of a group row.
func rowID(r workspace.Project) string {
	if r.Local != nil {
		return r.Local.Dir
	}
	return r.Ref.FullPath
}

// remoteText is the GitLab cell of a row.
func (t *projectsTab) remoteText(r workspace.Project) string {
	m := t.env().Msg
	if !r.Foreign {
		return r.Ref.FullPath
	}
	if r.Local == nil || r.Local.Remote == nil {
		if r.Local != nil && r.Local.Kind == gitremote.KindNoRemote {
			return m.Startup.NoRemote
		}
		return m.Startup.ForeignRemote
	}
	return r.Local.Remote.Host + "/" + r.Local.Remote.Path + " · " + m.Startup.ForeignRemote
}

func (t *projectsTab) row(r workspace.Project) table.Row {
	env := t.env()
	s, m := env.Styles, env.Msg.WS
	path := r.Ref.FullPath
	folder, branch := "—", ""
	if r.Local != nil {
		folder = r.Local.Name
		if repo := r.Local.Repo; repo != nil {
			branch = repo.Branch
		}
	}
	muted := r.Foreign || t.sh.noAccess(path)
	gl := t.remoteText(r)
	ov, known := t.sh.overview(path)
	switch {
	case r.Foreign:
	case t.sh.noAccess(path):
		gl += " · " + m.NoAccess
	case known && ov.Archived:
		gl += " · " + s.Warning.Render(m.Archived)
	}
	loading := s.Muted.Render("◐")
	dash := s.Muted.Render("—")
	mrs, def, tag, act := dash, dash, dash, dash
	if !muted {
		if !known && t.sh.loading {
			mrs, def, tag, act = loading, loading, loading, loading
		}
		if known {
			mrs = fmt.Sprint(ov.OpenMRs)
			if ov.OpenMRs == 0 {
				mrs = s.Muted.Render("0")
			}
			if ov.ReviewMRs > 0 {
				mrs += " " + s.Accent.Render(fmt.Sprintf("·%d", ov.ReviewMRs))
			}
			switch {
			case ov.DefaultPipeline != nil:
				def = s.Status(ov.DefaultPipeline.Status)
			case ov.Phase < 2 && t.sh.loading:
				def = loading
			}
			switch {
			case ov.LastTag != "":
				tag = ov.LastTag
			case !ov.TagKnown && t.sh.loading:
				tag = loading
			}
			act = render.Ago(env.Lang, env.Now(), ov.LastActivityAt)
		} else if r.Info != nil {
			act = render.Ago(env.Lang, env.Now(), r.Info.LastActivityAt)
		}
	}
	return table.Row{ID: rowID(r), Muted: muted, Cells: []string{
		folder, gl, branch, t.aheadBehind(r), mrs, def, tag, act,
	}}
}

// aheadBehind is the ↑↓ cell from the git tracker.
func (t *projectsTab) aheadBehind(r workspace.Project) string {
	if r.Local == nil || r.Local.Repo == nil {
		return ""
	}
	return t.env().Git.Cell(t.env().Styles, r.Local.Dir)
}

// gitLines describe the last fetch/pull of a folder: the outcome with its
// reason, and for a failure or skip the last line of git's output.
func (t *projectsTab) gitLines(r workspace.Project) []string {
	env := t.env()
	if r.Local == nil {
		return nil
	}
	res, ok := env.Git.Result(r.Local.Dir)
	if !ok || env.Git.Running(r.Local.Dir) {
		return nil
	}
	s, g := env.Styles, env.Msg.Git
	op := res.Op.String()
	var line string
	switch res.Status {
	case gitexec.StatusFailed:
		line = s.Error.Render("✗ " + op + ": " + localgit.ReasonText(env.Msg, res))
		if res.Reason == gitexec.ReasonAuth {
			line += s.Muted.Render(" — " + g.AuthHint(r.Local.Dir))
		}
	case gitexec.StatusSkipped:
		line = s.Warning.Render("○ " + op + ": " + localgit.ReasonText(env.Msg, res))
	case gitexec.StatusUpToDate:
		line = s.Muted.Render("✓ " + op + ": " + g.UpToDate)
	default:
		line = s.Success.Render("✓ " + op)
		if res.Moved > 0 {
			line = s.Success.Render("✓ " + op + " " + g.Moved(res.Moved))
		}
	}
	out := []string{line}
	if res.Status == gitexec.StatusFailed || res.Status == gitexec.StatusSkipped {
		if n := len(res.Output); n > 0 {
			out = append(out, s.Muted.Render(res.Output[n-1]))
		}
	}
	return out
}

func (t *projectsTab) selected() *workspace.Project {
	id := t.tbl.CursorID()
	for i := range t.rows {
		if rowID(t.rows[i]) == id {
			return &t.rows[i]
		}
	}
	return nil
}

// localBranch returns the project and branch whose open MR the preview
// shows for the selected row, or "" when there is none.
func (t *projectsTab) localBranch() (path, branch string) {
	r := t.selected()
	if r == nil || r.Foreign || r.Local == nil || r.Local.Repo == nil || r.Local.Repo.Detached || r.Local.Repo.Branch == "" {
		return "", ""
	}
	if t.sh.noAccess(r.Ref.FullPath) {
		return "", ""
	}
	return r.Ref.FullPath, r.Local.Repo.Branch
}

// selection identifies the selected row with the project and branch whose
// MR the preview shows.
func (t *projectsTab) selection() string {
	path, branch := t.localBranch()
	return t.tbl.CursorID() + "\x00" + path + "@" + branch
}

// loadLocalMR asks for the open MR of the selected row's local branch,
// once per row and branch.
func (t *projectsTab) loadLocalMR() tea.Cmd {
	path, branch := t.localBranch()
	if path == "" || !t.active || (t.sh.loading && !t.moved) {
		return nil // while the order may still change, wait for the user or the overview
	}
	key := path + "@" + branch
	if _, done := t.localMR[key]; done || t.lmFor == key {
		return nil
	}
	if t.lmCan != nil {
		t.lmCan()
	}
	tok := t.lmGen.Next()
	force := t.lmForce
	t.lmFor, t.lmForce = key, false
	env := t.env()
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	t.lmCan = cancel
	ref := gitlab.ProjectRef{FullPath: path}
	f := gitlab.MRFilter{State: gitlab.MRStateOpened, SourceBranch: branch}
	return func() tea.Msg {
		defer cancel()
		page, _, err := common.Fetch(ctx, env.Cache, "mrbranch:"+key, localMRTTL, force, func(ctx context.Context) (gitlab.Page[gitlab.MergeRequest], error) {
			return env.Client.MergeRequests().List(ctx, ref, f, "", 1)
		})
		msg := localMRMsg{tok: tok, key: key, err: err}
		if err == nil && len(page.Items) > 0 {
			msg.mr = &page.Items[0]
		}
		return msg
	}
}

func (t *projectsTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	if cmd := t.fl.HandleMsg(msg); cmd != nil {
		t.layout()
		return t, cmd
	}
	switch msg := msg.(type) {
	case localMRMsg:
		if !t.lmGen.Valid(msg.tok) {
			return t, nil
		}
		t.lmFor = ""
		if msg.err == nil {
			t.localMR[msg.key] = msg.mr
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		t.Rebuild()
	case tea.KeyPressMsg:
		return t, t.onKey(msg)
	}
	return t, nil
}

func (t *projectsTab) onKey(k tea.KeyPressMsg) tea.Cmd {
	env := t.env()
	if t.fl.Focused() {
		res, cmd := t.fl.Update(k)
		switch res {
		case filterline.Applied:
			t.text = strings.TrimSpace(t.fl.Value())
			t.fl.Close()
			t.Rebuild()
		case filterline.Cancelled:
			t.layout()
		}
		t.layout()
		return tea.Batch(cmd, t.loadLocalMR())
	}
	if t.tbl.HandleKey(k) {
		t.moved = true
		return t.loadLocalMR()
	}
	switch k.String() {
	case "/":
		cmd := t.fl.Open(t.text)
		t.layout()
		return cmd
	case "backspace":
		if t.text != "" {
			t.text = ""
			t.Rebuild()
			return t.loadLocalMR()
		}
	case "enter", "f":
		r := t.selected()
		if r == nil || r.Foreign {
			return nil
		}
		var repo *gitremote.Repo
		if r.Local != nil {
			repo = r.Local.Repo
		}
		mf := gitlab.MRFilter{State: gitlab.MRStateOpened, Scope: gitlab.MRScopeAll}
		return t.sh.openProject(r.Ref.FullPath, repo, 0, &mf, nil, "")
	case "o":
		if r := t.selected(); r != nil && !r.Foreign {
			return app.OpenURL(env, t.sh.projectURL(r.Ref.FullPath))
		}
	case "y":
		if r := t.selected(); r != nil && !r.Foreign {
			u := t.sh.projectURL(r.Ref.FullPath)
			return app.Copy(env, u, u)
		}
	}
	return nil
}

func (t *projectsTab) KeyMap() common.KeyMap {
	k, w := t.env().Msg.Keys, t.env().Msg.WS
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", w.KeyOpenProject, common.PriorityHigh),
		common.NewBinding([]string{"/"}, "", k.Filter, common.PriorityNormal),
		common.NewBinding([]string{"o"}, "", k.Browser, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
	}}
}

// ---- view ----

func (t *projectsTab) filterLine() string {
	env := t.env()
	s := env.Styles
	if t.fl.Focused() {
		return t.fl.ViewWithHint(env.Msg.WS.FilterHint, s.Muted)
	}
	extra := s.Muted.Render(env.Msg.WS.Search)
	if t.text != "" {
		extra = s.FilterValue.Render(t.text)
	}
	right := ""
	if t.text != "" {
		right = env.Msg.Project.Counter(len(t.rows), len(t.sh.scope.Rows), false)
	}
	return common.FilterBar(s, nil, extra, right, t.w)
}

func (t *projectsTab) emptyState() string {
	env := t.env()
	s, m := env.Styles, env.Msg
	sc := t.sh.scope
	foreign := 0
	for _, r := range sc.Rows {
		if r.Foreign {
			foreign++
		}
	}
	scanned := len(sc.Rows) + len(sc.Skipped)
	return "\n  " + s.Bold.Render(m.Startup.EmptyWorkspace(t.sh.source)) + "\n" +
		"  " + s.Muted.Render(m.Startup.EmptyWorkspaceDetail(scanned, foreign, len(sc.Skipped))) + "\n" +
		"  " + s.Muted.Render(m.WS.EmptyHint)
}

func (t *projectsTab) View() string {
	parts := []string{t.filterLine()}
	if t.empty() {
		parts = append(parts, t.emptyState())
	}
	parts = append(parts, t.tbl.View())
	if sk := t.sh.scope.Skipped; len(sk) > 0 {
		parts = append(parts, "  "+t.env().Styles.Muted.Render(t.env().Msg.Startup.SkippedNoGit(strings.Join(sk, ", "))))
	}
	if t.previewShown() {
		parts = append(parts, t.preview())
	}
	return strings.Join(parts, "\n")
}

// preview: folder → project · remote; open MRs, the default branch
// pipeline and the open MR of the local branch; the git result.
func (t *projectsTab) preview() string {
	env := t.env()
	s, m := env.Styles, env.Msg.WS
	frame := func(lines ...string) string {
		for i := range lines {
			lines[i] = " " + render.Clip(lines[i], t.w-4)
		}
		return common.Box(s, strings.Join(lines, "\n"), t.w, projPrevLns+2, common.BoxNormal)
	}
	r := t.selected()
	if r == nil {
		return frame("")
	}
	dot := s.Muted.Render("  ·  ")
	path := r.Ref.FullPath
	var head string
	if r.Local != nil {
		head = s.PreviewTitle.Render(r.Local.Name) + "  "
	}
	if r.Foreign {
		head += s.Muted.Render(t.remoteText(*r))
	} else {
		head += s.Muted.Render("→ ") + s.Accent.Render(path)
	}
	if r.Local != nil && r.Local.Repo != nil && r.Local.Repo.RemoteURL != "" {
		head += s.Muted.Render("  · " + r.Local.Repo.Remote + ": " + r.Local.Repo.RemoteURL)
	}
	if r.Foreign {
		return frame(append([]string{head}, t.gitLines(*r)...)...)
	}
	if t.sh.noAccess(path) {
		return frame(head, s.Error.Render("✗ "+env.Msg.Errors.NotFound))
	}
	ov, known := t.sh.overview(path)
	var sum []string
	if known {
		sum = append(sum, m.PrevOpenMRs(ov.OpenMRs, ov.ReviewMRs))
		switch p := ov.DefaultPipeline; {
		case p != nil:
			sum = append(sum, ov.DefaultBranch+" "+s.Accent.Render(fmt.Sprintf("#%d", p.ID))+" "+s.Status(p.Status))
		case ov.DefaultBranch != "" && ov.Phase >= 2:
			sum = append(sum, ov.DefaultBranch+" "+s.Muted.Render(m.PrevNoPipe))
		}
	} else {
		sum = append(sum, s.Muted.Render(m.PrevLoading))
	}
	if p, b := t.localBranch(); p != "" {
		local := m.PrevLocal(b)
		if mr, done := t.localMR[p+"@"+b]; done {
			if mr != nil {
				local += " → " + s.Accent.Render(fmt.Sprintf("!%d", mr.IID))
				if mr.HeadPipeline != nil {
					local += " " + s.StatusGlyph(mr.HeadPipeline.Status)
				}
			} else {
				local += s.Muted.Render(" · " + m.PrevNoMR)
			}
		}
		sum = append(sum, local)
	}
	return frame(append([]string{head, strings.Join(sum, dot)}, t.gitLines(*r)...)...)
}
