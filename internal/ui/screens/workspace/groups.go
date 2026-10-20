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

const groupPreviewLns = 3 // lines inside the preview frame of the group tabs

// groupBase is the loading and layout shared by the MRs and Pipelines
// tabs: full loads over every project, partial reloads of some projects
// (after an action, the fast tick of running pipelines) and the grouped
// table with the filter line.
type groupBase[T any] struct {
	sh   *shared
	gen  common.Gen // full loads
	part common.Gen // partial reloads
	poll common.Gen
	tbl  *grouped.Model
	fl   *filterline.Model

	cancel, partCancel context.CancelFunc
	fetch              func(ctx context.Context, ps []gitlab.ProjectRef) <-chan gitlab.BatchResult[T]

	data    map[string]gitlab.ProjectGroup[T]
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

func newGroupBase[T any](sh *shared, keys []filterline.Key) *groupBase[T] {
	b := &groupBase[T]{sh: sh, gen: common.NewGen(), part: common.NewGen(), poll: common.NewGen(),
		data: map[string]gitlab.ProjectGroup[T]{}}
	b.tbl = grouped.New(sh.env.Styles)
	b.fl = filterline.New(sh.env.Styles, keys)
	b.tbl.SetMoreText(sh.env.Msg.WS.More)
	return b
}

func (b *groupBase[T]) env() *app.Env { return b.sh.env }

func (b *groupBase[T]) UpdatedAt() time.Time { return b.updated }
func (b *groupBase[T]) CapturesInput() bool  { return b.fl.Focused() }

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

// load starts a full load. reset drops the loaded groups (filter change).
func (b *groupBase[T]) load(reset bool) tea.Cmd {
	refs := b.sh.refsInOrder()
	if !b.sh.ready || len(refs) == 0 {
		return nil
	}
	if b.cancel != nil {
		b.cancel()
	}
	if b.partCancel != nil {
		b.partCancel()
	}
	b.part.Next()
	tok := b.gen.Next()
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel, b.loading, b.failed = cancel, true, false
	if reset {
		b.data = map[string]gitlab.ProjectGroup[T]{}
		b.loaded = false
	}
	return listen(tok, b.fetch(ctx, refs))
}

// reload asks again for some projects without disturbing a full load.
func (b *groupBase[T]) reload(refs []gitlab.ProjectRef) tea.Cmd {
	if len(refs) == 0 || b.loading {
		return nil
	}
	if b.partCancel != nil {
		b.partCancel()
	}
	tok := b.part.Next()
	ctx, cancel := context.WithCancel(context.Background())
	b.partCancel = cancel
	return listen(tok, b.fetch(ctx, refs))
}

// stop cancels the loads (the tab or the screen is no longer shown).
func (b *groupBase[T]) stop() {
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	if b.partCancel != nil {
		b.partCancel()
		b.partCancel = nil
	}
	if b.loading {
		b.gen.Next()
		b.loading = false
	}
	b.part.Next()
	b.poll.Next()
}

// onBatch merges a batch. It returns whether the message was ours, whether
// a run ended (full: the whole tab) and the command to go on.
func (b *groupBase[T]) onBatch(msg batchMsg[T]) (ours, done, full bool, cmd tea.Cmd) {
	full = b.gen.Valid(msg.tok)
	if !full && !b.part.Valid(msg.tok) {
		return false, false, false, nil
	}
	if msg.done {
		if full {
			b.loading, b.loaded, b.cancel = false, true, nil
			b.updated = b.env().Now()
		}
		return true, true, full, nil
	}
	var fail tea.Cmd
	for _, g := range msg.res.Groups {
		b.data[g.Project.FullPath] = g
		if g.Err != nil && !errors.Is(g.Err, gitlab.ErrNotFound) && !b.failed {
			b.failed = true
			fail = b.env().Fail(g.Err)
		}
	}
	return true, false, full, tea.Batch(fail, msg.next)
}

// groups builds the grouped rows in project order. row maps a record to
// its table row, summary describes a ready group, empty is the text of a group
// without records.
func (b *groupBase[T]) groups(row func(T) table.Row, summary func(path string, g gitlab.ProjectGroup[T]) string, empty string) []grouped.Group {
	env := b.env()
	m := env.Msg
	out := make([]grouped.Group, 0, len(b.sh.order))
	for _, path := range b.sh.order {
		g := grouped.Group{Key: path, Title: path}
		pg, ok := b.data[path]
		switch {
		case b.sh.noAccess(path) || (ok && errors.Is(pg.Err, gitlab.ErrNotFound)):
			g.State, g.Note = grouped.Failed, m.WS.NoAccess
		case ok && pg.Err != nil:
			g.State, g.Note = grouped.Failed, app.ErrorText(env, pg.Err)
		case !ok:
			g.State, g.Note = grouped.Loading, m.WS.Loading
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
	return b.schedulePoll()
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
