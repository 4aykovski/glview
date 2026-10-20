// Package pipeline is the pipeline screen: stages as columns, jobs inside,
// downstream pipelines, a panel of the selected job and job actions.
package pipeline

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/joblog"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	minColW        = 25 // narrowest stage column, including the gap
	panelLines     = 4
	finalTTL       = 10 * time.Minute
	requestTimeout = 15 * time.Second
)

type (
	loadedMsg struct {
		tok common.Token
		pl  gitlab.Pipeline
		err error
	}
	traceMsg struct {
		tok  common.Token
		job  int64
		line string
	}
	jobActionMsg struct {
		kind jobAction
		name string
		err  error
	}
)

type jobAction int

const (
	actPlay jobAction = iota
	actRetry
	actCancel
)

// item is one row of a stage column: a job or a trigger job (bridge).
type item struct {
	job    gitlab.Job
	bridge *gitlab.Bridge
}

type column struct {
	stage gitlab.StageSummary
	items []item
}

// Screen is the pipeline screen.
type Screen struct {
	env  *app.Env
	ref  gitlab.ProjectRef
	id   int64
	pl   gitlab.Pipeline
	have bool // pl holds data (not only the summary the screen was opened with)

	gen, logGen, poll common.Gen
	logCancel         context.CancelFunc

	cols   []column
	si     int   // active stage
	sel    []int // selected item per stage
	xoff   int   // first visible column
	logFor int64
	logTxt string

	updated time.Time
	active  bool
	w, h    int
}

// New opens pipeline id of project ref; summary (from a list) is shown
// until the details arrive and may be nil.
func New(env *app.Env, ref gitlab.ProjectRef, id int64, summary *gitlab.PipelineSummary) *Screen {
	s := &Screen{env: env, ref: ref, id: id, gen: common.NewGen(), logGen: common.NewGen(), poll: common.NewGen()}
	if summary != nil {
		s.pl.PipelineSummary = *summary
	}
	return s
}

func (s *Screen) Init() tea.Cmd { return nil }

func (s *Screen) Title() string { return fmt.Sprintf("%s #%d", s.ref.FullPath, s.id) }

func (s *Screen) SetSize(w, h int) {
	s.w, s.h = w, h
	s.ensureVisible()
}

// Pipeline returns the loaded pipeline (for tests).
func (s *Screen) Pipeline() gitlab.Pipeline { return s.pl }

// Selected returns the selected item's job (zero value when none).
func (s *Screen) Selected() (gitlab.Job, bool) {
	it := s.current()
	if it == nil {
		return gitlab.Job{}, false
	}
	return it.job, true
}

func (s *Screen) KeyMap() common.KeyMap {
	k := s.env.Msg.Keys
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"h", "left"}, "h/l", k.Stage, common.PriorityHigh),
		common.NewBinding([]string{"j", "down"}, "j/k", k.Job, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", k.Log, common.PriorityHigh),
		common.NewBinding([]string{"a"}, "", k.Attempts, common.PriorityLow),
		common.NewBinding([]string{"P"}, "", k.Play, common.PriorityNormal),
		common.NewBinding([]string{"R"}, "", k.Retry, common.PriorityNormal),
		common.NewBinding([]string{"c"}, "", k.Cancel, common.PriorityNormal),
		common.NewBinding([]string{"r"}, "", k.Refresh, common.PriorityNormal),
		common.NewBinding([]string{"o"}, "", k.BrowserOne, common.PriorityLow),
		common.NewBinding([]string{"O"}, "", k.BrowserJob, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
	}}
}

// ---- loading ----

func (s *Screen) load(force bool) tea.Cmd {
	tok := s.gen.Next()
	env, ref, id := s.env, s.ref, s.id
	key := fmt.Sprintf("pipe:%s:%d", ref.Key(), id)
	ttl := time.Duration(0)
	if s.pl.Status.Final() {
		ttl = finalTTL
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		pl, _, err := common.Fetch(ctx, env.Cache, key, ttl, force, func(ctx context.Context) (gitlab.Pipeline, error) {
			return env.Client.Pipelines().Get(ctx, ref, id)
		})
		return loadedMsg{tok: tok, pl: pl, err: err}
	}
}

func (s *Screen) hasActive() bool {
	if s.pl.Status.Active() {
		return true
	}
	for _, j := range s.pl.Jobs {
		if j.Status.Active() {
			return true
		}
	}
	return false
}

func (s *Screen) pollDelay() time.Duration { return s.env.PollInterval(s.hasActive()) }

func (s *Screen) schedulePoll() tea.Cmd {
	if !s.active || s.env.AuthLost {
		s.poll.Next()
		return nil
	}
	return common.PollAfter(s.pollDelay(), s.poll.Next())
}

// loadTrace fetches the tail of the log of the selected running or failed
// job for the panel; the previous request is cancelled.
func (s *Screen) loadTrace() tea.Cmd {
	it := s.current()
	if s.logCancel != nil {
		s.logCancel()
		s.logCancel = nil
	}
	if it == nil || it.bridge != nil || (it.job.Status != gitlab.StatusRunning && it.job.Status != gitlab.StatusFailed) {
		s.logFor, s.logTxt = 0, ""
		return nil
	}
	if s.logFor == it.job.ID && it.job.Status == gitlab.StatusFailed {
		return nil // a finished job's log does not change
	}
	tok := s.logGen.Next()
	jobID := it.job.ID
	s.logFor, s.logTxt = jobID, ""
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	s.logCancel = cancel
	client, ref := s.env.Client, s.ref
	return func() tea.Msg {
		defer cancel()
		raw, err := client.Jobs().Trace(ctx, ref, jobID)
		if err != nil {
			return traceMsg{tok: tok, job: jobID}
		}
		return traceMsg{tok: tok, job: jobID, line: lastLine(raw)}
	}
}

// lastLine returns the last non-empty line of a trace without colors and
// progress rewrites.
func lastLine(raw []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if j := strings.LastIndexByte(l, '\r'); j >= 0 && j < len(l)-1 {
			l = l[j+1:]
		}
		l = strings.TrimSpace(ansi.Strip(strings.ReplaceAll(l, "\r", "")))
		if l != "" && !strings.HasPrefix(l, "section_start:") && !strings.HasPrefix(l, "section_end:") {
			return l
		}
	}
	return ""
}

// ---- model ----

// rebuild derives the columns from the pipeline, keeping the selection.
func (s *Screen) rebuild() {
	var selJob int64
	if it := s.current(); it != nil {
		selJob = it.job.ID
	}
	items := map[string][]item{}
	for _, j := range s.pl.Jobs {
		items[j.Stage] = append(items[j.Stage], item{job: j})
	}
	for i := range s.pl.Bridges {
		b := s.pl.Bridges[i]
		items[b.Stage] = append(items[b.Stage], item{job: b.Job, bridge: &b})
	}
	s.cols = s.cols[:0]
	stages := s.pl.Stages
	if len(stages) == 0 {
		seen := map[string]bool{}
		for _, j := range s.pl.Jobs {
			if !seen[j.Stage] {
				seen[j.Stage] = true
				stages = append(stages, gitlab.StageSummary{Name: j.Stage})
			}
		}
	}
	for _, st := range stages {
		its := items[st.Name]
		sort.SliceStable(its, func(i, j int) bool { return its[i].job.ID < its[j].job.ID })
		s.cols = append(s.cols, column{stage: st, items: its})
	}
	sel := make([]int, len(s.cols))
	copy(sel, s.sel)
	s.sel = sel
	s.si = min(s.si, max(0, len(s.cols)-1))
	for i, c := range s.cols {
		s.sel[i] = max(0, min(s.sel[i], len(c.items)-1))
	}
	if selJob != 0 { // follow the job across reloads and retries
		for i, c := range s.cols {
			for k, it := range c.items {
				if it.job.ID == selJob || slices.ContainsFunc(it.job.Attempts, func(a gitlab.Job) bool { return a.ID == selJob }) {
					s.si, s.sel[i] = i, k
				}
			}
		}
	}
	s.ensureVisible()
}

func (s *Screen) current() *item {
	if s.si >= len(s.cols) || len(s.cols[s.si].items) == 0 {
		return nil
	}
	return &s.cols[s.si].items[s.sel[s.si]]
}

func (s *Screen) hasDownstream() bool {
	for _, b := range s.pl.Bridges {
		if b.Downstream != nil {
			return true
		}
	}
	return false
}

func (s *Screen) columnCount() int {
	n := len(s.cols)
	if s.hasDownstream() {
		n++
	}
	return n
}

// stageWidth is the width of a stage column including the one-cell gap: the
// columns share the screen when n of them fit at minColW or wider, otherwise
// they stay minColW and scroll.
func stageWidth(w, n int) int {
	if n > 0 && n*minColW <= w {
		return w / n
	}
	return minColW
}

// colWidth is the width of column i of n: the division remainder goes to the
// last column when the columns are stretched.
func colWidth(w, n, i int) int {
	cw := stageWidth(w, n)
	if n > 0 && i == n-1 && n*minColW <= w {
		cw += w - cw*n
	}
	return cw
}

// colInner is the text width inside a column frame (the frame, the gap and
// one space of padding on each side).
func colInner(cw int) int { return cw - 1 - 4 }

func (s *Screen) visibleCols() int {
	n := s.columnCount()
	return min(max(1, n), max(1, s.w/stageWidth(s.w, n)))
}

func (s *Screen) ensureVisible() {
	vis := s.visibleCols()
	if s.si < s.xoff {
		s.xoff = s.si
	}
	if s.si >= s.xoff+vis {
		s.xoff = s.si - vis + 1
	}
	s.xoff = max(0, min(s.xoff, max(0, s.columnCount()-vis)))
}

// ---- update ----

func (s *Screen) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	env := s.env
	switch msg := msg.(type) {
	case app.ActivatedMsg:
		s.active = true
		if !s.have {
			return s, s.load(false)
		}
		if env.Now().Sub(s.updated) >= s.pollDelay() && !env.AuthLost {
			return s, s.load(true)
		}
		return s, s.schedulePoll()
	case app.DeactivatedMsg:
		s.active = false
		s.poll.Next()

	case loadedMsg:
		if !s.gen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			return s, tea.Batch(env.Fail(msg.err), s.schedulePoll())
		}
		s.pl, s.have, s.updated = msg.pl, true, env.Now()
		s.rebuild()
		return s, tea.Batch(s.schedulePoll(), s.loadTrace())

	case traceMsg:
		if s.logGen.Valid(msg.tok) && msg.job == s.logFor {
			s.logTxt = msg.line
		}

	case jobActionMsg:
		if !s.active {
			return s, nil
		}
		if env.Cache != nil {
			env.Cache.Invalidate(fmt.Sprintf("pipe:%s:%d", s.ref.Key(), s.id))
			env.Cache.Invalidate("pipes:" + s.ref.FullPath + ":")
		}
		if msg.err != nil {
			return s, env.Fail(msg.err)
		}
		m := env.Msg.Pipe
		text := map[jobAction]string{actPlay: m.JobPlayed(msg.name), actRetry: m.JobRetried(msg.name), actCancel: m.JobCanceled(msg.name)}[msg.kind]
		return s, tea.Batch(app.Toast(common.ToastSuccess, text, ""), s.load(true))

	case common.PollTickMsg:
		if !s.poll.Valid(msg.Token) || !s.active || env.AuthLost {
			return s, nil
		}
		return s, s.load(true)
	case app.CacheRevalidatedMsg:
		if msg.Key == fmt.Sprintf("pipe:%s:%d", s.ref.Key(), s.id) {
			return s, s.load(false)
		}
	case app.AuthLostMsg:
		s.poll.Next()
	case app.AuthRestoredMsg:
		if s.active {
			return s, s.load(true)
		}
	case app.FocusChangedMsg:
		if msg.Focused && s.active && s.have && env.Now().Sub(s.updated) >= s.pollDelay() {
			return s, s.load(true)
		}
	case tea.KeyPressMsg:
		return s, s.onKey(msg)
	}
	return s, nil
}

func (s *Screen) onKey(k tea.KeyPressMsg) tea.Cmd {
	env := s.env
	switch k.String() {
	case "h", "left":
		return s.moveStage(-1)
	case "l", "right":
		return s.moveStage(1)
	case "j", "down":
		return s.moveJob(1)
	case "k", "up":
		return s.moveJob(-1)
	case "g":
		return s.moveJob(-1 << 20)
	case "G":
		return s.moveJob(1 << 20)
	case "r":
		return s.load(true)
	case "y":
		return app.Copy(env, s.pl.WebURL, render.ShortURL(s.pl.WebURL))
	case "o":
		return app.OpenURL(env, s.pl.WebURL)
	case "O":
		return s.openJobURL()
	}
	it := s.current()
	if it == nil {
		return nil
	}
	m := env.Msg.Pipe
	switch k.String() {
	case "enter":
		return s.open(it)
	case "P":
		if it.bridge != nil || it.job.Status != gitlab.StatusManual {
			return app.Toast(common.ToastInfo, m.NotManual, "")
		}
		return app.OpenOverlay(newPlayForm(env, s.ref, it.job))
	case "R":
		if it.bridge != nil {
			return nil
		}
		return s.jobCmd(actRetry, it.job)
	case "a":
		if it.bridge != nil || len(it.job.Attempts) == 0 {
			return nil
		}
		ref, id, job := s.ref, s.id, it.job
		return app.OpenOverlay(joblog.NewAttemptsList(env, joblog.AttemptsOf(job), 0, func(i int) tea.Cmd {
			return app.Push(joblog.NewAttempt(env, ref, id, job, i))
		}))
	case "c":
		if it.bridge != nil {
			return nil
		}
		job := it.job
		return app.OpenOverlay(app.NewConfirm(env, m.CancelJobTitle(job.Name), m.CancelJobBody, s.jobCmd(actCancel, job)))
	}
	return nil
}

func (s *Screen) moveStage(d int) tea.Cmd {
	if len(s.cols) == 0 {
		return nil
	}
	s.si = max(0, min(s.si+d, len(s.cols)-1))
	s.ensureVisible()
	return s.loadTrace()
}

func (s *Screen) moveJob(d int) tea.Cmd {
	if s.si >= len(s.cols) {
		return nil
	}
	n := len(s.cols[s.si].items)
	s.sel[s.si] = max(0, min(s.sel[s.si]+d, max(0, n-1)))
	return s.loadTrace()
}

// openJobURL opens the selected job in the browser; for a bridge with a
// downstream pipeline, that pipeline.
func (s *Screen) openJobURL() tea.Cmd {
	it := s.current()
	if it == nil {
		return nil
	}
	url := it.job.WebURL
	if b := it.bridge; b != nil && b.Downstream != nil && b.Downstream.WebURL != "" {
		url = b.Downstream.WebURL
	}
	if url == "" {
		return nil
	}
	return app.OpenURL(s.env, url)
}

// open handles enter: the log of a job, or the downstream pipeline of a bridge.
func (s *Screen) open(it *item) tea.Cmd {
	env := s.env
	m := env.Msg.Pipe
	if it.bridge != nil {
		b := it.bridge
		switch {
		case b.Downstream != nil:
			d := *b.Downstream
			return app.Push(New(env, d.Project, d.ID, &d))
		case b.Status.Active() || b.Status == gitlab.StatusManual || b.Status == gitlab.StatusCreated:
			return nil // waiting for the downstream pipeline
		}
		return app.Toast(common.ToastInfo, m.ToastNoAccess, "")
	}
	return app.Push(joblog.New(env, s.ref, s.id, it.job))
}

func (s *Screen) jobCmd(kind jobAction, job gitlab.Job) tea.Cmd {
	client, ref := s.env.Client, s.ref
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		var err error
		switch kind {
		case actRetry:
			_, err = client.Jobs().Retry(ctx, ref, job.ID)
		case actCancel:
			err = client.Jobs().Cancel(ctx, ref, job.ID)
		}
		return jobActionMsg{kind: kind, name: job.Name, err: err}
	}
}

// ---- view ----

func (s *Screen) summaryLine() string {
	m := s.env.Msg
	counts := map[string]int{}
	order := []string{"✓", "✗", "◐", "►", "⊘", "○"}
	total := 0
	for _, j := range s.pl.Jobs {
		counts[common.Glyph(j.Status)]++
		total++
	}
	var parts []string
	for _, g := range order {
		if counts[g] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[g], g))
		}
	}
	if total == 0 {
		return m.Pipe.NoJobs
	}
	return m.Pipe.Summary(m.Plural.Jobs(total), strings.Join(parts, " "))
}

func (s *Screen) View() string {
	env := s.env
	st, m := env.Styles, env.Msg
	p := s.pl

	crumbs := st.Muted.Render(s.ref.FullPath+" › "+m.Pipe.Crumb+" › ") + st.Accent.Render(fmt.Sprintf("#%d", s.id))
	head := common.Header(st, crumbs, env.HeaderRight(s.updated), s.w)

	line1 := st.Status(p.Status) + "  " + st.Bold.Render(fmt.Sprintf("#%d", s.id))
	if p.Ref != "" {
		line1 += " on " + render.PipelineRef(p.Ref)
	}
	if p.SHA != "" {
		line1 += " · " + render.ShortSHA(p.SHA)
		if p.CommitTitle != "" {
			line1 += " " + p.CommitTitle
		}
	}
	who := "—"
	if p.User != nil {
		who = p.User.Username
	}
	line2 := st.Muted.Render(who+" · "+p.Source+" · "+render.Duration(p.Duration)) + "  " + s.summaryLine()

	bodyH := max(1, s.h-4-(panelLines+2))
	var body string
	switch {
	case s.have && len(s.cols) == 0 && p.YAMLErrors != "":
		body = st.Error.Render(m.Pipe.YAMLErrors) + "\n" + p.YAMLErrors
	case !s.have:
		body = st.Muted.Render(m.Project.Loading)
	default:
		body = s.columnsView(bodyH)
	}
	body = common.FitHeight(body, bodyH)
	return strings.Join([]string{head, common.Rule(st, s.w), render.Clip(line1, s.w), render.Clip(line2, s.w), body, s.panel()}, "\n")
}

// StatusHint is the footer status: how often the pipeline is refreshed.
func (s *Screen) StatusHint() string {
	return s.env.Msg.Project.StatusPoll(s.pollDelay().Round(time.Second).String())
}

func (s *Screen) columnsView(h int) string {
	st := s.env.Styles
	vis := s.visibleCols()
	more := s.xoff > 0 || s.xoff+vis < s.columnCount()
	avail := h
	if more {
		avail--
	}
	// All boxes share one height: the tallest column, capped by the space.
	rows := 0
	for _, c := range s.cols {
		rows = max(rows, len(c.items))
	}
	if s.hasDownstream() {
		rows = max(rows, s.downstreamRows())
	}
	boxH := max(4, min(avail, rows+4))

	var cols []string
	for i := s.xoff; i < min(s.columnCount(), s.xoff+vis); i++ {
		if i < len(s.cols) {
			cols = append(cols, s.stageView(i, boxH))
		} else {
			cols = append(cols, s.downstreamView(boxH))
		}
		cols = append(cols, " ")
	}
	out := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	if more {
		left, right := " ", " "
		if s.xoff > 0 {
			left = "◂"
		}
		if s.xoff+vis < s.columnCount() {
			right = "▸"
		}
		out += "\n" + st.Muted.Render(left+strings.Repeat(" ", max(0, s.w-2))+right)
	}
	return out
}

func (s *Screen) stageView(i, h int) string {
	st := s.env.Styles
	c := s.cols[i]
	cw := colWidth(s.w, s.columnCount(), i)
	inner := colInner(cw)
	name := ansi.Truncate(c.stage.Name, inner-6, "…")
	status := st.StatusGlyph(c.stage.Status)
	if c.stage.Status.Active() && c.stage.Total > 0 {
		status += st.Muted.Render(fmt.Sprintf(" %d/%d", c.stage.Done, c.stage.Total))
	}
	gap := max(1, inner-ansi.StringWidth(name)-ansi.StringWidth(status))
	lines := []string{st.Bold.Render(name) + strings.Repeat(" ", gap) + status, common.Rule(st, inner)}
	for k, it := range c.items {
		lines = append(lines, s.itemLine(it, inner, i == s.si && k == s.sel[i]))
	}
	for k := range lines {
		lines[k] = " " + lines[k]
	}
	kind := common.BoxNormal
	if i == s.si {
		kind = common.BoxActive
	}
	return common.Box(st, strings.Join(lines, "\n"), cw-1, h, kind)
}

func (s *Screen) itemLine(it item, w int, selected bool) string {
	st, m := s.env.Styles, s.env.Msg.Pipe
	dur := ""
	if it.job.Duration > 0 {
		dur = render.Duration(it.job.Duration)
	}
	name := it.job.Name
	glyph := common.Glyph(it.job.Status)
	suffix := ""
	if it.bridge != nil {
		glyph = "⇢"
		switch b := it.bridge; {
		case b.Downstream == nil && (b.Status.Active() || b.Status == gitlab.StatusManual || b.Status == gitlab.StatusCreated):
			suffix = " · " + m.BridgeWaiting
		case b.Downstream == nil:
			suffix = " · " + m.BridgeNoAccess
		}
	}
	right := dur
	w-- // the selection marker
	room := w - ansi.StringWidth(right) - 1
	// The name gives way to the suffix: "⇢ not… · no access".
	text := glyph + " " + ansi.Truncate(name, max(1, room-2-ansi.StringWidth(suffix)), "…") + suffix
	text += strings.Repeat(" ", max(1, w-ansi.StringWidth(text)-ansi.StringWidth(right))) + right
	if selected {
		return st.RowSelected.Render("›" + pad(text, w))
	}
	if it.bridge != nil {
		return " " + strings.Replace(text, glyph, st.StatusStyle(it.job.Status).Render(glyph), 1)
	}
	return " " + strings.Replace(text, glyph, st.StatusGlyph(it.job.Status), 1)
}

func (s *Screen) downstreamRows() int {
	n := 0
	for _, b := range s.pl.Bridges {
		if b.Downstream != nil {
			n += 2
		}
	}
	return n
}

func (s *Screen) downstreamView(h int) string {
	st, m := s.env.Styles, s.env.Msg.Pipe
	cw := colWidth(s.w, s.columnCount(), s.columnCount()-1)
	inner := colInner(cw)
	lines := []string{st.Bold.Render(m.Downstream), common.Rule(st, inner)}
	for _, b := range s.pl.Bridges {
		if b.Downstream == nil {
			continue
		}
		d := b.Downstream
		proj := b.DownstreamProject
		if proj == "" {
			proj = d.Project.FullPath
		}
		lines = append(lines, ansi.Truncate("⇢ "+proj, inner, "…"), "  "+st.Status(d.Status))
	}
	for k := range lines {
		lines[k] = " " + lines[k]
	}
	return common.Box(st, strings.Join(lines, "\n"), cw-1, h, common.BoxDashed)
}

func (s *Screen) panel() string {
	st, m := s.env.Styles, s.env.Msg.Pipe
	frame := func(lines ...string) string {
		for i := range lines {
			lines[i] = " " + render.Clip(lines[i], s.w-4)
		}
		return common.Box(st, strings.Join(lines, "\n"), s.w, panelLines+2, common.BoxNormal)
	}
	it := s.current()
	if it == nil {
		return frame("", "", "", "")
	}
	j := it.job
	l1 := st.Bold.Render(j.Name) + "  " + st.Status(j.Status)
	l2 := fmt.Sprintf("job #%d · %s", j.ID, j.Stage)
	if j.Runner != "" {
		l2 += " · " + m.PanelRunner + " " + j.Runner
	}
	if j.Duration > 0 {
		l2 += " · " + render.Duration(j.Duration)
	}
	l3 := st.Muted.Render(fmt.Sprintf("%s: %t", m.PanelAllowFailure, j.AllowFailure))
	if len(j.Attempts) > 0 {
		l3 += st.Muted.Render(" · " + m.PanelAttempts + ": ")
		l3 += attemptsLine(st, j, s.w-4-ansi.StringWidth(l3))
	}
	l4 := ""
	if it.bridge == nil && (j.Status == gitlab.StatusRunning || j.Status == gitlab.StatusFailed) {
		l4 = st.Muted.Render("▎ ") + s.logTxt
	}
	return frame(l1, st.Muted.Render(l2), l3, l4)
}

func pad(s string, w int) string {
	if n := w - ansi.StringWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
