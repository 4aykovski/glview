package workspace

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/filterline"
	"github.com/4aykovski/glview/internal/ui/common/grouped"
	"github.com/4aykovski/glview/internal/ui/common/table"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/screens/render"
	"github.com/4aykovski/glview/internal/ui/screens/tagactions"
)

// tagsTab is the Tags tab: the first per_project tags of every expanded
// project (newest version first) with the pipelines of the tags. The text
// search is on the server; a non-empty one is the filter mode. The "me"
// filter and the status cycle do not apply to tags.
type tagsTab struct {
	*groupBase[gitlab.Tag]
	search string
	cmp    *tagactions.Comparer

	// wantPath/wantTag: put the cursor on this tag once it is listed (a
	// tag just created).
	wantPath, wantTag string

	// dirty: projects whose tags changed while the tab was covered; they
	// are asked again on activation.
	dirty map[string]bool
}

func newTagsTab(sh *shared) *tagsTab {
	t := &tagsTab{cmp: tagactions.NewComparer()}
	t.groupBase = newGroupBase[gitlab.Tag](sh, "tags", nil)
	t.narrow = func() bool { return t.search != "" }
	t.fetch = func(ctx context.Context, ps []gitlab.ProjectRef) <-chan gitlab.BatchResult[gitlab.Tag] {
		return sh.env.Client.Workspace().Tags(ctx, ps, t.search, perProject(sh.env))
	}
	t.Rebuild()
	return t
}

// Count is the number of tags found by a search; collapsed groups have no
// count.
func (t *tagsTab) Count() int {
	if t.search == "" {
		return -1
	}
	return t.count()
}

func (t *tagsTab) Status() string { return t.env().Msg.WS.StatusGroups }

func (t *tagsTab) Activate() tea.Cmd {
	cmd := t.activate()
	var reload tea.Cmd
	if !t.loading { // a full load already asks them all
		var refs []gitlab.ProjectRef
		for path := range t.dirty {
			refs = append(refs, t.ref(path))
		}
		reload = t.reload(refs)
	}
	t.dirty = nil
	return tea.Batch(cmd, reload, t.loadCompare())
}

func (t *tagsTab) Deactivate() {
	t.deactivate()
	t.cmp.Stop()
}

// Refresh is r: the comparisons are asked again past the cache.
func (t *tagsTab) Refresh() tea.Cmd { return t.refreshCmp(true) }

// refreshCmp reloads the groups and marks the comparisons stale; they keep
// their numbers until the new answers come. force: past the cache.
func (t *tagsTab) refreshCmp(force bool) tea.Cmd {
	t.cmp.Expire(force)
	return t.refresh()
}

// removeTag drops a deleted tag from the loaded group: the reload merges
// nothing, but the shown list must not wait for it.
func (t *tagsTab) removeTag(path, name string) {
	g, ok := t.data[path]
	if !ok {
		return
	}
	i := slices.IndexFunc(g.Items, func(tag gitlab.Tag) bool { return tag.Name == name })
	if i < 0 {
		return
	}
	g.Items = slices.Delete(slices.Clone(g.Items), i, i+1)
	if g.Total > 0 {
		g.Total--
	}
	t.data[path] = g
}

func (t *tagsTab) Rebuild() {
	env := t.env()
	s, m := env.Styles, env.Msg
	t.tbl.SetStyles(s)
	t.fl.SetStyles(s)
	t.tbl.SetMoreText(m.WS.More)
	t.tbl.SetColumns([]table.Column{
		{Title: m.Tag.ColTag, Min: 10, Max: 28, Priority: 100},
		{Title: m.MR.ColCommit, Min: 7, Max: 8, Priority: 60},
		{Title: m.Tag.ColMessage, Min: 16, Flex: true, Priority: 90},
		{Title: m.MR.ColPipeline, Min: 10, Max: 14, Priority: 70},
		{Title: m.MR.ColAgo, Min: 4, Priority: 50, AlignRight: true},
	})
	t.setGroups(t.groups(t.row, t.summary, t.head, m.Tag.GroupEmpty))
	t.layout()
	if t.wantTag != "" && t.tbl.SetCursorID(grouped.RowID(t.wantPath, t.wantTag)) {
		t.wantPath, t.wantTag = "", ""
		t.moved = true
	}
}

func (t *tagsTab) row(tag gitlab.Tag) table.Row {
	env := t.env()
	s, tm := env.Styles, env.Msg.Tag
	msg := tagactions.Message(tag)
	if tag.Release != nil {
		msg += " " + s.Accent.Render(tm.ReleaseMark)
	}
	pipe := s.Muted.Render("—")
	if p := tag.Pipeline; p != nil {
		pipe = s.StatusGlyph(p.Status) + " " + s.Accent.Render(fmt.Sprintf("#%d", p.ID))
	}
	return table.Row{ID: tag.Name, Cells: []string{
		tag.Name, s.Muted.Render(tag.Commit.ShortSHA), msg, pipe, render.Ago(env.Lang, env.Now(), tag.Commit.CreatedAt),
	}}
}

func (t *tagsTab) summary(_ string, g gitlab.ProjectGroup[gitlab.Tag]) string {
	env := t.env()
	parts := []string{env.Msg.Plural.Tags(max(g.Total, len(g.Items)))}
	if g.Total > len(g.Items) {
		parts = append(parts, env.Msg.WS.GroupShown(len(g.Items)))
	}
	return strings.Join(parts, " · ")
}

// head is the header of a collapsed group: the latest tag from the
// overview, "no tags" when the overview knows there are none.
func (t *tagsTab) head(path string) string {
	ov, ok := t.sh.overview(path)
	switch {
	case !ok:
		return ""
	case ov.LastTag != "":
		return ov.LastTag
	case ov.TagKnown:
		return t.env().Msg.Tag.GroupEmpty
	}
	return ""
}

// toggle collapses or expands the group under the cursor.
func (t *tagsTab) toggle() tea.Cmd {
	path, cmd := t.groupBase.toggle()
	t.Rebuild()
	t.onToggled(path)
	return tea.Batch(cmd, t.loadCompare())
}

func (t *tagsTab) selected() *gitlab.Tag {
	g, r, kind := t.tbl.Cursor()
	if kind != grouped.KindRow {
		return nil
	}
	items := t.data[g.Key].Items
	for i := range items {
		if items[i].Name == r.ID {
			return &items[i]
		}
	}
	return nil
}

// archived reports a project that cannot get or lose tags.
func (t *tagsTab) archived(path string) bool {
	ov, ok := t.sh.overview(path)
	return ok && ov.Archived
}

// loadCompare asks the comparison of the selected tag with the default
// branch of its project (from the overview).
func (t *tagsTab) loadCompare() tea.Cmd {
	tag := t.selected()
	if tag == nil || !t.active {
		return nil
	}
	path := t.cursorGroup()
	ov, ok := t.sh.overview(path)
	if !ok || ov.DefaultBranch == "" {
		return nil
	}
	return t.cmp.Load(t.env(), t.ref(path), tag.Name, ov.DefaultBranch)
}

func (t *tagsTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	env := t.env()
	if cmd := t.fl.HandleMsg(msg); cmd != nil {
		t.layout()
		return t, cmd
	}
	switch msg := msg.(type) {
	case batchMsg[gitlab.Tag]:
		ours, done, full, cmd := t.onBatch(msg)
		if !ours {
			return t, nil
		}
		t.Rebuild()
		if done {
			cmds := []tea.Cmd{t.loadCompare()}
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
		return t, tea.Batch(cmd, t.loadCompare())
	case tagactions.CompareMsg:
		t.cmp.OnMsg(msg)
		return t, nil
	case common.PollTickMsg:
		if t.active && !env.AuthLost && t.poll.Valid(msg.Token) {
			return t, t.refreshCmp(false)
		}
	case tagactions.DeletedMsg:
		// Only the tab that asked toasts; the others follow the change.
		own := t.seq.Owns(msg.Owner)
		var cmd tea.Cmd
		if own {
			cmd = tagactions.Done(env, msg)
		}
		if msg.Err != nil {
			return t, cmd
		}
		path := msg.Ref.FullPath
		t.sh.forgetLastTag(path, msg.Name)
		t.removeTag(path, msg.Name)
		t.cmp.Reset()
		t.Rebuild()
		return t, tea.Batch(cmd, t.followChange(own, path))
	case tagactions.CreatedMsg:
		path := msg.Ref.FullPath
		own := t.seq.Owns(msg.Owner)
		if own {
			tagactions.Created(env, msg)
			t.wantPath, t.wantTag = path, msg.Tag.Name
		}
		t.sh.setLastTag(path, msg.Tag.Name)
		t.cmp.Reset()
		t.Rebuild()
		return t, t.followChange(own, path)
	case app.AuthLostMsg:
		t.poll.Next()
	case app.AuthRestoredMsg:
		if t.active {
			return t, t.load(false)
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		t.Rebuild()
	case tea.KeyPressMsg:
		return t, t.onKey(msg)
	}
	return t, nil
}

// followChange asks the project again after its tags changed: at once when
// the tab asked for the change or is shown, on return otherwise.
func (t *tagsTab) followChange(own bool, path string) tea.Cmd {
	if !own && !t.active {
		if _, ok := t.data[path]; ok {
			if t.dirty == nil {
				t.dirty = map[string]bool{}
			}
			t.dirty[path] = true
		}
		return nil
	}
	return t.reload([]gitlab.ProjectRef{t.ref(path)})
}

func (t *tagsTab) onKey(k tea.KeyPressMsg) tea.Cmd {
	env := t.env()
	if t.fl.Focused() {
		res, cmd := t.fl.Update(k)
		t.layout()
		if res == filterline.Applied {
			return tea.Batch(cmd, t.applySearch())
		}
		return cmd
	}
	if t.handleKey(k) {
		return t.loadCompare()
	}
	path := t.cursorGroup()
	switch k.String() {
	case "space":
		return t.toggle()
	case "P":
		return t.openP()
	case "/":
		cmd := t.fl.Open(t.search)
		t.layout()
		return cmd
	case "backspace":
		t.search = ""
		save := t.resetP()
		return tea.Batch(save, t.refilter())
	case "f":
		if path == "" {
			return nil
		}
		return t.sh.openProject(path, t.sh.primaryRepo(path), project.TabTags, nil, nil, t.search)
	case "o":
		if path != "" {
			return app.OpenURL(env, tagactions.TagsURL(t.sh.projectURL(path), t.search))
		}
	case "n":
		if path != "" && !t.archived(path) {
			ov, _ := t.sh.overview(path)
			return tagactions.NewTag(env, t.seq.Current(), t.ref(path), ov.DefaultBranch)
		}
	}
	if _, _, kind := t.tbl.Cursor(); kind == grouped.KindHeader && k.String() == "enter" {
		return t.toggle()
	}
	tag := t.selected()
	if tag == nil {
		return nil
	}
	ref := t.ref(path)
	switch k.String() {
	case "enter":
		return tagactions.OpenPipeline(env, ref, *tag)
	case "y":
		return tagactions.Copy(env, *tag)
	case "D":
		if !t.archived(path) {
			return tagactions.Delete(env, t.seq.Current(), ref, *tag)
		}
	}
	return nil
}

func (t *tagsTab) refilter() tea.Cmd {
	t.refilterMode()
	t.cmp.Reset()
	cmd := t.load(true)
	t.Rebuild()
	return cmd
}

func (t *tagsTab) applySearch() tea.Cmd {
	_, free, err := t.fl.Parsed()
	if err != nil {
		return nil
	}
	t.search = strings.TrimSpace(free)
	return t.refilter()
}

func (t *tagsTab) View() string {
	parts := []string{t.filterLine(), t.tbl.View()}
	if t.previewShown() {
		parts = append(parts, t.preview())
	}
	return strings.Join(parts, "\n")
}

// filterLine is "/ search  me: not applied to tags  [P chip]" and on the
// right "per project: 3 · sort: version ↓" with how many projects are shown.
func (t *tagsTab) filterLine() string {
	env := t.env()
	s, m := env.Styles, env.Msg.Tag
	if line, ok := t.filterInput(t.filterErr(nil)); ok {
		return line
	}
	left := s.Accent.Render("/") + " " + s.FilterKey.Render(m.SearchLabel)
	if t.search != "" {
		left += s.FilterKey.Render(":") + " " + s.FilterValue.Render(t.search)
	}
	left += "  " + s.Muted.Render(m.NoMine)
	if chip := t.chip(); chip != "" {
		left += "  " + chip
	}
	right := m.PerProject(perProject(env)) + " · " + m.SortVersion
	if info := t.shownInfo(); info != "" {
		right = info + "  " + right
	}
	return common.FilterBarText(s, left, right, t.w)
}

func (t *tagsTab) preview() string {
	env := t.env()
	tag := t.selected()
	if tag == nil {
		return t.frame("", "", "")
	}
	path := t.cursorGroup()
	ov, _ := t.sh.overview(path)
	d := tagactions.PreviewData{Tag: *tag, Branch: ov.DefaultBranch}
	if e, ok := t.cmp.Entry(t.ref(path), tag.Name); ok {
		d.State, d.Ahead = e.State, e.Ahead
	} else if d.Branch != "" {
		d.State = tagactions.CompareLoading
	}
	if t.search == "" {
		d.Suggest = tagactions.Suggested(t.data[path].Items)
	}
	return t.frame(tagactions.Preview(env, d)...)
}

func (t *tagsTab) KeyMap() common.KeyMap {
	env := t.env()
	k, w, tm := env.Msg.Keys, env.Msg.WS, env.Msg.Tag
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"J", "K"}, "J/K", w.KeyGroups, common.PriorityNormal),
		common.NewBinding([]string{"space"}, "space", w.KeyCollapse, common.PriorityLow),
		common.NewBinding([]string{"enter"}, "", tm.KeyPipeline, common.PriorityHigh),
		common.NewBinding([]string{"f"}, "", w.KeyProject, common.PriorityHigh),
		common.NewBinding([]string{"n"}, "", tm.KeyNew, common.PriorityNormal),
		common.NewBinding([]string{"D"}, "", tm.KeyDelete, common.PriorityNormal),
		common.NewBinding([]string{"/"}, "", tm.KeySearch, common.PriorityNormal),
		common.NewBinding([]string{"P"}, "", w.KeyProjects, common.PriorityLow),
		common.NewBinding([]string{"backspace"}, "⌫", k.Reset, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", tm.KeyCopyName, common.PriorityLow),
		common.NewBinding([]string{"o"}, "", k.Browser, common.PriorityLow),
	}}
}
