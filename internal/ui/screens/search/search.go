// Package search is the project picker (p) and the global search (F):
// one overlay with an input and a sectioned list (SPEC §3.11, §3.12). The
// picker works without network; the global search adds a server request
// 300 ms after the last keystroke.
package search

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/screens/render"
	wsscreen "github.com/4aykovski/glview/internal/ui/screens/workspace"
)

const (
	debounce  = 300 * time.Millisecond
	minQuery  = 3 // GitLab does not search shorter strings
	timeout   = 15 * time.Second
	maxWidth  = 84
	maxHeight = 24

	defaultHalfPage = 5 // before the first View
)

type kind int

const (
	kindSection kind = iota // section title
	kindInfo                // a message line (nothing found, searching…)
	kindProject
	kindGroup
)

// line is one line of the list.
type line struct {
	kind    kind
	text    string // section title or message
	path    string
	matched []int // byte offsets of the matched characters in path
	entry   app.CatalogEntry
	note    string // "current", "local"
	group   gitlab.Group
	err     bool
}

func (l line) selectable() bool { return l.kind == kindProject || l.kind == kindGroup }

type (
	debounceMsg struct{ tok int }
	resultMsg   struct {
		tok      common.Token
		q        string
		projects []gitlab.Project
		groups   []gitlab.Group
		err      error
	}
)

// Overlay is the picker or the global search.
type Overlay struct {
	env    *app.Env
	global bool
	input  textinput.Model
	sel    int // index into lines (a selectable one)
	lines  []line
	total  int // picker: all known projects
	listH  int // lines of the list shown by the last View (for half pages)

	tok    int        // input version for the debounce
	gen    common.Gen // server requests
	cancel context.CancelFunc
	busy   bool
	q      string // query of the shown server results
	err    error
	remote []gitlab.Project
	groups []gitlab.Group
}

var _ app.Overlay = (*Overlay)(nil)

// New creates the picker (global false) or the global search.
func New(env *app.Env, global bool) *Overlay {
	o := &Overlay{env: env, global: global, gen: common.NewGen()}
	o.input = common.NewInput(env.Styles)
	o.input.Prompt = "› "
	o.input.Focus()
	o.rebuild()
	return o
}

// DebounceMsg returns the debounce tick that is due next (for tests).
func (o *Overlay) DebounceMsg() tea.Msg { return debounceMsg{tok: o.tok} }

// Lines returns the plain texts of the list lines (for tests).
func (o *Overlay) Lines() []string {
	out := make([]string, len(o.lines))
	for i, l := range o.lines {
		if l.selectable() {
			out[i] = l.path
		} else {
			out[i] = l.text
		}
	}
	return out
}

func (o *Overlay) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case debounceMsg:
		if msg.tok != o.tok {
			return o, nil // a newer keystroke follows
		}
		return o, o.request()
	case resultMsg:
		if !o.gen.Valid(msg.tok) {
			return o, nil
		}
		o.busy, o.cancel = false, nil
		o.q, o.remote, o.groups, o.err = msg.q, msg.projects, msg.groups, msg.err
		o.rebuild()
		return o, nil
	case app.LangChangedMsg, app.ThemeChangedMsg:
		o.input.SetStyles(o.env.Styles.Input)
		o.rebuild()
		return o, nil
	case tea.KeyPressMsg:
		return o.key(msg)
	}
	return o, nil
}

func (o *Overlay) key(k tea.KeyPressMsg) (app.Overlay, tea.Cmd) {
	switch k.String() {
	case "esc":
		o.stop()
		return nil, nil
	case "down", "ctrl+n":
		o.move(1)
		return o, nil
	case "up", "ctrl+p":
		o.move(-1)
		return o, nil
	case "ctrl+d", "pgdown": // never reach the input: ctrl+d deletes, ctrl+u erases
		o.page(1)
		return o, nil
	case "ctrl+u", "pgup":
		o.page(-1)
		return o, nil
	case "enter":
		return o.open()
	}
	before := o.input.Value()
	var cmd tea.Cmd
	o.input, cmd = o.input.Update(k)
	if o.input.Value() == before {
		return o, cmd
	}
	o.sel = 0
	o.rebuild()
	if !o.global {
		return o, cmd
	}
	o.tok++
	if o.cancel != nil { // the answer would be stale
		o.cancel()
		o.cancel = nil
		o.gen.Next()
		o.busy = false
	}
	if utf8.RuneCountInString(strings.TrimSpace(o.input.Value())) < minQuery {
		return o, cmd
	}
	tok := o.tok
	return o, tea.Batch(cmd, tea.Tick(debounce, func(time.Time) tea.Msg { return debounceMsg{tok: tok} }))
}

// request starts the server search for the current input.
func (o *Overlay) request() tea.Cmd {
	q := strings.TrimSpace(o.input.Value())
	if utf8.RuneCountInString(q) < minQuery || o.env.Client == nil {
		return nil
	}
	if o.cancel != nil {
		o.cancel()
	}
	tok := o.gen.Next()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	o.cancel, o.busy, o.err = cancel, true, nil
	o.rebuild()
	client := o.env.Client
	return func() tea.Msg {
		defer cancel()
		ps, gs, err := client.Search().Search(ctx, q)
		return resultMsg{tok: tok, q: q, projects: ps, groups: gs, err: err}
	}
}

func (o *Overlay) stop() {
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
	o.gen.Next()
}

func (o *Overlay) move(d int) {
	n := len(o.lines)
	for i := o.sel + d; i >= 0 && i < n; i += d {
		if o.lines[i].selectable() {
			o.sel = i
			return
		}
	}
}

// page moves the selection half a visible list down (d > 0) or up; section
// and message lines count. It lands on the nearest selectable line in the
// direction of the move, or before it at the end of the list.
func (o *Overlay) page(d int) {
	n := len(o.lines)
	if n == 0 {
		return
	}
	step := max(1, o.listH/2)
	if o.listH == 0 {
		step = defaultHalfPage
	}
	target := min(max(o.sel+d*step, 0), n-1)
	for _, dir := range []int{d, -d} {
		for i := target; i >= 0 && i < n; i += dir {
			if o.lines[i].selectable() {
				o.sel = i
				return
			}
		}
	}
}

// Selected returns the path of the selected line ("" if none, for tests).
func (o *Overlay) Selected() string {
	if l := o.selected(); l != nil {
		return l.path
	}
	return ""
}

// Input returns the text of the input (for tests).
func (o *Overlay) Input() string { return o.input.Value() }

func (o *Overlay) selected() *line {
	if o.sel >= 0 && o.sel < len(o.lines) && o.lines[o.sel].selectable() {
		return &o.lines[o.sel]
	}
	return nil
}

// open opens the selected project (on its MRs tab, with the known local
// folder) or the group as a Workspace above the start screen.
func (o *Overlay) open() (app.Overlay, tea.Cmd) {
	l := o.selected()
	if l == nil {
		return o, nil
	}
	o.stop()
	env := o.env
	if l.kind == kindGroup {
		return nil, app.OpenGroup(wsscreen.NewGroup(env, l.group))
	}
	scr := project.New(env, project.Options{Path: l.path, Repo: env.Catalog.LocalRepo(l.path), Tab: project.TabMRs})
	return nil, app.OpenProject(scr)
}

// known lists the catalog projects once each, in section order.
func (o *Overlay) known() []line {
	m := o.env.Msg.Search
	c := o.env.Catalog
	seen := map[string]bool{}
	var out []line
	add := func(section string, entries []app.CatalogEntry, note string) {
		for _, e := range entries {
			if e.Path == "" || seen[e.Path] {
				continue
			}
			seen[e.Path] = true
			out = append(out, line{kind: kindProject, path: e.Path, entry: e, note: note, text: section})
		}
	}
	if cur := c.Current(); cur != nil {
		add(m.SecRemote, []app.CatalogEntry{*cur}, m.Current)
	}
	add(m.SecWorkspace, c.Workspace(), "")
	add(m.SecStarred, c.Starred(), "")
	add(m.SecRecent, c.Recent(), "")
	return out
}

// match filters lines by a fuzzy match of q against the path; the best
// matches come first within a section.
func match(q string, all []line) []line {
	if q == "" {
		return all
	}
	paths := make([]string, len(all))
	for i, l := range all {
		paths[i] = l.path
	}
	var out []line
	for _, mt := range fuzzy.Find(q, paths) {
		l := all[mt.Index]
		l.matched = mt.MatchedIndexes
		out = append(out, l)
	}
	// Keep the section order, best match first inside a section.
	order := map[string]int{}
	for _, l := range all {
		if _, ok := order[l.text]; !ok {
			order[l.text] = len(order)
		}
	}
	sorted := make([]line, 0, len(out))
	for sec := 0; sec < len(order); sec++ {
		for _, l := range out {
			if order[l.text] == sec {
				sorted = append(sorted, l)
			}
		}
	}
	return sorted
}

func (o *Overlay) rebuild() {
	if o.global {
		o.rebuildGlobal()
	} else {
		o.rebuildPicker()
	}
	if o.selected() == nil {
		o.sel = 0
		if len(o.lines) > 0 && !o.lines[0].selectable() {
			o.move(1)
		}
	}
}

func (o *Overlay) rebuildPicker() {
	m := o.env.Msg.Search
	all := o.known()
	o.total = len(all)
	found := match(strings.TrimSpace(o.input.Value()), all)
	o.lines = o.lines[:0]
	sec := ""
	for _, l := range found {
		if l.text != sec {
			sec = l.text
			o.lines = append(o.lines, line{kind: kindSection, text: sec})
		}
		o.lines = append(o.lines, l)
	}
	if len(found) == 0 {
		o.lines = append(o.lines, line{kind: kindInfo, text: m.NothingFound})
	}
}

func (o *Overlay) rebuildGlobal() {
	m := o.env.Msg.Search
	q := strings.TrimSpace(o.input.Value())
	o.lines = o.lines[:0]
	if utf8.RuneCountInString(q) < minQuery {
		o.lines = append(o.lines, line{kind: kindInfo, text: m.TypeMore})
		return
	}
	local := match(q, o.known())
	seen := map[string]bool{}
	o.lines = append(o.lines, line{kind: kindSection, text: m.SecProjects})
	for _, l := range local {
		seen[l.path] = true
		l.note = m.Local
		o.lines = append(o.lines, l)
	}
	fresh := o.q == q
	if fresh {
		for _, p := range o.remote {
			if seen[p.FullPath] {
				continue
			}
			seen[p.FullPath] = true
			e := app.CatalogEntry{Path: p.FullPath, OpenMRs: -1, Activity: p.LastActivityAt, Starred: p.Starred}
			o.lines = append(o.lines, line{kind: kindProject, path: p.FullPath, entry: e, matched: matchIdx(q, p.FullPath)})
		}
	}
	switch {
	case o.busy:
		o.lines = append(o.lines, line{kind: kindInfo, text: m.Searching})
	case o.err != nil:
		o.lines = append(o.lines, line{kind: kindInfo, text: m.Failed(app.ErrorText(o.env, o.err)), err: true})
	case fresh && len(o.lines) == 1 && len(o.groups) == 0:
		o.lines = append(o.lines, line{kind: kindInfo, text: m.NothingGlobal})
	}
	if fresh && len(o.groups) > 0 {
		o.lines = append(o.lines, line{kind: kindSection, text: m.SecGroups})
		for _, g := range o.groups {
			o.lines = append(o.lines, line{kind: kindGroup, path: g.FullPath, group: g, matched: matchIdx(q, g.FullPath)})
		}
	}
}

func matchIdx(q, s string) []int {
	if ms := fuzzy.Find(q, []string{s}); len(ms) > 0 {
		return ms[0].MatchedIndexes
	}
	return nil
}

// ---- view ----

func (o *Overlay) View(width, height int) string {
	env := o.env
	s, m := env.Styles, env.Msg.Search
	w := min(maxWidth, width-4)
	inner := w - 4
	o.input.SetWidth(max(1, inner-2))
	title := m.PickerTitle
	hints := m.PickerHints
	if o.global {
		title, hints = m.GlobalTitle, m.GlobalHints
	}
	host := strings.TrimPrefix(strings.TrimPrefix(env.Host, "https://"), "http://")
	head := lipgloss.PlaceHorizontal(inner, lipgloss.Left, s.ModalTitle.Render(title)+"   "+s.Muted.Render(host))

	listH := max(3, min(maxHeight, height-4)-6)
	o.listH = listH
	start := 0
	if o.sel >= listH {
		start = o.sel - listH + 1
	}
	var rows []string
	for i := start; i < len(o.lines) && len(rows) < listH; i++ {
		rows = append(rows, o.renderLine(i, inner))
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}
	counter := ""
	if !o.global {
		shown := 0
		for _, l := range o.lines {
			if l.selectable() {
				shown++
			}
		}
		counter = m.Counter(shown, o.total)
	}
	foot := s.Muted.Render(hints)
	if counter != "" {
		gap := max(1, inner-lipgloss.Width(foot)-lipgloss.Width(counter))
		foot += strings.Repeat(" ", gap) + s.Muted.Render(counter)
	}
	body := head + "\n" + o.input.View() + "\n" + common.Rule(s, inner) + "\n" + strings.Join(rows, "\n") + "\n" + common.Rule(s, inner) + "\n" + foot
	return s.Modal.Width(w).Render(body)
}

func (o *Overlay) renderLine(i, w int) string {
	env := o.env
	s := env.Styles
	l := o.lines[i]
	switch l.kind {
	case kindSection:
		return s.Muted.Render(strings.ToUpper(l.text))
	case kindInfo:
		if l.err {
			return s.Error.Render(render.Clip(l.text, w))
		}
		return s.Muted.Render(render.Clip(l.text, w))
	}
	sel := i == o.sel
	marker := "  "
	if sel {
		marker = s.Cursor.Render("› ")
	}
	name := highlight(s, l.path, l.matched)
	meta := o.meta(l)
	text := marker + name
	if meta != "" {
		text += "  " + s.Muted.Render(meta)
	}
	text = render.Clip(text, w)
	if sel {
		return s.RowSelected.Render(render.Pad(text, w))
	}
	return text
}

// meta is "★ · 3 MR · 1h", "current" or "local".
func (o *Overlay) meta(l line) string {
	env := o.env
	m := env.Msg.Search
	var parts []string
	if l.note != "" {
		parts = append(parts, l.note)
	}
	if l.kind == kindGroup {
		return strings.Join(parts, " · ")
	}
	if l.entry.Starred {
		parts = append(parts, "★")
	}
	if l.entry.OpenMRs >= 0 {
		parts = append(parts, m.MRs(l.entry.OpenMRs))
	}
	if !l.entry.Activity.IsZero() {
		parts = append(parts, i18n.RelTimeShort(env.Lang, env.Now().Sub(l.entry.Activity)))
	}
	return strings.Join(parts, " · ")
}

// highlight renders path with the matched characters in Accent+Bold.
func highlight(s *common.Styles, path string, matched []int) string {
	if len(matched) == 0 {
		return path
	}
	hit := make(map[int]bool, len(matched))
	for _, i := range matched {
		hit[i] = true
	}
	style := s.Accent.Bold(true)
	var b strings.Builder
	for i, r := range path {
		if hit[i] {
			b.WriteString(style.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
