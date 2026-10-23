package project

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/render"
	"github.com/4aykovski/glview/internal/ui/screens/tagactions"
)

const (
	tagPipelineWindow = 100 // newest tag pipelines matched to the tags (SPEC §5.2)
	tagsPollInterval  = 30 * time.Second
)

type (
	tagsLoadedMsg struct {
		tok     common.Token
		cursor  string
		refresh bool
		page    gitlab.Page[gitlab.Tag]
		pipes   []gitlab.PipelineSummary
		hasPipe bool // the pipelines answered (an empty answer included)
		err     error
	}
	// projectReadyMsg tells the tabs that the project is loaded: the
	// default branch and the archived flag are known.
	projectReadyMsg struct{}
)

// tagsTab is the tag list of the project (SPEC §3.4).
type tagsTab struct {
	sh   *shared
	gen  common.Gen
	poll common.Gen
	tbl  *table.Model

	search  string
	input   textinput.Model
	editing bool

	cancel context.CancelFunc
	cmp    *tagactions.Comparer

	items    []gitlab.Tag
	pipes    []gitlab.PipelineSummary
	firstLen int
	next     string
	total    int
	want     string // put the cursor on this tag once it is listed

	loaded, loading, moreLoading, moreErr bool
	err                                   error
	updated                               time.Time
	active                                bool
	w, h                                  int
}

func newTagsTab(sh *shared) *tagsTab {
	t := &tagsTab{sh: sh, gen: common.NewGen(), poll: common.NewGen(), total: -1,
		cmp: tagactions.NewComparer()}
	t.tbl = table.New(sh.env.Styles)
	t.input = common.NewInput(sh.env.Styles)
	t.rebuild()
	return t
}

func (t *tagsTab) env() *app.Env { return t.sh.env }

func (t *tagsTab) UpdatedAt() time.Time { return t.updated }

func (t *tagsTab) Count() int {
	if !t.loaded {
		return -1
	}
	return t.total
}

// Status is "6 / 58": the position of the cursor and the number of tags.
func (t *tagsTab) Status() string {
	if len(t.items) == 0 {
		return ""
	}
	total := t.total
	if total < len(t.items) {
		total = len(t.items)
	}
	return fmt.Sprintf("%d / %d", t.tbl.Cursor()+1, total)
}

func (t *tagsTab) CapturesInput() bool { return t.editing }

// Items returns the loaded tags (for tests).
func (t *tagsTab) Items() []gitlab.Tag { return t.items }

func (t *tagsTab) SetSize(w, h int) {
	t.w, t.h = w, h
	t.input.SetWidth(max(10, w-30))
	t.layout()
}

func (t *tagsTab) previewShown() bool {
	return t.w >= previewMinW && t.h >= tagactions.PreviewLines+2+8
}

func (t *tagsTab) layout() {
	body := t.h - 1
	if t.previewShown() {
		body -= tagactions.PreviewLines + 2
	}
	t.tbl.SetSize(t.w, max(1, body))
}

func (t *tagsTab) rebuild() {
	env := t.env()
	s, m, tm := env.Styles, env.Msg.MR, env.Msg.Tag
	t.tbl.SetStyles(env.Styles)
	t.tbl.SetColumns([]table.Column{
		{Title: tm.ColTag, Min: 10, Max: 28, Priority: 100},
		{Title: m.ColCommit, Min: 7, Max: 8, Priority: 60},
		{Title: tm.ColMessage, Min: 16, Flex: true, Priority: 90},
		{Title: m.ColPipeline, Min: 10, Max: 14, Priority: 70},
		{Title: m.ColAgo, Min: 4, Priority: 50, AlignRight: true},
	})
	rows := make([]table.Row, len(t.items))
	for i, tag := range t.items {
		msg := tagactions.Message(tag)
		if tag.Release != nil {
			msg += " " + s.Accent.Render(tm.ReleaseMark)
		}
		rows[i] = table.Row{ID: tag.Name, Cells: []string{
			tag.Name, s.Muted.Render(tag.Commit.ShortSHA), msg, t.pipelineCell(tag), render.Ago(env.Lang, env.Now(), tag.Commit.CreatedAt),
		}}
	}
	t.tbl.SetRows(rows)
	t.tbl.SetEmpty(t.emptyText())
	t.setTail()
	t.layout()
	if t.want != "" && t.tbl.SetCursorID(t.want) {
		t.want = ""
	}
}

func (t *tagsTab) pipelineCell(tag gitlab.Tag) string {
	s := t.env().Styles
	if tag.Pipeline == nil {
		return s.Muted.Render("—")
	}
	return s.StatusGlyph(tag.Pipeline.Status) + " " + s.Accent.Render(fmt.Sprintf("#%d", tag.Pipeline.ID))
}

func (t *tagsTab) setTail() {
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

func (t *tagsTab) emptyText() string {
	m := t.env().Msg.Tag
	if t.search != "" {
		return "\n  " + m.EmptyQuery(t.search)
	}
	return "\n  " + m.EmptyNone
}

// ---- loading ----

func (t *tagsTab) key(cursor string) string {
	return tagactions.ListKey(t.sh.ref.FullPath, t.search, cursor)
}

func (t *tagsTab) prefix() string { return tagactions.ListPrefix(t.sh.ref.FullPath) }

// fetch loads a page of tags; the first page (and a refresh) also reads the
// newest pipelines of tags. A failed pipelines answer leaves the column
// as it was and goes to the debug log.
func (t *tagsTab) fetch(tok common.Token, cursor string, force, refresh bool) tea.Cmd {
	env := t.env()
	client, ref, search, limit, key := env.Client, t.sh.ref, t.search, env.Settings.PageSize, t.key(cursor)
	if t.cancel != nil && cursor == "" {
		t.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	if cursor == "" {
		t.cancel = cancel
	}
	return func() tea.Msg {
		defer cancel()
		page, _, err := common.Fetch(ctx, env.Cache, key, listTTL, force, func(ctx context.Context) (gitlab.Page[gitlab.Tag], error) {
			return client.Tags().List(ctx, ref, search, cursor, limit)
		})
		msg := tagsLoadedMsg{tok: tok, cursor: cursor, refresh: refresh, page: page, err: err}
		if err != nil || cursor != "" {
			return msg
		}
		pipes, _, perr := common.Fetch(ctx, env.Cache, tagactions.PipesKey(ref.FullPath), listTTL, force, func(ctx context.Context) ([]gitlab.PipelineSummary, error) {
			return client.Tags().Pipelines(ctx, ref, tagPipelineWindow)
		})
		if perr != nil {
			debuglog.L().Debug("tags: pipelines failed", "project", ref.FullPath, "err", perr)
			return msg
		}
		msg.pipes, msg.hasPipe = pipes, true
		return msg
	}
}

func (t *tagsTab) loadFirst(reset, force bool) tea.Cmd {
	tok := t.gen.Next()
	t.moreLoading, t.moreErr, t.err = false, false, nil
	if force || reset {
		t.cmp.Reset()
	}
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

func (t *tagsTab) loadMore() tea.Cmd {
	if t.next == "" || t.moreLoading || t.loading {
		return nil
	}
	t.moreLoading, t.moreErr = true, false
	t.setTail()
	return t.fetch(t.gen.Current(), t.next, false, false)
}

func (t *tagsTab) Refresh() tea.Cmd {
	if t.moreErr && len(t.items) > 0 {
		t.moreLoading, t.moreErr = true, false
		t.setTail()
		return t.fetch(t.gen.Current(), t.next, true, false)
	}
	return tea.Batch(t.loadFirst(false, true), t.loadCompare())
}

func (t *tagsTab) schedulePoll() tea.Cmd {
	if !t.active || t.env().AuthLost {
		t.poll.Next()
		return nil
	}
	return common.PollAfter(t.env().PollInterval(false), t.poll.Next())
}

func (t *tagsTab) Activate() tea.Cmd {
	t.active = true
	switch {
	case !t.loaded && !t.loading:
		return t.loadFirst(true, false)
	case t.loaded && t.env().Now().Sub(t.updated) >= tagsPollInterval && !t.env().AuthLost:
		return t.loadFirst(false, true)
	}
	return t.schedulePoll()
}

func (t *tagsTab) Deactivate() {
	t.active = false
	t.poll.Next()
	t.cmp.Stop()
}

// ---- comparison with the default branch ----

func (t *tagsTab) selected() *gitlab.Tag {
	id := t.tbl.CursorID()
	for i := range t.items {
		if t.items[i].Name == id {
			return &t.items[i]
		}
	}
	return nil
}

// loadCompare asks for the comparison of the selected tag with the default
// branch, once per tag. It does not wait for the preview to fit: the size
// may arrive later.
func (t *tagsTab) loadCompare() tea.Cmd {
	tag := t.selected()
	branch := t.sh.defaultBranch
	if tag == nil || branch == "" || !t.active {
		return nil
	}
	return t.cmp.Load(t.env(), t.sh.ref, tag.Name, branch)
}

// ---- update ----

func (t *tagsTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	env := t.env()
	switch msg := msg.(type) {
	case tagsLoadedMsg:
		return t, t.onLoaded(msg)
	case tagactions.CompareMsg:
		t.cmp.OnMsg(msg)
		return t, nil
	case projectReadyMsg:
		return t, t.loadCompare()
	case tagactions.DeletedMsg:
		if msg.Ref.FullPath != t.sh.ref.FullPath {
			return t, nil
		}
		cmd := tagactions.Done(env, msg)
		if msg.Err != nil {
			return t, cmd
		}
		return t, tea.Batch(cmd, t.loadFirst(false, true))
	case tagactions.CreatedMsg:
		if msg.Ref.FullPath != t.sh.ref.FullPath {
			return t, nil
		}
		tagactions.Created(env, msg)
		t.want = msg.Tag.Name
		return t, t.loadFirst(false, true)
	case common.PollTickMsg:
		if !t.poll.Valid(msg.Token) || !t.active || env.AuthLost {
			return t, nil
		}
		return t, tea.Batch(t.loadFirst(false, true), t.loadCompare())
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
		if msg.Focused && t.active && t.loaded && env.Now().Sub(t.updated) >= tagsPollInterval {
			return t, t.loadFirst(false, true)
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		t.input.SetStyles(env.Styles.Input)
		t.rebuild()
	case tea.KeyPressMsg:
		return t, t.onKey(msg)
	}
	return t, nil
}

func (t *tagsTab) onLoaded(msg tagsLoadedMsg) tea.Cmd {
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
	if msg.hasPipe {
		t.pipes = msg.pipes
	}
	page := msg.page
	switch {
	case msg.cursor != "":
		t.moreLoading = false
		t.items = appendUniqueTags(t.items, page.Items)
		t.next = page.NextCursor
	case msg.refresh && t.loaded && len(t.items) > t.firstLen:
		tail := t.items[t.firstLen:]
		t.items = appendUniqueTags(slices.Clone(page.Items), tail)
		t.firstLen = len(page.Items)
	default:
		t.items = slices.Clone(page.Items)
		t.firstLen = len(page.Items)
		t.next = page.NextCursor
	}
	for i := range t.items {
		t.items[i].Pipeline = nil
	}
	gitlab.MatchTagPipelines(t.items, t.pipes)
	if msg.cursor == "" {
		t.total = page.Total
		t.loaded, t.loading = true, false
		t.updated = env.Now()
	}
	t.rebuild()
	cmds := []tea.Cmd{t.schedulePoll(), t.loadCompare()}
	if t.tbl.NearEnd(nearEnd) {
		cmds = append(cmds, t.loadMore())
	}
	return tea.Batch(cmds...)
}

func appendUniqueTags(dst, src []gitlab.Tag) []gitlab.Tag {
	seen := make(map[string]bool, len(dst))
	for _, tag := range dst {
		seen[tag.Name] = true
	}
	for _, tag := range src {
		if !seen[tag.Name] {
			dst = append(dst, tag)
		}
	}
	return dst
}

func (t *tagsTab) onKey(k tea.KeyPressMsg) tea.Cmd {
	env := t.env()
	if t.editing {
		switch k.String() {
		case "enter":
			t.editing = false
			t.input.Blur()
			t.search = strings.TrimSpace(t.input.Value())
			return t.loadFirst(true, false)
		case "esc":
			t.editing = false
			t.input.Blur()
			return nil
		}
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(k)
		return cmd
	}
	if t.tbl.HandleKey(k) {
		cmds := []tea.Cmd{t.loadCompare()}
		if t.tbl.NearEnd(nearEnd) {
			cmds = append(cmds, t.loadMore())
		}
		return tea.Batch(cmds...)
	}
	switch k.String() {
	case "/":
		t.editing = true
		t.input.SetValue(t.search)
		t.input.CursorEnd()
		return t.input.Focus()
	case "backspace":
		if t.search == "" {
			return nil
		}
		t.search = ""
		return t.loadFirst(true, false)
	case "o":
		return app.OpenURL(env, tagactions.TagsURL(t.sh.projectURL(), t.search))
	case "n":
		if !t.sh.archived {
			return tagactions.NewTag(env, t.sh.ref, t.sh.defaultBranch)
		}
		return nil
	}
	tag := t.selected()
	if tag == nil {
		return nil
	}
	switch k.String() {
	case "enter":
		return tagactions.OpenPipeline(env, t.sh.ref, *tag)
	case "y":
		return tagactions.Copy(env, *tag)
	case "D":
		if !t.sh.archived {
			return tagactions.Delete(env, t.sh.ref, *tag)
		}
	}
	return nil
}

// ---- view ----

func (t *tagsTab) filterLine() string {
	env := t.env()
	s, m := env.Styles, env.Msg.Tag
	left := s.Accent.Render("/") + " " + s.FilterKey.Render(m.SearchLabel)
	switch {
	case t.editing:
		left += s.FilterKey.Render(":") + " " + t.input.View() + "  " + s.Muted.Render(m.SearchHint)
		return render.Clip(left, t.w)
	case t.search != "":
		left += s.FilterKey.Render(":") + " " + s.FilterValue.Render(t.search)
	}
	return common.FilterBarText(s, left, m.SortLabel, t.w)
}

func (t *tagsTab) View() string {
	parts := []string{t.filterLine(), t.tbl.View()}
	if t.previewShown() {
		parts = append(parts, t.preview())
	}
	return strings.Join(parts, "\n")
}

func (t *tagsTab) preview() string {
	env := t.env()
	s := env.Styles
	frame := func(lines ...string) string {
		for i := range lines {
			lines[i] = " " + render.Clip(lines[i], t.w-4)
		}
		return common.Box(s, strings.Join(lines, "\n"), t.w, tagactions.PreviewLines+2, common.BoxNormal)
	}
	tag := t.selected()
	if tag == nil {
		return frame("", "", "")
	}
	d := tagactions.PreviewData{Tag: *tag, Branch: t.sh.defaultBranch}
	if e, ok := t.cmp.Entry(t.sh.ref, tag.Name); ok {
		d.State, d.Ahead = e.State, e.Ahead
	} else if d.Branch != "" {
		d.State = tagactions.CompareLoading
	}
	if t.search == "" {
		d.Suggest = tagactions.Suggested(t.items)
	}
	return frame(tagactions.Preview(env, d)...)
}

func (t *tagsTab) KeyMap() common.KeyMap {
	k, tm := t.env().Msg.Keys, t.env().Msg.Tag
	hide := func(b common.Binding) common.Binding {
		b.Hidden = t.sh.archived
		return b
	}
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", tm.KeyPipeline, common.PriorityHigh),
		common.NewBinding([]string{"/"}, "", tm.KeySearch, common.PriorityNormal),
		hide(common.NewBinding([]string{"n"}, "", tm.KeyNew, common.PriorityNormal)),
		hide(common.NewBinding([]string{"D"}, "", tm.KeyDelete, common.PriorityNormal)),
		common.NewBinding([]string{"backspace"}, "⌫", k.Reset, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", tm.KeyCopyName, common.PriorityLow),
		common.NewBinding([]string{"o"}, "", k.Browser, common.PriorityLow),
	}}
}
