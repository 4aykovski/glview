// Package todos is the To-Do screen: the current user's to-do items of the
// whole instance, with filters, a preview and the actions on them (SPEC
// §3.17).
package todos

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/issuedetail"
	"github.com/4aykovski/glview/internal/ui/screens/mrdetail"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	requestTimeout = 15 * time.Second
	listTTL        = 30 * time.Second
	cachePrefix    = "todos:"
	nearEnd        = 5
	previewMinW    = 100 // below this width the preview is hidden
	previewRightW  = 140 // the preview goes right of the list from this width
	listShare      = 55  // percent of the width for the list with the preview on the right
	belowMin       = 7   // lines of the preview below the list, frame included
	belowMax       = 11  // title, two lines, the longest body and the frame
	bodyLines      = 6   // the longest to-do body in the preview
	headerLines    = 2   // header and its rule
)

type (
	loadedMsg struct {
		tok     common.Token
		cursor  string
		refresh bool
		page    gitlab.Page[gitlab.Todo]
		err     error
	}
	doneMsg struct {
		owner common.Token
		id    int64
		err   error
	}
	allDoneMsg struct {
		owner common.Token
		err   error
	}
)

// undo remembers what a row marked done looked like, to put it back when
// GitLab refuses.
type undo struct {
	todo      gitlab.Todo
	at        int
	prevCount int
	hadCount  bool
}

type mdKey struct {
	s    *common.Styles
	w    int
	text string
}

// Screen is the To-Do screen.
type Screen struct {
	env  *app.Env
	gen  common.Gen
	poll common.Gen
	tbl  *table.Model

	filter gitlab.TodoFilter
	cancel context.CancelFunc

	items    []gitlab.Todo
	firstLen int
	next     string
	total    int

	// hidden are the to-dos marked done whose request is still in flight: a
	// list that arrives meanwhile must not bring them back.
	hidden map[int64]bool
	undo   map[int64]undo

	md map[mdKey]string

	loaded, loading, moreLoading, moreErr bool
	err                                   error
	updated                               time.Time
	active                                bool
	w, h                                  int
}

var _ app.TodoScreen = (*Screen)(nil)

// New creates the screen; it loads when it becomes the top of the stack.
func New(env *app.Env) *Screen {
	s := &Screen{env: env, gen: common.NewGen(), poll: common.NewGen(), total: -1,
		filter: defaultFilter(), hidden: map[int64]bool{}, undo: map[int64]undo{}, md: map[mdKey]string{}}
	s.tbl = table.New(env.Styles)
	s.rebuild()
	return s
}

func defaultFilter() gitlab.TodoFilter { return gitlab.TodoFilter{State: "pending"} }

// IsTodoScreen marks the screen for the root (T).
func (s *Screen) IsTodoScreen() {}

func (s *Screen) Init() tea.Cmd { return nil }

func (s *Screen) Title() string { return s.env.Msg.Todo.Crumb + " · " + s.host() }

func (s *Screen) host() string {
	return strings.TrimPrefix(strings.TrimPrefix(s.env.Host, "https://"), "http://")
}

// Items returns the loaded to-dos (for tests).
func (s *Screen) Items() []gitlab.Todo { return s.items }

// Filter returns the current filter (for tests).
func (s *Screen) Filter() gitlab.TodoFilter { return s.filter }

func (s *Screen) SetSize(w, h int) {
	s.w, s.h = w, h
	s.layout()
}

// StatusHint is "5 / 12": the cursor position and the number of to-dos.
func (s *Screen) StatusHint() string {
	if len(s.items) == 0 {
		return ""
	}
	return fmt.Sprintf("%d / %d", s.tbl.Cursor()+1, max(s.total, len(s.items)))
}

// ---- layout ----

func (s *Screen) bodyHeight() int { return max(1, s.h-headerLines) }

func (s *Screen) previewRight() bool { return s.w >= previewRightW && s.bodyHeight() >= 8 }

func (s *Screen) previewBelow() bool {
	return !s.previewRight() && s.w >= previewMinW && s.bodyHeight() >= belowMin+1+8
}

func (s *Screen) listWidth() int {
	if s.previewRight() {
		return s.w*listShare/100 - 1
	}
	return s.w
}

// belowHeight is the height of the preview under the list, frame included:
// what the rows leave, but the list keeps up to half and the preview needs
// no more than belowMax.
func (s *Screen) belowHeight() int {
	body := s.bodyHeight() - 1
	list := min(len(s.items)+3, body/2)
	return min(belowMax, max(belowMin, body-max(list, 5)))
}

func (s *Screen) layout() {
	body := s.bodyHeight() - 1
	if s.previewBelow() {
		body -= s.belowHeight()
	}
	s.tbl.SetSize(s.listWidth(), max(1, body))
}

// ---- rows ----

// ref is the target in a row: !479, #1318, the SHA of a commit, else "—".
func ref(t gitlab.Todo) string {
	switch {
	case t.TargetType == "MergeRequest" && t.TargetIID > 0:
		return fmt.Sprintf("!%d", t.TargetIID)
	case t.TargetType == "Issue" && t.TargetIID > 0:
		return fmt.Sprintf("#%d", t.TargetIID)
	case t.TargetType == "Commit" && t.TargetRef != "":
		return t.TargetRef
	}
	return "—"
}

func (s *Screen) rebuild() {
	env := s.env
	st, m := env.Styles, env.Msg.Todo
	s.tbl.SetStyles(st)
	s.tbl.SetColumns([]table.Column{
		{Title: m.ColTarget, Min: 6, Priority: 100},
		{Title: m.ColTitle, Min: 20, Flex: true, Priority: 90},
		{Title: m.ColAction, Min: 8, Max: 22, Priority: 80},
		{Title: m.ColAgo, Min: 4, Priority: 70, AlignRight: true},
		{Title: m.ColAuthor, Min: 6, Max: 16, Priority: 60},
		{Title: m.ColProject, Min: 12, Max: 36, Priority: 50},
	})
	rows := make([]table.Row, len(s.items))
	for i, t := range s.items {
		project := "—"
		if t.Project != nil && t.Project.FullPath != "" {
			project = t.Project.FullPath
		}
		author := t.Author.Username
		if author == "" {
			author = "—"
		}
		rows[i] = table.Row{ID: strconv.FormatInt(t.ID, 10), Cells: []string{
			st.Accent.Render(ref(t)), t.TargetTitle, m.Action(t.Action), render.Ago(env.Lang, env.Now(), t.CreatedAt), author, project,
		}}
	}
	s.tbl.SetRows(rows)
	s.tbl.SetEmpty(s.emptyText())
	s.setTail()
	s.layout()
}

func (s *Screen) setTail() {
	m := s.env.Msg.Project
	switch {
	case s.err != nil, s.moreErr:
		s.tbl.SetTail(table.TailError, m.LoadFailed)
	case s.loading && len(s.items) == 0:
		s.tbl.SetTail(table.TailLoading, m.Loading)
	case s.moreLoading:
		s.tbl.SetTail(table.TailLoading, m.LoadingMore)
	default:
		s.tbl.SetTail(table.TailNone, "")
	}
}

func (s *Screen) emptyText() string {
	m := s.env.Msg.Todo
	switch {
	case s.filter.Type != "":
		return "\n  " + m.EmptyFiltered
	case s.filter.State == "done":
		return "\n  " + m.EmptyDone
	}
	return "\n  " + m.EmptyPending
}

func (s *Screen) selected() *gitlab.Todo {
	id := s.tbl.CursorID()
	for i := range s.items {
		if strconv.FormatInt(s.items[i].ID, 10) == id {
			return &s.items[i]
		}
	}
	return nil
}

func (s *Screen) pending() bool { return s.filter.State != "done" }

// ---- loading ----

func (s *Screen) key(cursor string) string {
	return fmt.Sprintf("%s%s:%s:%s", cachePrefix, s.filter.State, s.filter.Type, cursor)
}

func (s *Screen) fetch(tok common.Token, cursor string, force, refresh bool) tea.Cmd {
	env := s.env
	client, f, limit, key := env.Client, s.filter, env.Settings.PageSize, s.key(cursor)
	if s.cancel != nil && cursor == "" {
		s.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	if cursor == "" {
		s.cancel = cancel
	}
	return func() tea.Msg {
		defer cancel()
		page, _, err := common.Fetch(ctx, env.Cache, key, listTTL, force, func(ctx context.Context) (gitlab.Page[gitlab.Todo], error) {
			return client.Todos().List(ctx, f, cursor, limit)
		})
		return loadedMsg{tok: tok, cursor: cursor, refresh: refresh, page: page, err: err}
	}
}

// loadFirst requests the first page. reset drops the rows (filter change);
// otherwise the result is merged into the loaded rows.
func (s *Screen) loadFirst(reset, force bool) tea.Cmd {
	tok := s.gen.Next()
	s.moreLoading, s.moreErr, s.err = false, false, nil
	if reset {
		s.items, s.firstLen, s.next, s.total, s.loaded = nil, 0, "", -1, false
		s.loading = true
		s.rebuild()
	} else {
		s.loading = !s.loaded
		s.setTail()
	}
	return s.fetch(tok, "", force, !reset)
}

func (s *Screen) loadMore() tea.Cmd {
	if s.next == "" || s.moreLoading || s.loading {
		return nil
	}
	s.moreLoading, s.moreErr = true, false
	s.setTail()
	return s.fetch(s.gen.Current(), s.next, false, false)
}

func (s *Screen) refresh() tea.Cmd {
	if s.moreErr && len(s.items) > 0 {
		s.moreLoading, s.moreErr = true, false
		s.setTail()
		return s.fetch(s.gen.Current(), s.next, true, false)
	}
	return s.loadFirst(false, true)
}

func (s *Screen) schedulePoll() tea.Cmd {
	if !s.active || s.env.AuthLost {
		s.poll.Next()
		return nil
	}
	return common.PollAfter(s.env.PollInterval(false), s.poll.Next())
}

// ---- update ----

func (s *Screen) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	env := s.env
	switch msg := msg.(type) {
	case app.ActivatedMsg:
		s.active = true
		switch {
		case env.AuthLost:
			return s, nil
		case !s.loaded && !s.loading:
			return s, s.loadFirst(true, false)
		case s.loaded:
			// Coming back (or T on this screen) refreshes the list.
			return s, s.loadFirst(false, true)
		}
		return s, s.schedulePoll()
	case app.DeactivatedMsg:
		s.active = false
		s.poll.Next()

	case loadedMsg:
		return s, s.onLoaded(msg)
	case doneMsg:
		return s, s.onDone(msg)
	case allDoneMsg:
		return s, s.onAllDone(msg)

	case common.PollTickMsg:
		if !s.poll.Valid(msg.Token) || !s.active || env.AuthLost {
			return s, nil
		}
		return s, s.loadFirst(false, true)
	case app.CacheRevalidatedMsg:
		if strings.HasPrefix(msg.Key, cachePrefix) && s.loaded {
			return s, s.loadFirst(false, false)
		}
	case app.AuthLostMsg:
		s.poll.Next()
	case app.AuthRestoredMsg:
		if s.active {
			return s, s.loadFirst(false, true)
		}
	case app.FocusChangedMsg:
		if msg.Focused && s.active && s.loaded && env.Now().Sub(s.updated) >= env.PollInterval(false) {
			return s, s.loadFirst(false, true)
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		s.md = map[mdKey]string{}
		s.rebuild()
	case tea.KeyPressMsg:
		return s, s.onKey(msg)
	}
	return s, nil
}

func (s *Screen) onLoaded(msg loadedMsg) tea.Cmd {
	if !s.gen.Valid(msg.tok) {
		return nil
	}
	env := s.env
	if msg.err != nil {
		if msg.cursor != "" {
			s.moreLoading, s.moreErr = false, true
			s.setTail()
			return env.Fail(msg.err)
		}
		s.loading = false
		if len(s.items) == 0 {
			s.err = msg.err
		}
		s.setTail()
		return tea.Batch(env.Fail(msg.err), s.schedulePoll())
	}
	page := msg.page
	page.Items = slices.DeleteFunc(slices.Clone(page.Items), func(t gitlab.Todo) bool { return s.hidden[t.ID] })
	switch {
	case msg.cursor != "":
		s.moreLoading = false
		s.items = appendUnique(s.items, page.Items)
		s.next = page.NextCursor
	case msg.refresh && s.loaded && len(s.items) > s.firstLen:
		tail := s.items[s.firstLen:]
		s.items = appendUnique(slices.Clone(page.Items), tail)
		s.firstLen = len(page.Items)
		// s.next stays: it continues after the tail that is already loaded.
	default:
		s.items = page.Items
		s.firstLen = len(page.Items)
		s.next = page.NextCursor
	}
	var count tea.Cmd
	if msg.cursor == "" {
		s.total = page.Total
		s.loaded, s.loading = true, false
		s.updated = env.Now()
		if s.filter.State == "pending" && s.filter.Type == "" {
			count = app.TodoCount(page.Total)
		}
	}
	s.rebuild()
	cmds := []tea.Cmd{s.schedulePoll(), count}
	if s.tbl.NearEnd(nearEnd) {
		cmds = append(cmds, s.loadMore())
	}
	return tea.Batch(cmds...)
}

func appendUnique(dst, src []gitlab.Todo) []gitlab.Todo {
	seen := make(map[int64]bool, len(dst))
	for _, t := range dst {
		seen[t.ID] = true
	}
	for _, t := range src {
		if !seen[t.ID] {
			dst = append(dst, t)
		}
	}
	return dst
}

// ---- actions ----

// markDone takes the selected to-do out of the list at once and asks
// GitLab; an error brings it back (SPEC §3.17).
func (s *Screen) markDone() tea.Cmd {
	t := s.selected()
	if t == nil || !s.pending() {
		return nil
	}
	env := s.env
	todo := *t
	i := slices.IndexFunc(s.items, func(x gitlab.Todo) bool { return x.ID == todo.ID })
	s.items = slices.Delete(s.items, i, i+1)
	if i < s.firstLen {
		s.firstLen--
	}
	if s.total > 0 {
		s.total--
	}
	s.hidden[todo.ID] = true
	s.undo[todo.ID] = undo{todo: todo, at: i, prevCount: env.TodoCount, hadCount: env.TodoKnown}
	s.rebuild()

	// The counter goes down before the request starts: a refusal puts it
	// back after that, never before.
	req := s.request(todo.ID)
	if env.TodoKnown && env.TodoCount > 0 {
		req = tea.Sequence(app.TodoCount(env.TodoCount-1), req)
	}
	cmds := []tea.Cmd{req}
	if s.tbl.NearEnd(nearEnd) {
		cmds = append(cmds, s.loadMore())
	}
	return tea.Batch(cmds...)
}

func (s *Screen) request(id int64) tea.Cmd {
	client, owner := s.env.Client, s.gen.Current()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return doneMsg{owner: owner, id: id, err: client.Todos().MarkDone(ctx, id)}
	}
}

func (s *Screen) onDone(msg doneMsg) tea.Cmd {
	if !s.gen.Owns(msg.owner) {
		return nil
	}
	env := s.env
	u, ok := s.undo[msg.id]
	delete(s.undo, msg.id)
	delete(s.hidden, msg.id)
	if env.Cache != nil {
		env.Cache.Invalidate(cachePrefix)
	}
	if msg.err == nil {
		label := ref(u.todo)
		if label == "—" {
			label = u.todo.TargetTitle
		}
		return app.Toast(common.ToastSuccess, env.Msg.Todo.Done(label), "")
	}
	cmds := []tea.Cmd{env.Fail(msg.err)}
	if ok {
		s.restore(u)
		if u.hadCount {
			cmds = append(cmds, app.TodoCount(u.prevCount))
		}
	}
	return tea.Batch(cmds...)
}

// restore puts a row refused by GitLab back where it was.
func (s *Screen) restore(u undo) {
	at := min(u.at, len(s.items))
	s.items = slices.Insert(s.items, at, u.todo)
	if at <= s.firstLen {
		s.firstLen++
	}
	if s.total >= 0 {
		s.total++
	}
	s.rebuild()
	s.tbl.SetCursorID(strconv.FormatInt(u.todo.ID, 10))
}

// markAllDone asks for a confirm and marks every pending to-do done.
func (s *Screen) markAllDone() tea.Cmd {
	if !s.pending() {
		return nil
	}
	env := s.env
	n := s.total
	if env.TodoKnown {
		n = env.TodoCount
	}
	client, owner := env.Client, s.gen.Current()
	do := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return allDoneMsg{owner: owner, err: client.Todos().MarkAllDone(ctx)}
	}
	m := env.Msg.Todo
	return app.OpenOverlay(app.NewConfirm(env, m.ConfirmAll, m.ConfirmBody(n), do))
}

func (s *Screen) onAllDone(msg allDoneMsg) tea.Cmd {
	if !s.gen.Owns(msg.owner) {
		return nil
	}
	env := s.env
	if env.Cache != nil {
		env.Cache.Invalidate(cachePrefix)
	}
	if msg.err != nil {
		return env.Fail(msg.err)
	}
	return tea.Batch(
		app.TodoCount(0),
		app.Toast(common.ToastSuccess, env.Msg.Todo.Cleared, ""),
		s.loadFirst(true, true),
	)
}

// open opens the target: the MR or issue screen of the project of the
// to-do, the browser for everything else. The to-do stays pending.
func (s *Screen) open(t gitlab.Todo) tea.Cmd {
	env := s.env
	if t.Project != nil && t.TargetIID > 0 {
		switch t.TargetType {
		case "MergeRequest":
			mr := gitlab.MergeRequest{Project: *t.Project, IID: t.TargetIID, Title: t.TargetTitle, State: gitlab.MRState(t.TargetState), WebURL: t.TargetURL}
			return app.Push(mrdetail.New(env, *t.Project, nil, mr, false))
		case "Issue":
			is := gitlab.Issue{Project: *t.Project, IID: t.TargetIID, Title: t.TargetTitle, State: t.TargetState, WebURL: t.TargetURL}
			return app.Push(issuedetail.New(env, *t.Project, is, false))
		}
	}
	if t.TargetURL != "" {
		return app.OpenURL(env, t.TargetURL)
	}
	return nil
}

// pageURL is the To-Do page of the web UI with the filters; defaults are
// left out.
func (s *Screen) pageURL() string {
	q := url.Values{}
	if s.filter.State == "done" {
		q.Set("state", "done")
	}
	if s.filter.Type != "" {
		q.Set("type", s.filter.Type)
	}
	u := strings.TrimRight(s.env.Host, "/") + "/dashboard/todos"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (s *Screen) onKey(k tea.KeyPressMsg) tea.Cmd {
	env := s.env
	if s.tbl.HandleKey(k) {
		if s.tbl.NearEnd(nearEnd) {
			return s.loadMore()
		}
		return nil
	}
	switch k.String() {
	case "r":
		return s.refresh()
	case "s":
		if s.filter.State == "done" {
			s.filter.State = "pending"
		} else {
			s.filter.State = "done"
		}
		return s.loadFirst(true, false)
	case "t":
		s.filter.Type = map[string]string{"": "MergeRequest", "MergeRequest": "Issue", "Issue": ""}[s.filter.Type]
		return s.loadFirst(true, false)
	case "backspace":
		s.filter = defaultFilter()
		return s.loadFirst(true, false)
	case "o":
		return app.OpenURL(env, s.pageURL())
	case "X":
		return s.markAllDone()
	}
	t := s.selected()
	if t == nil {
		return nil
	}
	switch k.String() {
	case "enter":
		return s.open(*t)
	case "x":
		return s.markDone()
	case "O":
		if t.TargetURL != "" {
			return app.OpenURL(env, t.TargetURL)
		}
	case "y":
		if t.TargetURL != "" {
			return app.Copy(env, t.TargetURL, render.ShortURL(t.TargetURL))
		}
	}
	return nil
}

// ---- view ----

func typeLabel(t string) string {
	switch t {
	case "MergeRequest":
		return "MR"
	case "Issue":
		return "issue"
	}
	return "any"
}

func (s *Screen) filterLine() string {
	st, m := s.env.Styles, s.env.Msg
	pairs := [][2]string{{"state", s.filter.State}, {"type", typeLabel(s.filter.Type)}}
	parts := make([]string, len(pairs))
	for i, kv := range pairs {
		parts[i] = st.FilterKey.Render(kv[0]+":") + st.FilterValue.Render(kv[1])
	}
	right := ""
	if s.loaded {
		right = m.Project.Counter(len(s.items), s.total, false)
	}
	return common.FilterBarText(st, strings.Join(parts, "  "), right, s.listWidth())
}

// body renders the markdown of a to-do, memoised: glamour is slow.
func (s *Screen) body(text string, w int) string {
	k := mdKey{s.env.Styles, w, text}
	if out, ok := s.md[k]; ok {
		return out
	}
	out := common.Markdown(s.env.Styles, common.HTMLToMarkdown(text), w)
	if len(s.md) > 64 {
		s.md = map[mdKey]string{}
	}
	s.md[k] = out
	return out
}

// preview renders the selected to-do into exactly h lines of at most w
// cells: title, target, reason and the body (SPEC §3.17).
func (s *Screen) preview(t gitlab.Todo, w, h int) []string {
	env := s.env
	st, m := env.Styles, env.Msg.Todo
	dot := st.Muted.Render(" · ")
	title := t.TargetTitle
	if title == "" {
		title = ref(t)
	}
	var where []string
	if r := ref(t); r != "—" {
		where = append(where, st.Accent.Render(r))
	}
	if t.Project != nil && t.Project.FullPath != "" {
		where = append(where, t.Project.FullPath)
	}
	if t.TargetState != "" {
		where = append(where, st.Muted.Render(t.TargetState))
	}
	where = append(where, st.Muted.Render(render.Ago(env.Lang, env.Now(), t.CreatedAt)))
	why := m.Action(t.Action)
	if t.Author.Username != "" {
		why += st.Muted.Render(" · ") + t.Author.Username
	}
	out := []string{st.Bold.Render(title), strings.Join(where, dot), why}
	if strings.TrimSpace(t.Body) != "" {
		body := strings.Split(s.body(t.Body, w), "\n")
		if len(body) > bodyLines {
			body = append(body[:bodyLines-1:bodyLines-1], st.Muted.Render("…"))
		}
		out = append(out, body...)
	}
	for len(out) < h {
		out = append(out, "")
	}
	out = out[:h]
	for i := range out {
		out[i] = render.Clip(out[i], w)
	}
	return out
}

func (s *Screen) View() string {
	env := s.env
	st := env.Styles
	crumb := st.Bold.Render(env.Msg.Todo.Crumb) + " " + st.Muted.Render("· "+s.host())
	head := common.Header(st, crumb, env.HeaderRight(s.updated), s.w) + "\n" + common.Rule(st, s.w)
	left := s.filterLine() + "\n" + s.tbl.View()
	switch {
	case s.previewRight():
		return head + "\n" + s.viewRight(left)
	case s.previewBelow() && s.selected() != nil:
		return head + "\n" + left + "\n" + s.viewBelow()
	}
	return head + "\n" + left
}

// viewRight puts the preview right of the list, a vertical rule between.
func (s *Screen) viewRight(left string) string {
	st := s.env.Styles
	lw := s.listWidth() + 1
	pw := s.w - lw - 1
	h := s.bodyHeight()
	var prev []string
	if t := s.selected(); t != nil {
		prev = s.preview(*t, pw-3, h)
	}
	ll := strings.Split(left, "\n")
	var b strings.Builder
	for i := range h {
		l, p := "", ""
		if i < len(ll) {
			l = ll[i]
		}
		if i < len(prev) {
			p = "  " + prev[i]
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(render.Pad(ansi.Truncate(l, lw, ""), lw) + st.Muted.Render("│") + p)
	}
	return b.String()
}

// viewBelow draws the preview in a frame under the list.
func (s *Screen) viewBelow() string {
	st := s.env.Styles
	h := s.belowHeight()
	lines := s.preview(*s.selected(), s.w-4, h-2)
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return common.Box(st, strings.Join(lines, "\n"), s.w, h, common.BoxNormal)
}

func (s *Screen) KeyMap() common.KeyMap {
	k, m := s.env.Msg.Keys, s.env.Msg.Todo
	pendingOnly := func(b common.Binding) common.Binding {
		b.Hidden = !s.pending()
		return b
	}
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", k.Open, common.PriorityHigh),
		pendingOnly(common.NewBinding([]string{"x"}, "", m.KeyDone, common.PriorityNormal)),
		pendingOnly(common.NewBinding([]string{"X"}, "", m.KeyDoneAll, common.PriorityLow)),
		common.NewBinding([]string{"s"}, "", k.State, common.PriorityNormal),
		common.NewBinding([]string{"t"}, "", m.KeyType, common.PriorityNormal),
		common.NewBinding([]string{"backspace"}, "⌫", k.Reset, common.PriorityLow),
		common.NewBinding([]string{"o"}, "", m.KeyBrowserPage, common.PriorityLow),
		common.NewBinding([]string{"O"}, "", m.KeyTarget, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
	}}
}
