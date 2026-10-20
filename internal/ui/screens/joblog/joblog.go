// Package joblog is the job log screen: numbered lines, foldable sections,
// follow mode, search and horizontal scrolling.
package joblog

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	jl "github.com/4aykovski/glview/internal/joblog"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	pollEvery      = 3 * time.Second // logs are polled at a fixed 3s while the job runs
	requestTimeout = 30 * time.Second
	hScroll        = 8
)

type (
	logMsg struct {
		tok   common.Token
		job   gitlab.Job
		trace []byte
		err   error
	}
	actMsg struct {
		retry bool
		job   gitlab.Job
		err   error
	}
)

// Screen is the job log.
type Screen struct {
	env        *app.Env
	ref        gitlab.ProjectRef
	pipelineID int64
	job        gitlab.Job

	// The attempts of the job, newest first, and the one on screen; R and c
	// act on jobs[0], the latest.
	jobs []gitlab.Job
	sel  int

	p        jl.Parser
	consumed int
	folded   map[int]bool
	known    int   // sections already given their initial fold state
	vis      []int // indexes of visible lines
	cursor   int   // line index
	offset   int   // position in vis
	xoff     int
	follow   bool

	search    textinput.Model
	searching bool
	query     string
	matches   []jl.Match
	cur       int

	gen, poll common.Gen
	loaded    bool
	updated   time.Time
	active    bool
	w, h      int
}

// New opens the log of job (a job of pipeline pipelineID); job.Attempts
// are its earlier attempts that [ and ] switch to.
func New(env *app.Env, ref gitlab.ProjectRef, pipelineID int64, job gitlab.Job) *Screen {
	return NewAttempt(env, ref, pipelineID, job, 0)
}

// NewAttempt opens the log of attempt i of a retried job, counted from the
// latest one (0) as AttemptsOf(latest) lists them.
func NewAttempt(env *app.Env, ref gitlab.ProjectRef, pipelineID int64, latest gitlab.Job, i int) *Screen {
	in := common.NewInput(env.Styles)
	in.Prompt = env.Msg.Log.SearchPrompt + " "
	jobs := AttemptsOf(latest)
	i = max(0, min(i, len(jobs)-1))
	s := &Screen{
		env: env, ref: ref, pipelineID: pipelineID, job: jobs[i], jobs: jobs, sel: i,
		folded: map[int]bool{}, search: in, gen: common.NewGen(), poll: common.NewGen(),
	}
	s.follow = s.polling() // an earlier attempt is final
	return s
}

func (s *Screen) Init() tea.Cmd { return nil }

func (s *Screen) Title() string { return fmt.Sprintf("#%d › %s", s.pipelineID, s.job.Name) }

func (s *Screen) SetSize(w, h int) { s.w, s.h = w, h; s.clamp() }

// CapturesInput lets the search field take every key.
func (s *Screen) CapturesInput() bool { return s.searching }

// Job returns the job being shown (for tests).
func (s *Screen) Job() gitlab.Job { return s.job }

// target is the job R and c act on: the latest attempt.
func (s *Screen) target() gitlab.Job { return s.jobs[0] }

// Following reports follow mode (for tests).
func (s *Screen) Following() bool { return s.follow }

func (s *Screen) KeyMap() common.KeyMap {
	k := s.env.Msg.Keys
	b := []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"f"}, "", k.Follow, common.PriorityHigh),
		common.NewBinding([]string{"space"}, "", k.Fold, common.PriorityNormal),
		common.NewBinding([]string{"z"}, "", k.FoldAll, common.PriorityLow),
		common.NewBinding([]string{"/"}, "", k.Find, common.PriorityNormal),
		common.NewBinding([]string{"n"}, "n/N", k.Next, common.PriorityLow),
	}
	if len(s.jobs) > 1 {
		b = append(b,
			common.NewBinding([]string{"a"}, "", k.Attempts, common.PriorityLow),
			common.NewBinding([]string{"[", "]"}, "[/]", k.PrevNextAttempt, common.PriorityLow),
		)
	}
	return common.KeyMap{Short: append(b,
		common.NewBinding([]string{"R"}, "", k.Retry, common.PriorityNormal),
		common.NewBinding([]string{"c"}, "", k.Cancel, common.PriorityNormal),
		common.NewBinding([]string{"o"}, "", k.Browser, common.PriorityLow),
	)}
}

// ---- loading ----

func (s *Screen) load() tea.Cmd {
	tok := s.gen.Next()
	client, ref, id := s.env.Client, s.ref, s.job.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		job, err := client.Jobs().Get(ctx, ref, id)
		if err != nil {
			return logMsg{tok: tok, err: err}
		}
		trace, err := client.Jobs().Trace(ctx, ref, id)
		return logMsg{tok: tok, job: job, trace: trace, err: err}
	}
}

// polling is on while the job may still write output; an earlier attempt
// is final and loaded once.
func (s *Screen) polling() bool { return s.sel == 0 && s.job.Status.Active() }

func (s *Screen) schedulePoll() tea.Cmd {
	if !s.active || s.env.AuthLost || !s.polling() {
		s.poll.Next()
		return nil
	}
	return common.PollAfter(s.pollDelay(), s.poll.Next())
}

func (s *Screen) pollDelay() time.Duration {
	if s.env.Focused {
		return pollEvery
	}
	return common.PollInterval(pollEvery, false)
}

// feed parses the new part of the trace; a shorter trace is parsed anew.
func (s *Screen) feed(trace []byte) {
	if len(trace) < s.consumed {
		s.p.Reset()
		s.consumed = 0
		s.folded = map[int]bool{}
		s.known = 0
		s.cursor, s.offset = 0, 0
	}
	s.p.Write(trace[s.consumed:])
	s.consumed = len(trace)
	secs := s.p.Sections()
	for ; s.known < len(secs); s.known++ {
		s.folded[s.known] = secs[s.known].Collapsed
	}
	s.rebuild()
	if s.query != "" {
		s.matches = jl.Search(s.p.Lines(), s.query)
	}
	if s.follow {
		s.toEnd()
	}
	s.clamp()
}

// ---- visibility ----

func (s *Screen) hiddenBySection(sec int, includeSelf bool) bool {
	secs := s.p.Sections()
	if !includeSelf && sec >= 0 {
		sec = secs[sec].Parent
	}
	for ; sec >= 0; sec = secs[sec].Parent {
		if s.folded[sec] {
			return true
		}
	}
	return false
}

func (s *Screen) rebuild() {
	s.vis = s.vis[:0]
	for i, l := range s.p.Lines() {
		if l.Section >= 0 && s.hiddenBySection(l.Section, !l.Header) {
			continue
		}
		s.vis = append(s.vis, i)
	}
}

func (s *Screen) curPos() int {
	i := sort.SearchInts(s.vis, s.cursor)
	if i >= len(s.vis) {
		return max(0, len(s.vis)-1)
	}
	return i
}

func (s *Screen) setPos(pos int) {
	if len(s.vis) == 0 {
		s.cursor = 0
		return
	}
	pos = max(0, min(pos, len(s.vis)-1))
	s.cursor = s.vis[pos]
}

func (s *Screen) toEnd() { s.setPos(len(s.vis) - 1) }

// searchBarShown reports whether the search strip (rule + line) is drawn
// above the footer.
func (s *Screen) searchBarShown() bool { return s.searching || s.query != "" }

func (s *Screen) bodyHeight() int {
	h := s.h - 2 // header and its rule
	if s.searchBarShown() {
		h -= 2
	}
	if s.p.Dropped > 0 {
		h--
	}
	return max(1, h)
}

func (s *Screen) clamp() {
	pos := s.curPos()
	body := s.bodyHeight()
	if pos < s.offset {
		s.offset = pos
	}
	if pos >= s.offset+body {
		s.offset = pos - body + 1
	}
	s.offset = max(0, min(s.offset, max(0, len(s.vis)-body)))
}

// ---- update ----

func (s *Screen) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	env := s.env
	switch msg := msg.(type) {
	case app.ActivatedMsg:
		s.active = true
		if !s.loaded {
			return s, s.load()
		}
		if s.polling() && !env.AuthLost {
			return s, s.load()
		}
		return s, nil
	case app.DeactivatedMsg:
		s.active = false
		s.poll.Next()

	case logMsg:
		if !s.gen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			return s, tea.Batch(env.Fail(msg.err), s.schedulePoll())
		}
		s.job, s.loaded, s.updated = msg.job, true, env.Now()
		s.job.Attempts = nil
		s.jobs[s.sel] = s.job
		s.feed(msg.trace)
		return s, s.schedulePoll()

	case actMsg:
		if !s.active {
			return s, nil
		}
		if env.Cache != nil {
			env.Cache.Invalidate(fmt.Sprintf("pipe:%s:%d", s.ref.Key(), s.pipelineID))
		}
		if msg.err != nil {
			return s, env.Fail(msg.err)
		}
		m := env.Msg.Pipe
		if msg.retry {
			// The new job becomes the latest attempt and its log is shown.
			nj := msg.job
			nj.Attempts = nil
			s.jobs = append([]gitlab.Job{nj}, s.jobs...)
			cmd := s.show(0)
			s.follow = true
			return s, tea.Batch(app.Toast(common.ToastSuccess, m.JobRetried(msg.job.Name), ""), cmd)
		}
		return s, tea.Batch(app.Toast(common.ToastSuccess, m.JobCanceled(msg.job.Name), ""), s.load())

	case common.PollTickMsg:
		if !s.poll.Valid(msg.Token) || !s.active || env.AuthLost || !s.polling() {
			return s, nil
		}
		return s, s.load()
	case app.AuthLostMsg:
		s.poll.Next()
	case app.AuthRestoredMsg:
		if s.active {
			return s, s.load()
		}
	case app.FocusChangedMsg:
		if msg.Focused && s.active && s.loaded && s.polling() {
			return s, s.load()
		}
	case tea.KeyPressMsg:
		return s, s.onKey(msg)
	}
	return s, nil
}

func (s *Screen) onKey(k tea.KeyPressMsg) tea.Cmd {
	env := s.env
	if s.searching {
		return s.searchKey(k)
	}
	body := s.bodyHeight()
	key := k.String()
	switch key {
	case "j", "down":
		s.move(1)
	case "k", "up":
		s.follow = false
		s.move(-1)
	case "pgdown", "ctrl+d":
		s.move(body - 1)
	case "pgup", "ctrl+u":
		s.follow = false
		s.move(-(body - 1))
	case "g", "home":
		s.follow = false
		s.setPos(0)
	case "G", "end", "f":
		s.follow = true
		s.toEnd()
	case "left", "h":
		s.xoff = max(0, s.xoff-hScroll)
	case "right", "l":
		s.xoff += hScroll
	case "space":
		s.toggleFold()
	case "z":
		s.toggleAll()
	case "/":
		s.searching = true
		s.search.SetValue(s.query)
		s.search.CursorEnd()
		s.clamp()
		return s.search.Focus()
	case "n":
		s.step(1)
	case "N":
		s.step(-1)
	case "esc":
		if s.query != "" {
			s.query, s.matches = "", nil
			s.clamp()
			return nil
		}
	case "o":
		return app.OpenURL(env, s.job.WebURL)
	case "a":
		if len(s.jobs) > 1 {
			return app.OpenOverlay(NewAttemptsList(env, s.jobs, s.sel, func(i int) tea.Cmd {
				if i == s.sel {
					return nil
				}
				return s.show(i)
			}))
		}
	case "[":
		if s.sel < len(s.jobs)-1 {
			return s.show(s.sel + 1)
		}
	case "]":
		if s.sel > 0 {
			return s.show(s.sel - 1)
		}
	case "R":
		return s.retry()
	case "c":
		m := env.Msg.Pipe
		return app.OpenOverlay(app.NewConfirm(env, m.CancelJobTitle(s.target().Name), m.CancelJobBody, s.cancel()))
	}
	s.clamp()
	return nil
}

// show switches the screen to attempt i: the log is read anew with the
// search, folds and scroll reset; only the latest attempt follows and polls.
func (s *Screen) show(i int) tea.Cmd {
	s.sel, s.job = i, s.jobs[i]
	s.p.Reset()
	s.consumed, s.known, s.cursor, s.offset, s.xoff = 0, 0, 0, 0, 0
	s.folded = map[int]bool{}
	s.matches, s.query = nil, ""
	s.loaded = false
	s.follow = s.polling()
	s.rebuild()
	s.poll.Next()
	return s.load()
}

func (s *Screen) move(d int) {
	s.setPos(s.curPos() + d)
	if d > 0 && s.curPos() >= len(s.vis)-1 && s.polling() {
		s.follow = true // reached the end of a live log
	}
}

// toggleFold folds or unfolds the section under the cursor.
func (s *Screen) toggleFold() {
	lines := s.p.Lines()
	if len(lines) == 0 || s.cursor >= len(lines) {
		return
	}
	sec := lines[s.cursor].Section
	if sec < 0 {
		return
	}
	s.folded[sec] = !s.folded[sec]
	s.rebuild()
	if s.folded[sec] {
		s.cursor = max(s.p.Sections()[sec].FirstLine, 0)
		if i := sort.SearchInts(s.vis, s.cursor); i >= len(s.vis) || s.vis[i] != s.cursor {
			s.setPos(i)
		}
	}
}

func (s *Screen) toggleAll() {
	anyOpen := false
	for i := range s.p.Sections() {
		if !s.folded[i] {
			anyOpen = true
		}
	}
	for i := range s.p.Sections() {
		s.folded[i] = anyOpen
	}
	s.rebuild()
	if anyOpen {
		s.cursor = 0
		s.setPos(0)
	}
}

// reveal unfolds the sections containing line.
func (s *Screen) reveal(line int) {
	lines := s.p.Lines()
	if line < 0 || line >= len(lines) {
		return
	}
	for sec := lines[line].Section; sec >= 0; sec = s.p.Sections()[sec].Parent {
		if lines[line].Header && sec == lines[line].Section {
			continue
		}
		s.folded[sec] = false
	}
	s.rebuild()
}

func (s *Screen) searchKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		s.searching = false
		s.search.Blur()
		s.clamp()
		return nil
	case "enter":
		s.searching = false
		s.search.Blur()
		s.query = strings.TrimSpace(s.search.Value())
		s.matches = jl.Search(s.p.Lines(), s.query)
		s.cur = -1
		s.follow = false
		s.step(1)
		s.clamp()
		return nil
	}
	var cmd tea.Cmd
	s.search, cmd = s.search.Update(k)
	return cmd
}

// step moves to the next (d>0) or previous match from the cursor.
func (s *Screen) step(d int) {
	if len(s.matches) == 0 {
		return
	}
	s.follow = false
	if s.cur < 0 {
		// first jump: nearest match at or after the cursor
		s.cur = 0
		for i, m := range s.matches {
			if m.Line >= s.cursor {
				s.cur = i
				break
			}
		}
	} else {
		s.cur = (s.cur + d + len(s.matches)) % len(s.matches)
	}
	line := s.matches[s.cur].Line
	s.reveal(line)
	s.cursor = line
	s.clamp()
}

func (s *Screen) retry() tea.Cmd {
	client, ref, job := s.env.Client, s.ref, s.target()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		nj, err := client.Jobs().Retry(ctx, ref, job.ID)
		return actMsg{retry: true, job: nj, err: err}
	}
}

func (s *Screen) cancel() tea.Cmd {
	client, ref, job := s.env.Client, s.ref, s.target()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return actMsg{job: job, err: client.Jobs().Cancel(ctx, ref, job.ID)}
	}
}

// ---- view ----

func (s *Screen) View() string {
	env := s.env
	st, m := env.Styles, env.Msg
	j := s.job
	ctx := fmt.Sprintf("#%d › %s ", s.pipelineID, j.Name) + st.Status(j.Status)
	if j.Duration > 0 {
		ctx += " " + render.Duration(j.Duration)
	}
	if n := len(s.jobs); n > 1 {
		ctx += st.Muted.Render(" · " + m.Log.Attempt(n-s.sel, n))
	}
	follow := m.Log.NoFollow
	if s.follow {
		follow = st.Success.Render(m.Log.Follow)
	}
	total := len(s.p.Lines()) + s.p.Dropped
	right := follow + " · " + m.Plural.Lines(total)
	if !s.updated.IsZero() {
		right += " · " + m.Log.Updated(render.Ago(env.Lang, env.Now(), s.updated))
	}
	out := []string{common.Header(st, ctx, right, s.w), common.Rule(st, s.w)}
	if s.p.Dropped > 0 {
		out = append(out, st.Warning.Render(m.Log.Truncated))
	}
	out = append(out, common.FitHeight(s.body(), s.bodyHeight()))
	if s.searchBarShown() {
		out = append(out, common.Rule(st, s.w), s.searchBar())
	}
	return strings.Join(out, "\n")
}

// StatusHint is the footer status: the log is polled every 3s while running.
func (s *Screen) StatusHint() string {
	if s.polling() {
		return s.env.Msg.Project.StatusPoll(s.pollDelay().Round(time.Second).String())
	}
	return ""
}

// searchBar is the strip above the footer: the query on the left, the match
// counter and keys on the right.
func (s *Screen) searchBar() string {
	st, m := s.env.Styles, s.env.Msg.Log
	left := s.search.View()
	if !s.searching {
		left = st.Accent.Render("/") + " " + s.query
	}
	right := st.Muted.Render(" · " + m.SearchKeys)
	if s.query != "" {
		right = s.matchStatus() + right
	}
	gap := max(1, s.w-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func (s *Screen) matchStatus() string {
	m := s.env.Msg.Log
	if len(s.matches) == 0 {
		return s.env.Styles.Warning.Render(m.NoMatches)
	}
	i := max(s.cur, 0)
	return s.env.Styles.Accent.Render(m.MatchOf(i+1, len(s.matches)))
}

func (s *Screen) body() string {
	st, m := s.env.Styles, s.env.Msg
	lines := s.p.Lines()
	partial := s.p.Partial()
	if len(lines) == 0 && partial == nil {
		if s.loaded {
			return st.Muted.Render(m.Log.Empty(string(s.job.Status)))
		}
		return st.Muted.Render(m.Project.Loading)
	}
	body := s.bodyHeight()
	gutter := len(fmt.Sprint(len(lines)+s.p.Dropped+1)) + 1
	textW := max(1, s.w-gutter-2)
	matchLine := map[int]bool{}
	for _, mt := range s.matches {
		matchLine[mt.Line] = true
	}
	curMatch := -1
	if s.cur >= 0 && s.cur < len(s.matches) {
		curMatch = s.matches[s.cur].Line
	}

	var out []string
	for pos := s.offset; pos < len(s.vis) && len(out) < body; pos++ {
		i := s.vis[pos]
		out = append(out, s.row(i, lines[i], gutter, textW, matchLine[i], i == curMatch))
	}
	if partial != nil && len(out) < body && s.offset+len(out) >= len(s.vis) {
		num := st.LogLineNo.Render(fmt.Sprintf("%*d", gutter-1, len(lines)+s.p.Dropped+1))
		out = append(out, "  "+num+" "+ansi.Cut(partial.Text, s.xoff, s.xoff+textW))
	}
	return strings.Join(out, "\n")
}

func (s *Screen) row(i int, l jl.Line, gutter, textW int, match, current bool) string {
	st := s.env.Styles
	marker := " "
	if i == s.cursor {
		marker = "›"
	}
	numStr := fmt.Sprintf("%*d", gutter-1, i+s.p.Dropped+1)
	text := l.Text
	if l.Header {
		text = s.headerText(l, textW)
	}
	if match {
		text = highlight(l.Plain, s.query, st, current)
		if l.Header {
			text = s.headerPrefix(l) + text
		}
	}
	cut := ansi.Cut(text, s.xoff, s.xoff+textW)
	if i == s.cursor {
		// The cursor row is drawn plain so the reverse video is not broken
		// by the colors inside the line.
		plain := ansi.Strip(cut)
		pad := max(0, textW-ansi.StringWidth(plain))
		return st.RowSelected.Render(marker + numStr + " " + plain + strings.Repeat(" ", pad))
	}
	return st.Accent.Render(marker) + st.LogLineNo.Render(numStr) + " " + cut
}

func (s *Screen) headerPrefix(l jl.Line) string {
	mark := "▾ "
	if s.folded[l.Section] {
		mark = "▸ "
	}
	return strings.Repeat("  ", s.depth(l.Section)) + s.env.Styles.LogSection.Render(mark)
}

func (s *Screen) headerText(l jl.Line, width int) string {
	st := s.env.Styles
	sec := s.p.Sections()[l.Section]
	text := s.headerPrefix(l) + st.LogSection.Render(l.Plain)
	if d := sec.Duration(); d > 0 || !sec.Open() {
		dur := render.Duration(max(d, time.Second))
		gap := max(2, width-ansi.StringWidth(text)-len(dur))
		text += strings.Repeat(" ", gap) + st.Muted.Render(dur)
	}
	return text
}

func (s *Screen) depth(sec int) int {
	d := 0
	for p := s.p.Sections()[sec].Parent; p >= 0; p = s.p.Sections()[p].Parent {
		d++
	}
	return d
}

// highlight marks every occurrence of query in plain text (case-insensitive).
func highlight(plain, query string, st *common.Styles, current bool) string {
	if query == "" {
		return plain
	}
	style := st.LogMatch
	if current {
		style = st.LogMatchCurrent
	}
	low, q := strings.ToLower(plain), strings.ToLower(query)
	var b strings.Builder
	for {
		i := strings.Index(low, q)
		if i < 0 {
			b.WriteString(plain)
			break
		}
		b.WriteString(plain[:i])
		b.WriteString(style.Render(plain[i : i+len(q)]))
		plain, low = plain[i+len(q):], low[i+len(q):]
	}
	return b.String()
}
