package workspace

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

// pickedMsg carries what the P overlay applies to the tab that issued tok:
// the marked projects and the text of the input.
type pickedMsg struct {
	tok    common.Token
	marked map[string]bool
	text   string
}

// pickLine is one project of the P overlay list.
type pickLine struct {
	path    string
	matched []int // byte offsets of the matched characters in path
}

// projectPicker is the P overlay of the MRs and Pipelines tabs (SPEC
// §3.3): the GitLab projects of the workspace with marks, narrowed by a
// local fuzzy match. It asks GitLab nothing; enter sends pickedMsg, esc
// closes it without a change.
type projectPicker struct {
	env    *app.Env
	tok    common.Token
	tab    string   // "mrs" or "pipelines"
	all    []string // marked first, then by activity (fixed while open)
	marked map[string]bool
	input  textinput.Model
	lines  []pickLine
	sel    int
	listH  int // list lines shown by the last View (for half pages)
}

var _ app.Overlay = (*projectPicker)(nil)

// newPicker opens the P overlay over paths (in activity order) with the
// current marks and text.
func newPicker(env *app.Env, tok common.Token, tab string, paths []string, marked map[string]bool, text string) *projectPicker {
	p := &projectPicker{env: env, tok: tok, tab: tab, marked: map[string]bool{}}
	for _, path := range paths {
		if marked[path] {
			p.marked[path] = true
			p.all = append(p.all, path)
		}
	}
	for _, path := range paths {
		if !marked[path] {
			p.all = append(p.all, path)
		}
	}
	p.input = common.NewInput(env.Styles)
	p.input.Prompt = "› "
	p.input.SetValue(text)
	p.input.CursorEnd()
	p.input.Focus()
	p.rebuild()
	return p
}

func (p *projectPicker) rebuild() {
	q := strings.TrimSpace(p.input.Value())
	p.lines = p.lines[:0]
	if q == "" {
		for _, path := range p.all {
			p.lines = append(p.lines, pickLine{path: path})
		}
	} else {
		// Keep the list order (marked first, then by activity): the
		// lines do not jump while the user marks them.
		hits := map[int][]int{}
		for _, m := range fuzzy.Find(q, p.all) {
			hits[m.Index] = m.MatchedIndexes
		}
		for i, path := range p.all {
			if m, ok := hits[i]; ok {
				p.lines = append(p.lines, pickLine{path: path, matched: m})
			}
		}
	}
	p.sel = min(p.sel, max(0, len(p.lines)-1))
}

func (p *projectPicker) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case app.LangChangedMsg, app.ThemeChangedMsg:
		p.input.SetStyles(p.env.Styles.Input)
	case tea.KeyPressMsg:
		return p.key(msg)
	}
	return p, nil
}

func (p *projectPicker) any(int) bool { return true }

func (p *projectPicker) key(k tea.KeyPressMsg) (app.Overlay, tea.Cmd) {
	n := len(p.lines)
	switch k.String() {
	case "esc":
		return nil, nil
	case "enter":
		msg := pickedMsg{tok: p.tok, marked: p.marked, text: strings.TrimSpace(p.input.Value())}
		return nil, func() tea.Msg { return msg }
	case "down", "ctrl+n":
		p.sel = common.ListMove(n, p.sel, 1, p.any)
	case "up", "ctrl+p":
		p.sel = common.ListMove(n, p.sel, -1, p.any)
	case "ctrl+d", "pgdown": // never reach the input: ctrl+d deletes, ctrl+u erases
		p.sel = common.ListPage(n, p.sel, 1, p.listH, p.any)
	case "ctrl+u", "pgup":
		p.sel = common.ListPage(n, p.sel, -1, p.listH, p.any)
	case "space": // paths have no spaces: it marks, it is not typed
		if p.sel < n {
			path := p.lines[p.sel].path
			if p.marked[path] {
				delete(p.marked, path)
			} else {
				p.marked[path] = true
			}
		}
	case "ctrl+a":
		p.markVisible()
	default:
		before := p.input.Value()
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(k)
		if p.input.Value() != before {
			p.sel = 0
			p.rebuild()
		}
		return p, cmd
	}
	return p, nil
}

// markVisible marks every line of the list, or unmarks them all if they
// are all marked.
func (p *projectPicker) markVisible() {
	all := true
	for _, l := range p.lines {
		all = all && p.marked[l.path]
	}
	for _, l := range p.lines {
		if all {
			delete(p.marked, l.path)
		} else {
			p.marked[l.path] = true
		}
	}
}

func (p *projectPicker) View(width, height int) string {
	env := p.env
	s, m := env.Styles, env.Msg
	w := common.ListModalWidth(width)
	inner := w - 4
	p.input.SetWidth(max(1, inner-2))
	tab := m.Tabs.MRs
	if p.tab == "pipelines" {
		tab = m.Tabs.Pipelines
	}
	head := s.ModalTitle.Render(m.WS.PickTitle + " · " + tab)
	if n := len(p.marked); n > 0 {
		head += "  " + s.Muted.Render(m.WS.PickMarked(n))
	}
	head = lipgloss.PlaceHorizontal(inner, lipgloss.Left, head)

	listH := common.ListModalRows(height)
	p.listH = listH
	var rows []string
	for i := common.ListStart(p.sel, listH); i < len(p.lines) && len(rows) < listH; i++ {
		rows = append(rows, p.renderLine(i, inner))
	}
	if len(p.lines) == 0 {
		rows = append(rows, s.Muted.Render(m.WS.NothingFound))
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}
	return common.ListModal(s, w, head, p.input.View(), rows, m.WS.PickHints, m.Search.Counter(len(p.lines), len(p.all)))
}

func (p *projectPicker) renderLine(i, w int) string {
	s := p.env.Styles
	l := p.lines[i]
	sel := i == p.sel
	marker := "  "
	if sel {
		marker = s.Cursor.Render("› ")
	}
	box := "[ ]"
	if p.marked[l.path] {
		box = s.Accent.Render("[x]")
	}
	text := render.Clip(marker+box+" "+common.Highlight(s, l.path, l.matched), w)
	if sel {
		return s.RowSelected.Render(render.Pad(text, w))
	}
	return text
}
