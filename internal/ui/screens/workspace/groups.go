package workspace

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
	"github.com/4aykovski/glview/internal/ui/common/grouped"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	groupPreviewLns = 3                      // lines inside the preview frame of the group tabs
	expandDelay     = 100 * time.Millisecond // groups expanded within it are asked in one request
)

// expandMsg asks the groups expanded during the last expandDelay.
type expandMsg struct{ tok common.Token }

// groupBase is the loading and layout shared by the MRs and Pipelines
// tabs (SPEC §3.3). Groups are collapsed until the user expands them; the
// expanded set is kept per host, workspace and tab in the state file. Only
// expanded groups are asked: a full load (first open, poll, r) asks all of
// them, expanding asks the new ones, reloads ask some of them (after an
// action, the fast tick). Every request owns the projects it asks: a batch
// is taken only for the projects whose latest request it is.
type groupBase[T any] struct {
	sh   *shared
	name string     // the tab in the state file: "mrs" or "pipelines"
	seq  common.Gen // request tokens
	poll common.Gen
	expG common.Gen // expand debounce
	tbl  *grouped.Model
	fl   *filterline.Model

	fetch func(ctx context.Context, ps []gitlab.ProjectRef) <-chan gitlab.BatchResult[T]

	data     map[string]gitlab.ProjectGroup[T]
	owner    map[string]common.Token             // the latest request of each project
	runs     map[common.Token]context.CancelFunc // requests in flight
	full     common.Token                        // the running full load
	expanded map[string]bool                     // saved in the state file
	want     map[string]bool                     // expanded, waiting for expandDelay

	loaded  bool
	loading bool
	updated time.Time
	active  bool
	failed  bool // the running load already toasted an error
	moved   bool // the user moved the cursor: keep it on its line
	w, h    int
}

// setGroups shows new groups; until the user moves, the cursor stays on
// the first line (the order may still change with the overview).
func (b *groupBase[T]) setGroups(gs []grouped.Group) {
	b.tbl.SetGroups(gs)
	if !b.moved {
		b.tbl.Top()
	}
}

// handleKey moves the cursor and remembers that the user did.
func (b *groupBase[T]) handleKey(k tea.KeyPressMsg) bool {
	if b.tbl.HandleKey(k) {
		b.moved = true
		return true
	}
	return false
}

func newGroupBase[T any](sh *shared, name string, keys []filterline.Key) *groupBase[T] {
	b := &groupBase[T]{sh: sh, name: name, seq: common.NewGen(), poll: common.NewGen(), expG: common.NewGen(),
		data: map[string]gitlab.ProjectGroup[T]{}, owner: map[string]common.Token{},
		runs: map[common.Token]context.CancelFunc{}, expanded: map[string]bool{}, want: map[string]bool{}}
	for _, p := range sh.env.Catalog.Expanded(sh.source, name) {
		b.expanded[p] = true
	}
	b.tbl = grouped.New(sh.env.Styles)
	b.fl = filterline.New(sh.env.Styles, keys)
	b.tbl.SetMoreText(sh.env.Msg.WS.More)
	return b
}

func (b *groupBase[T]) env() *app.Env { return b.sh.env }

// UpdatedAt is the time of the last full load; with every group collapsed
// the tab shows the overview, and its time.
func (b *groupBase[T]) UpdatedAt() time.Time {
	if len(b.targets()) == 0 {
		return b.sh.updated
	}
	return b.updated
}

func (b *groupBase[T]) CapturesInput() bool { return b.fl.Focused() }

func (b *groupBase[T]) SetSize(w, h int) {
	b.w, b.h = w, h
	b.fl.SetWidth(w - 2)
	b.layout()
}

func (b *groupBase[T]) previewShown() bool {
	return b.w >= previewMinW && b.h >= groupPreviewLns+2+minTableRows
}

func (b *groupBase[T]) filterHeight() int {
	if b.fl.Focused() {
		return b.fl.Height()
	}
	return 1
}

func (b *groupBase[T]) layout() {
	body := b.h - b.filterHeight()
	if b.previewShown() {
		body -= groupPreviewLns + 2
	}
	b.tbl.SetSize(b.w, max(1, body))
}

// targets are the projects a full load asks: the expanded groups in
// display order.
func (b *groupBase[T]) targets() []gitlab.ProjectRef {
	var out []gitlab.ProjectRef
	for _, p := range b.sh.order {
		if b.expanded[p] {
			out = append(out, b.ref(p))
		}
	}
	return out
}

// request asks refs; the request owns them until a newer one asks them.
func (b *groupBase[T]) request(refs []gitlab.ProjectRef) (common.Token, tea.Cmd) {
	tok := b.seq.Next()
	ctx, cancel := context.WithCancel(context.Background())
	b.runs[tok] = cancel
	for _, r := range refs {
		b.owner[r.FullPath] = tok
	}
	return tok, listen(tok, b.fetch(ctx, refs))
}

// inFlight reports whether a running request asks path.
func (b *groupBase[T]) inFlight(path string) bool {
	tok, ok := b.owner[path]
	if !ok {
		return false
	}
	_, ok = b.runs[tok]
	return ok
}

// load starts a full load. reset drops the loaded groups (filter change).
func (b *groupBase[T]) load(reset bool) tea.Cmd {
	if !b.sh.ready {
		return nil
	}
	b.stopRuns()
	b.expG.Next()
	b.want = map[string]bool{}
	if reset {
		b.data = map[string]gitlab.ProjectGroup[T]{}
		b.loaded = false
	}
	b.failed = false
	refs := b.targets()
	if len(refs) == 0 {
		b.loaded, b.updated = true, b.env().Now()
		return b.schedulePoll()
	}
	tok, cmd := b.request(refs)
	b.full, b.loading = tok, true
	return cmd
}

// reload asks again for some projects without disturbing a full load.
func (b *groupBase[T]) reload(refs []gitlab.ProjectRef) tea.Cmd {
	if len(refs) == 0 || b.loading {
		return nil
	}
	_, cmd := b.request(refs)
	return cmd
}

// fill asks the groups that should show records but have none and are not
// asked yet (expanded while the tab was hidden).
func (b *groupBase[T]) fill() tea.Cmd {
	var refs []gitlab.ProjectRef
	for _, r := range b.targets() {
		if _, ok := b.data[r.FullPath]; !ok && !b.inFlight(r.FullPath) {
			refs = append(refs, r)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	_, cmd := b.request(refs)
	return cmd
}

// stopRuns cancels every request.
func (b *groupBase[T]) stopRuns() {
	for _, cancel := range b.runs {
		cancel()
	}
	b.runs = map[common.Token]context.CancelFunc{}
	b.owner = map[string]common.Token{}
	b.full = common.Token{}
	b.loading = false
}

// stop cancels the loads (the tab or the screen is no longer shown).
func (b *groupBase[T]) stop() {
	b.stopRuns()
	b.expG.Next()
	b.want = map[string]bool{}
	b.poll.Next()
}

// onBatch merges a batch. It returns whether the message was ours, whether
// a run ended (full: the whole tab) and the command to go on.
func (b *groupBase[T]) onBatch(msg batchMsg[T]) (ours, done, full bool, cmd tea.Cmd) {
	cancel, ok := b.runs[msg.tok]
	if !ok {
		return false, false, false, nil
	}
	full = msg.tok == b.full
	if msg.done {
		cancel()
		delete(b.runs, msg.tok)
		if full {
			b.loading, b.loaded, b.full = false, true, common.Token{}
			b.updated = b.env().Now()
		}
		return true, true, full, nil
	}
	var fail tea.Cmd
	for _, g := range msg.res.Groups {
		path := g.Project.FullPath
		if b.owner[path] != msg.tok {
			continue // a newer request asks it, or the group was collapsed
		}
		b.data[path] = g
		if g.Err != nil && !errors.Is(g.Err, gitlab.ErrNotFound) && !b.failed {
			b.failed = true
			fail = b.env().Fail(g.Err)
		}
	}
	return true, false, full, tea.Batch(fail, msg.next)
}

// toggle collapses or expands the group under the cursor and saves the
// expanded set. An expanded group is asked after expandDelay, together
// with the groups expanded meanwhile. The caller rebuilds the table and
// calls onToggled.
func (b *groupBase[T]) toggle() (string, tea.Cmd) {
	path := b.cursorGroup()
	if path == "" || b.sh.noAccess(path) {
		return "", nil
	}
	delete(b.data, path)
	delete(b.owner, path)
	var cmd tea.Cmd
	if b.expanded[path] {
		delete(b.expanded, path)
		delete(b.want, path)
	} else {
		b.expanded[path] = true
		b.want[path] = true
		if len(b.want) == 1 {
			tok := b.expG.Next()
			cmd = tea.Tick(expandDelay, func(time.Time) tea.Msg { return expandMsg{tok: tok} })
		}
	}
	b.moved = true
	return path, tea.Batch(cmd, b.save())
}

// onToggled puts the cursor on the header of the toggled group.
func (b *groupBase[T]) onToggled(path string) {
	if path != "" {
		b.tbl.SetCursorID(grouped.HeaderID(path))
	}
}

// onExpand asks the groups expanded during expandDelay in one request.
func (b *groupBase[T]) onExpand(msg expandMsg) tea.Cmd {
	if !b.expG.Valid(msg.tok) {
		return nil
	}
	var refs []gitlab.ProjectRef
	for _, r := range b.targets() {
		if b.want[r.FullPath] {
			refs = append(refs, r)
		}
	}
	b.want = map[string]bool{}
	if !b.active || len(refs) == 0 {
		return nil // fill asks them when the tab is shown again
	}
	_, cmd := b.request(refs)
	return cmd
}

// ExpandMsg returns the expand tick that is due next (for tests).
func (b *groupBase[T]) ExpandMsg() tea.Msg { return expandMsg{tok: b.expG.Current()} }

// save writes the expanded groups of the workspace projects.
func (b *groupBase[T]) save() tea.Cmd {
	var paths []string
	for _, p := range b.sh.order {
		if b.expanded[p] {
			paths = append(paths, p)
		}
	}
	return b.env().Catalog.SetExpanded(b.sh.source, b.name, paths)
}

// refresh is the poll tick and r: the expanded groups and the overview for
// the headers of the collapsed ones.
func (b *groupBase[T]) refresh() tea.Cmd {
	return tea.Batch(b.load(false), b.sh.headers())
}

// groups builds the grouped rows in project order. row maps a record to
// its table row, summary describes a loaded group, head a collapsed one
// (from the overview), empty is the text of a group without records.
func (b *groupBase[T]) groups(row func(T) table.Row, summary func(path string, g gitlab.ProjectGroup[T]) string,
	head func(path string) string, empty string) []grouped.Group {
	env := b.env()
	m := env.Msg
	out := make([]grouped.Group, 0, len(b.sh.order))
	for _, path := range b.sh.order {
		g := grouped.Group{Key: path, Title: path}
		pg, ok := b.data[path]
		switch {
		case b.sh.noAccess(path) || (ok && errors.Is(pg.Err, gitlab.ErrNotFound)):
			g.State, g.Note = grouped.Failed, m.WS.NoAccess
		case !b.expanded[path]:
			g.Collapsed, g.Summary = true, head(path)
		case ok && pg.Err != nil:
			g.State, g.Note = grouped.Failed, app.ErrorText(env, pg.Err)
		case !ok:
			g.State, g.Note, g.Summary = grouped.Expanding, m.WS.Loading, head(path)
		case len(pg.Items) == 0:
			g.State, g.Note = grouped.Empty, empty
		default:
			for _, it := range pg.Items {
				g.Rows = append(g.Rows, row(it))
			}
			if pg.Total > len(pg.Items) {
				g.More = pg.Total - len(pg.Items)
			}
			g.Summary = summary(path, pg)
		}
		out = append(out, g)
	}
	return out
}

// count is the number of records over all projects (-1 until loaded).
func (b *groupBase[T]) count() int {
	if !b.loaded {
		return -1
	}
	n := 0
	for _, g := range b.data {
		n += max(g.Total, len(g.Items))
	}
	return n
}

// cursorGroup returns the project path of the line under the cursor.
func (b *groupBase[T]) cursorGroup() string {
	g, _, _ := b.tbl.Cursor()
	if g == nil {
		return ""
	}
	return g.Key
}

// ref is the project of path with its ID when known.
func (b *groupBase[T]) ref(path string) gitlab.ProjectRef {
	ref := gitlab.ProjectRef{FullPath: path}
	if ov, ok := b.sh.overview(path); ok {
		ref.ID = ov.ID
	}
	return ref
}

func (b *groupBase[T]) schedulePoll() tea.Cmd {
	env := b.env()
	if !b.active || env.AuthLost {
		b.poll.Next()
		return nil
	}
	return common.PollAfter(env.WorkspacePoll(), b.poll.Next())
}

// activate loads the tab the first time and refreshes stale data.
func (b *groupBase[T]) activate() tea.Cmd {
	b.active = true
	env := b.env()
	switch {
	case !b.loaded && !b.loading:
		return b.load(len(b.data) == 0)
	case b.loaded && env.Now().Sub(b.updated) >= env.WorkspacePoll() && !env.AuthLost:
		return b.load(false)
	}
	return tea.Batch(b.fill(), b.schedulePoll())
}

func (b *groupBase[T]) deactivate() {
	b.active = false
	wasLoading := b.loading
	b.stop()
	if wasLoading {
		b.loaded = false // finish the load the next time the tab is shown
	}
}

// filterBar renders the filter line (or the open input).
func (b *groupBase[T]) filterBar(pairs [][2]string, extra string, errText func(error) string) string {
	env := b.env()
	s := env.Styles
	if b.fl.Focused() {
		lines := strings.Split(b.fl.View(), "\n")
		if err := b.fl.Err(); err != nil {
			return strings.Join(lines, "\n") + "\n" + s.Error.Render(errText(err))
		}
		lines[0] += "  " + s.Muted.Render(env.Msg.Project.FilterHint)
		return strings.Join(lines, "\n")
	}
	return common.FilterBar(s, pairs, extra, "", b.w)
}

// frame draws the preview box.
func (b *groupBase[T]) frame(lines ...string) string {
	s := b.env().Styles
	for i := range lines {
		lines[i] = " " + render.Clip(lines[i], b.w-4)
	}
	return common.Box(s, strings.Join(lines, "\n"), b.w, groupPreviewLns+2, common.BoxNormal)
}

// filterErr is the text of a filter parse error.
func (b *groupBase[T]) filterErr(keys []filterline.Key) func(error) string {
	return func(err error) string {
		m := b.env().Msg.Project
		var pe *filterline.ParseError
		if errors.As(err, &pe) {
			if pe.Kind == filterline.UnknownKey {
				names := make([]string, len(keys))
				for i, k := range keys {
					names[i] = k.Name
				}
				return m.FilterUnknown(pe.Key, strings.Join(names, ", "))
			}
			return m.FilterBad(pe.Key, pe.Value, strings.Join(pe.Allowed, ", "))
		}
		return err.Error()
	}
}
