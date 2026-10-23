package project

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/comment"
	"github.com/4aykovski/glview/internal/ui/screens/issueactions"
	"github.com/4aykovski/glview/internal/ui/screens/issuedetail"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	issuesPollInterval = 30 * time.Second
	relatedTTL         = 5 * time.Minute
	notesTTL           = 30 * time.Second
	relatedParallel    = 4   // related-MR requests in flight at once
	issuePreviewRightW = 140 // the preview goes right of the list from this width
	issueListShare     = 55  // percent of the width for the list with the preview on the right
	issueBelowMin      = 7   // lines of the preview below the list, frame included
)

type (
	issuesLoadedMsg struct {
		tok     common.Token
		cursor  string
		refresh bool
		page    gitlab.Page[gitlab.Issue]
		err     error
	}
	relatedLoadedMsg struct {
		tok common.Token
		iid int64
		mrs []gitlab.RelatedMR
		err error
	}
	issueNotesMsg struct {
		tok   common.Token
		iid   int64
		notes []gitlab.Note
		err   error
	}
)

type relatedEntry struct {
	mrs []gitlab.RelatedMR
	at  time.Time
}

type notesEntry struct {
	notes []gitlab.Note
	state issueactions.NotesState
	at    time.Time
}

// issuesTab is the issue list of the project (SPEC §3.4).
type issuesTab struct {
	sh   *shared
	gen  common.Gen
	poll common.Gen
	tbl  *table.Model
	fl   *filterline.Model

	filter gitlab.IssueFilter
	cancel context.CancelFunc

	items    []gitlab.Issue
	firstLen int
	next     string
	total    int

	// Related MRs are asked in the background, a few at a time.
	relGen    common.Gen
	related   map[int64]relatedEntry
	relQueue  []int64
	relActive int
	relForce  bool // the next page brings everything again (r)
	forceRel  map[int64]bool

	// Comments of the selected issue for the preview.
	notesGen common.Gen
	notesIID int64 // issue of the request in flight
	notes    map[int64]notesEntry

	loaded, loading, moreLoading, moreErr bool
	err                                   error
	updated                               time.Time
	active                                bool
	dirty                                 bool // changed behind the tab's back: reload on activation
	w, h                                  int
}

func newIssuesTab(sh *shared) *issuesTab {
	t := &issuesTab{sh: sh, gen: common.NewGen(), poll: common.NewGen(), relGen: common.NewGen(), notesGen: common.NewGen(), total: -1,
		filter: gitlab.IssueFilter{State: "opened"}, related: map[int64]relatedEntry{}, notes: map[int64]notesEntry{}, forceRel: map[int64]bool{}}
	t.tbl = table.New(sh.env.Styles)
	t.fl = filterline.New(sh.env.Styles, t.filterKeys())
	t.rebuild()
	return t
}

func (t *issuesTab) env() *app.Env { return t.sh.env }

func (t *issuesTab) filterKeys() []filterline.Key {
	m, im := t.env().Msg.Project, t.env().Msg.Issue
	return []filterline.Key{
		{Name: "state", Desc: m.KeyState, Values: []string{"opened", "closed", "all"}},
		{Name: "assignee", Desc: im.KeyAssignee, Values: []string{"any", "@me", "none"}, Free: true, Source: t.sh.membersSource()},
		{Name: "label", Desc: im.KeyLabel, Multi: true, Source: t.sh.labelsSource()},
		{Name: "milestone", Desc: im.KeyMilestone, Values: []string{"any", "none"}, Free: true, Source: t.sh.milestonesSource()},
	}
}

func (t *issuesTab) UpdatedAt() time.Time { return t.updated }

// Count is the number of issues matching the filter, when known.
func (t *issuesTab) Count() int {
	if !t.loaded {
		return -1
	}
	return t.total
}

// Status is "5 / 12": the position of the cursor and the number of issues.
func (t *issuesTab) Status() string {
	if len(t.items) == 0 {
		return ""
	}
	total := max(t.total, len(t.items))
	return fmt.Sprintf("%d / %d", t.tbl.Cursor()+1, total)
}

func (t *issuesTab) CapturesInput() bool { return t.fl.Focused() }

// Items returns the loaded issues (for tests).
func (t *issuesTab) Items() []gitlab.Issue { return t.items }

// Related returns the related MRs loaded for the issue (for tests).
func (t *issuesTab) Related(iid int64) []gitlab.RelatedMR { return t.related[iid].mrs }

func (t *issuesTab) SetSize(w, h int) {
	t.w, t.h = w, h
	t.fl.SetWidth(t.listWidth() - 2)
	t.layout()
}

// ---- layout ----

// previewRight reports whether the preview stands right of the list.
func (t *issuesTab) previewRight() bool { return t.w >= issuePreviewRightW && t.h >= 8 }

// previewBelow reports whether the preview stands under the list (the rule
// of the MRs tab).
func (t *issuesTab) previewBelow() bool {
	return !t.previewRight() && t.w >= previewMinW && t.h >= previewLines+2+8
}

// listWidth is the width of the list and the filter line: with the preview
// on the right, a column is left free before the rule.
func (t *issuesTab) listWidth() int {
	if t.previewRight() {
		return t.w*issueListShare/100 - 1
	}
	return t.w
}

// belowHeight is the height of the preview under the list, frame included:
// what the rows of the list leave, but the list keeps up to half.
func (t *issuesTab) belowHeight() int {
	body := t.h - 1
	list := min(len(t.items)+3, body/2)
	return max(issueBelowMin, body-max(list, 5))
}

func (t *issuesTab) filterHeight() int {
	if t.fl.Focused() {
		return t.fl.Height()
	}
	return 1
}

func (t *issuesTab) layout() {
	t.fl.SetWidth(t.listWidth() - 2)
	body := t.h - t.filterHeight()
	if t.previewBelow() {
		body -= t.belowHeight()
	}
	t.tbl.SetSize(t.listWidth(), max(1, body))
}

// ---- rows ----

func (t *issuesTab) rebuild() {
	env := t.env()
	s, m, im := env.Styles, env.Msg.MR, env.Msg.Issue
	t.tbl.SetStyles(s)
	t.fl.SetStyles(s)
	t.tbl.SetColumns([]table.Column{
		{Title: m.ColIID, Min: 6, Priority: 100},
		{Title: m.ColTitle, Min: 20, Flex: true, Priority: 90},
		{Title: im.ColLabels, Min: 8, Max: 24, Priority: 50},
		{Title: m.ColUpd, Min: 4, Priority: 60, AlignRight: true},
	})
	rows := make([]table.Row, len(t.items))
	for i, is := range t.items {
		title := is.Title
		if mark := issueactions.RelatedMark(env, t.related[is.IID].mrs); mark != "" {
			title += " " + mark
		}
		rows[i] = table.Row{ID: strconv.FormatInt(is.IID, 10), Cells: []string{
			s.Accent.Render(fmt.Sprintf("#%d", is.IID)), title, strings.Join(is.Labels, " "), render.Ago(env.Lang, env.Now(), is.UpdatedAt),
		}}
	}
	t.tbl.SetRows(rows)
	t.tbl.SetEmpty(t.emptyText())
	t.setTail()
	t.layout()
}

func (t *issuesTab) setTail() {
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

func (t *issuesTab) isDefaultFilter(ignoreAssignee bool) bool {
	f := t.filter
	return (f.State == "" || f.State == "opened") && (ignoreAssignee || f.Assignee == "") && f.Milestone == "" && len(f.Labels) == 0 && f.Search == ""
}

func (t *issuesTab) emptyText() string {
	m := t.env().Msg.Issue
	switch {
	case t.isDefaultFilter(false):
		return "\n  " + m.EmptyOpen
	case t.filter.Assignee == "@me" && t.isDefaultFilter(true):
		return "\n  " + m.EmptyMine
	}
	return "\n  " + m.EmptyFiltered
}

// ---- loading ----

func (t *issuesTab) key(cursor string) string {
	return issueactions.ListKey(t.sh.ref.FullPath, t.filter, cursor)
}

func (t *issuesTab) prefix() string {
	return issueactions.ListKey(t.sh.ref.FullPath, t.filter, "")
}

func (t *issuesTab) fetch(tok common.Token, cursor string, force, refresh bool) tea.Cmd {
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
		page, _, err := common.Fetch(ctx, env.Cache, key, listTTL, force, func(ctx context.Context) (gitlab.Page[gitlab.Issue], error) {
			return client.Issues().List(ctx, ref, f, cursor, limit)
		})
		return issuesLoadedMsg{tok: tok, cursor: cursor, refresh: refresh, page: page, err: err}
	}
}

// loadFirst requests the first page. reset drops the rows (filter change);
// otherwise the result is merged into the loaded rows.
func (t *issuesTab) loadFirst(reset, force bool) tea.Cmd {
	tok := t.gen.Next()
	t.moreLoading, t.moreErr, t.err = false, false, nil
	if reset {
		t.items, t.firstLen, t.next, t.total, t.loaded = nil, 0, "", -1, false
		t.loading = true
		t.resetRelated()
		t.rebuild()
	} else {
		t.loading = !t.loaded
		t.setTail()
	}
	return t.fetch(tok, "", force, !reset)
}

func (t *issuesTab) loadMore() tea.Cmd {
	if t.next == "" || t.moreLoading || t.loading {
		return nil
	}
	t.moreLoading, t.moreErr = true, false
	t.setTail()
	return t.fetch(t.gen.Current(), t.next, false, false)
}

func (t *issuesTab) Refresh() tea.Cmd {
	if t.moreErr && len(t.items) > 0 {
		t.moreLoading, t.moreErr = true, false
		t.setTail()
		return t.fetch(t.gen.Current(), t.next, true, false)
	}
	t.relForce = true // r goes past the cache of the related MRs too
	return tea.Batch(t.loadFirst(false, true), t.loadNotes(true))
}

func (t *issuesTab) schedulePoll() tea.Cmd {
	if !t.active || t.env().AuthLost {
		t.poll.Next()
		return nil
	}
	return common.PollAfter(t.env().PollInterval(false), t.poll.Next())
}

func (t *issuesTab) Activate() tea.Cmd {
	t.active = true
	switch {
	case !t.loaded && !t.loading:
		return t.loadFirst(true, false)
	case t.loaded && t.dirty && !t.env().AuthLost:
		t.dirty = false
		return tea.Batch(t.loadFirst(false, true), t.loadNotes(true))
	case t.loaded && t.env().Now().Sub(t.updated) >= issuesPollInterval && !t.env().AuthLost:
		return t.loadFirst(false, true)
	}
	return t.schedulePoll()
}

func (t *issuesTab) Deactivate() {
	t.active = false
	t.poll.Next()
}

// ---- related merge requests ----

// resetRelated forgets the related MRs and drops the answers in flight.
func (t *issuesTab) resetRelated() {
	t.relGen.Next()
	t.related = map[int64]relatedEntry{}
	t.relQueue, t.relActive = nil, 0
	t.forceRel = map[int64]bool{}
}

// queueRelated puts the issues of the list that have merge requests and no
// fresh answer in the queue; force asks again for all of them.
func (t *issuesTab) queueRelated(force bool) tea.Cmd {
	now := t.env().Now()
	for _, is := range t.items {
		if is.MergeRequestsCount == 0 || slices.Contains(t.relQueue, is.IID) {
			continue
		}
		if e, ok := t.related[is.IID]; ok && !force && now.Sub(e.at) < relatedTTL {
			continue
		}
		if force {
			t.forceRel[is.IID] = true
		}
		t.relQueue = append(t.relQueue, is.IID)
	}
	return t.pumpRelated()
}

// pumpRelated starts requests while there is room.
func (t *issuesTab) pumpRelated() tea.Cmd {
	var cmds []tea.Cmd
	env := t.env()
	for t.relActive < relatedParallel && len(t.relQueue) > 0 {
		iid := t.relQueue[0]
		t.relQueue = t.relQueue[1:]
		t.relActive++
		tok, force := t.relGen.Current(), t.forceRel[iid]
		delete(t.forceRel, iid)
		client, ref := env.Client, t.sh.ref
		key := issueactions.RelatedKey(ref.FullPath, iid)
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			mrs, _, err := common.Fetch(ctx, env.Cache, key, relatedTTL, force, func(ctx context.Context) ([]gitlab.RelatedMR, error) {
				return client.Issues().RelatedMRs(ctx, ref, iid)
			})
			return relatedLoadedMsg{tok: tok, iid: iid, mrs: mrs, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (t *issuesTab) onRelated(msg relatedLoadedMsg) tea.Cmd {
	if !t.relGen.Valid(msg.tok) {
		return nil
	}
	t.relActive--
	if msg.err != nil {
		// The marker is a nicety: no toast, a trace in the debug log.
		debuglog.L().Debug("issues: related MRs failed", "project", t.sh.ref.FullPath, "iid", msg.iid, "err", msg.err)
	} else {
		t.related[msg.iid] = relatedEntry{mrs: msg.mrs, at: t.env().Now()}
		t.rebuild()
	}
	return t.pumpRelated()
}

// ---- comments of the selected issue ----

func (t *issuesTab) selected() *gitlab.Issue {
	id := t.tbl.CursorID()
	for i := range t.items {
		if strconv.FormatInt(t.items[i].IID, 10) == id {
			return &t.items[i]
		}
	}
	return nil
}

// loadNotes asks for the last comments of the selected issue unless they
// are fresh or already on the way; force (r, poll) asks again. The answer
// for another issue is dropped.
func (t *issuesTab) loadNotes(force bool) tea.Cmd {
	is := t.selected()
	if is == nil || !t.active {
		return nil
	}
	e, ok := t.notes[is.IID]
	if !force {
		switch {
		case ok && e.state == issueactions.NotesReady && t.env().Now().Sub(e.at) < notesTTL:
			t.notesGen.Next() // an answer for the issue left behind is not needed
			return nil
		case ok && e.state == issueactions.NotesLoading && t.notesIID == is.IID:
			return nil
		}
	}
	if !ok {
		t.notes[is.IID] = notesEntry{state: issueactions.NotesLoading}
	}
	tok := t.notesGen.Next()
	t.notesIID = is.IID
	env, ref, iid := t.env(), t.sh.ref, is.IID
	key := issueactions.NotesKey(ref.FullPath, iid)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		notes, _, err := common.Fetch(ctx, env.Cache, key, notesTTL, force, func(ctx context.Context) ([]gitlab.Note, error) {
			return env.Client.Issues().Notes(ctx, ref, iid, issueactions.NotesLast)
		})
		return issueNotesMsg{tok: tok, iid: iid, notes: notes, err: err}
	}
}

func (t *issuesTab) onNotes(msg issueNotesMsg) {
	is := t.selected()
	if !t.notesGen.Valid(msg.tok) || is == nil || is.IID != msg.iid {
		return
	}
	if msg.err != nil {
		// A failed refresh keeps what is shown; otherwise the preview says so.
		if e := t.notes[msg.iid]; e.state != issueactions.NotesReady {
			t.notes[msg.iid] = notesEntry{state: issueactions.NotesFailed}
		}
		return
	}
	t.notes[msg.iid] = notesEntry{notes: msg.notes, state: issueactions.NotesReady, at: t.env().Now()}
}

// ---- update ----

func (t *issuesTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	env := t.env()
	if cmd := t.fl.HandleMsg(msg); cmd != nil {
		t.layout()
		return t, cmd
	}
	switch msg := msg.(type) {
	case filterline.SuggestionsMsg:
		t.layout()
		return t, nil
	case issuesLoadedMsg:
		return t, t.onLoaded(msg)
	case relatedLoadedMsg:
		return t, t.onRelated(msg)
	case issueNotesMsg:
		t.onNotes(msg)
		return t, nil
	case comment.SentMsg:
		if msg.Target.Kind != comment.KindIssue || msg.Target.Project.FullPath != t.sh.ref.FullPath {
			return t, nil
		}
		delete(t.notes, msg.Target.IID)
		if !t.active {
			t.dirty = true // covered by the issue screen: reload on return
			return t, nil
		}
		return t, tea.Batch(t.loadFirst(false, true), t.loadNotes(true))
	case issueactions.StateMsg:
		if msg.Ref.FullPath != t.sh.ref.FullPath {
			return t, nil
		}
		if !t.active {
			// The issue screen on top did it and showed the toast.
			t.dirty = t.dirty || msg.Err == nil
			return t, nil
		}
		cmd := issueactions.Done(env, msg)
		if msg.Err != nil {
			return t, cmd
		}
		return t, tea.Batch(cmd, t.loadFirst(false, true))
	case common.PollTickMsg:
		if !t.poll.Valid(msg.Token) || !t.active || env.AuthLost {
			return t, nil
		}
		return t, tea.Batch(t.loadFirst(false, true), t.loadNotes(true))
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
		if msg.Focused && t.active && t.loaded && env.Now().Sub(t.updated) >= issuesPollInterval {
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

func (t *issuesTab) onLoaded(msg issuesLoadedMsg) tea.Cmd {
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
		t.items = appendUniqueIssues(t.items, page.Items)
		t.next = page.NextCursor
	case msg.refresh && t.loaded && len(t.items) > t.firstLen:
		tail := t.items[t.firstLen:]
		t.items = appendUniqueIssues(slices.Clone(page.Items), tail)
		t.firstLen = len(page.Items)
		t.next = page.NextCursor
	default:
		t.items = slices.Clone(page.Items)
		t.firstLen = len(page.Items)
		t.next = page.NextCursor
	}
	force := false
	if msg.cursor == "" {
		t.total = page.Total
		t.loaded, t.loading = true, false
		t.updated = env.Now()
		force, t.relForce = t.relForce, false
	}
	t.rebuild()
	cmds := []tea.Cmd{t.schedulePoll(), t.queueRelated(force), t.loadNotes(false)}
	if t.tbl.NearEnd(nearEnd) {
		cmds = append(cmds, t.loadMore())
	}
	return tea.Batch(cmds...)
}

func appendUniqueIssues(dst, src []gitlab.Issue) []gitlab.Issue {
	seen := make(map[int64]bool, len(dst))
	for _, i := range dst {
		seen[i.IID] = true
	}
	for _, i := range src {
		if !seen[i.IID] {
			dst = append(dst, i)
		}
	}
	return dst
}

func (t *issuesTab) onKey(k tea.KeyPressMsg) tea.Cmd {
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
		cmds := []tea.Cmd{t.loadNotes(false)}
		if t.tbl.NearEnd(nearEnd) {
			cmds = append(cmds, t.loadMore())
		}
		return tea.Batch(cmds...)
	}
	switch k.String() {
	case "/":
		t.fl.SetKeys(t.filterKeys())
		t.fl.SetCurrent(t.filterValues())
		cmd := t.fl.Open(t.filterText())
		t.layout()
		return cmd
	case "s":
		next := map[string]string{"": "closed", "opened": "closed", "closed": "all", "all": "opened"}[t.filter.State]
		t.filter.State = next
		return t.loadFirst(true, false)
	case "m":
		if t.filter.Assignee == "@me" {
			t.filter.Assignee = ""
		} else {
			t.filter.Assignee = "@me"
		}
		return t.loadFirst(true, false)
	case "backspace":
		t.filter = gitlab.IssueFilter{State: "opened"}
		return t.loadFirst(true, false)
	case "o":
		return app.OpenURL(env, issueactions.ListURL(t.sh.projectURL(), t.filter, env.User.Username))
	}
	is := t.selected()
	if is == nil {
		return nil
	}
	switch k.String() {
	case "enter":
		return app.Push(issuedetail.New(env, t.sh.ref, *is, t.sh.archived))
	case "c":
		if !t.sh.archived {
			return comment.Open(env, comment.Target{Project: t.sh.ref, Kind: comment.KindIssue, IID: is.IID, Title: is.Title})
		}
	case "x":
		if !t.sh.archived {
			return issueactions.Toggle(env, t.sh.ref, *is)
		}
	case "O":
		if is.WebURL != "" {
			return app.OpenURL(env, is.WebURL)
		}
	case "y":
		if is.WebURL != "" {
			return app.Copy(env, is.WebURL, render.ShortURL(is.WebURL))
		}
	}
	return nil
}

// ---- filter ----

func issueDefaults() map[string]string {
	return map[string]string{"state": "opened", "assignee": "any", "milestone": "any"}
}

// filterValues returns the current key:value set, defaults included.
func (t *issuesTab) filterValues() map[string]string {
	v := issueDefaults()
	f := t.filter
	if f.State != "" {
		v["state"] = f.State
	}
	if f.Assignee != "" {
		v["assignee"] = f.Assignee
	}
	if f.Milestone != "" {
		v["milestone"] = f.Milestone
	}
	if len(f.Labels) > 0 {
		v["label"] = strings.Join(f.Labels, ",")
	}
	return v
}

// filterText is the starting text of the input: the non-default pairs.
func (t *issuesTab) filterText() string {
	vals := t.filterValues()
	for k, d := range issueDefaults() {
		if vals[k] == d {
			delete(vals, k)
		}
	}
	return filterline.Format(t.filterKeys(), vals, t.filter.Search)
}

func (t *issuesTab) applyFilter() tea.Cmd {
	vals, free, err := t.fl.Parsed()
	if err != nil {
		return nil
	}
	f := gitlab.IssueFilter{State: "opened", Search: free}
	if v := vals["state"]; v != "" {
		f.State = v
	}
	if v := vals["assignee"]; v != "" && v != "any" {
		f.Assignee = v
	}
	if v := vals["milestone"]; v != "" && v != "any" {
		f.Milestone = v
	}
	if v := vals["label"]; v != "" && v != "any" {
		f.Labels = strings.Split(v, ",")
	}
	t.filter = f
	return t.loadFirst(true, false)
}

func (t *issuesTab) filterError(err error) string {
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

// ---- view ----

func (t *issuesTab) filterLine() string {
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
	if t.filter.Search != "" {
		extra = s.FilterValue.Render(t.filter.Search)
	}
	return common.FilterBar(s, pairs, extra, "", t.listWidth())
}

func (t *issuesTab) previewData() issueactions.PreviewData {
	is := t.selected()
	d := issueactions.PreviewData{Issue: *is, Hint: !t.sh.archived}
	if e, ok := t.related[is.IID]; ok {
		d.Related = e.mrs
	}
	e, ok := t.notes[is.IID]
	if !ok {
		e.state = issueactions.NotesLoading
	}
	d.Notes, d.State = e.notes, e.state
	return d
}

func (t *issuesTab) View() string {
	left := t.filterLine() + "\n" + t.tbl.View()
	switch {
	case t.previewRight():
		return t.viewRight(left)
	case t.previewBelow() && t.selected() != nil:
		return left + "\n" + t.viewBelow()
	}
	return left
}

// viewRight puts the preview right of the list, a vertical rule between.
func (t *issuesTab) viewRight(left string) string {
	s := t.env().Styles
	lw := t.listWidth() + 1
	pw := t.w - lw - 1
	var prev []string
	if t.selected() != nil {
		prev = issueactions.Preview(t.env(), t.previewData(), pw-3, t.h)
	}
	ll := strings.Split(left, "\n")
	var b strings.Builder
	for i := range t.h {
		l := ""
		if i < len(ll) {
			l = ll[i]
		}
		p := ""
		if i < len(prev) {
			p = "  " + prev[i]
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(render.Pad(ansi.Truncate(l, lw, ""), lw) + s.Muted.Render("│") + p)
	}
	return b.String()
}

// viewBelow draws the preview in a frame under the list.
func (t *issuesTab) viewBelow() string {
	s := t.env().Styles
	h := t.belowHeight()
	lines := issueactions.Preview(t.env(), t.previewData(), t.w-4, h-2)
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return common.Box(s, strings.Join(lines, "\n"), t.w, h, common.BoxNormal)
}

func (t *issuesTab) KeyMap() common.KeyMap {
	k, im := t.env().Msg.Keys, t.env().Msg.Issue
	hide := func(b common.Binding) common.Binding {
		b.Hidden = t.sh.archived
		return b
	}
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", k.Open, common.PriorityHigh),
		common.NewBinding([]string{"/"}, "", k.Filter, common.PriorityNormal),
		hide(common.NewBinding([]string{"c"}, "", k.Comment, common.PriorityNormal)),
		hide(common.NewBinding([]string{"x"}, "", im.KeyToggle, common.PriorityNormal)),
		common.NewBinding([]string{"s"}, "", k.State, common.PriorityNormal),
		common.NewBinding([]string{"m"}, "", k.Me, common.PriorityNormal),
		common.NewBinding([]string{"backspace"}, "⌫", k.Reset, common.PriorityLow),
		common.NewBinding([]string{"o"}, "", k.BrowserList, common.PriorityLow),
		common.NewBinding([]string{"O"}, "", im.KeyBrowser, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
	}}
}
