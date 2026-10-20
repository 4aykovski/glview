// Package grouped is the table of the workspace MRs and Pipelines tabs:
// records grouped by project under header lines, a "N more" line per
// group, collapsible groups and placeholder lines for groups that are
// still loading, empty or failed (SPEC §3.3). Columns are laid out by the
// same rules as the project-mode table (table.Layout).
package grouped

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/table"
)

const (
	marker   = "› "
	noMarker = "  "
	pageStep = 10
)

// State is the state of a group.
type State int

const (
	Ready   State = iota // records loaded (Rows may still be empty: see Empty)
	Loading              // the batch with this project has not arrived yet
	Empty                // no records: one collapsed line
	Failed               // the project failed (no access…)
)

// Group is one project of the table.
type Group struct {
	Key     string      // project path; unique
	Title   string      // plain title (the project path)
	Summary string      // styled text after the title ("6 open · showing 3")
	Rows    []table.Row // records; IDs unique within the group
	More    int         // records not shown ("N more")
	State   State
	Note    string // text of the Loading, Empty and Failed lines
}

// Kind is the kind of a line.
type Kind int

const (
	KindNone Kind = iota
	KindHeader
	KindRow
	KindMore
)

type line struct {
	kind  Kind
	group int
	row   int
	id    string
}

// Model is the grouped table.
type Model struct {
	styles    *common.Styles
	cols      []table.Column
	groups    []Group
	collapsed map[string]bool
	moreText  func(n int) string

	lines  []line
	cursor int
	offset int
	w, h   int // h includes the column header and the rule
	layout []int
}

// New creates an empty table.
func New(s *common.Styles) *Model {
	return &Model{styles: s, collapsed: map[string]bool{}, moreText: func(int) string { return "" }}
}

// SetStyles switches the theme.
func (m *Model) SetStyles(s *common.Styles) { m.styles = s }

// SetColumns replaces the columns (language change).
func (m *Model) SetColumns(cols []table.Column) {
	m.cols = cols
	m.layout = nil
}

// SetMoreText sets the text of the "N more" lines.
func (m *Model) SetMoreText(f func(n int) string) { m.moreText = f }

// SetSize sets the total area, the column header included.
func (m *Model) SetSize(w, h int) {
	m.w, m.h = w, h
	m.layout = nil
	m.clampView()
}

// Groups returns the groups (for tests).
func (m *Model) Groups() []Group { return m.groups }

// SetGroups replaces the groups. The cursor stays on the same line ID;
// when that line is gone it moves to its group's header, and to the first
// line when the group is gone too. Collapsed groups stay collapsed.
func (m *Model) SetGroups(groups []Group) {
	id := m.CursorID()
	m.groups = groups
	m.layout = nil
	m.rebuild()
	m.restore(id)
}

func headerID(key string) string { return "group/" + key }
func moreID(key string) string   { return "more/" + key }
func rowID(key, id string) string {
	return "row/" + key + "/" + id
}

// groupOf extracts the group key from a line ID.
func groupOf(id string) string {
	kind, rest, ok := strings.Cut(id, "/")
	if !ok {
		return ""
	}
	if kind == "row" {
		if i := strings.LastIndexByte(rest, '/'); i >= 0 {
			return rest[:i]
		}
	}
	return rest
}

// expanded reports whether the group shows its records.
func (m *Model) expanded(g Group) bool {
	return g.State == Ready && len(g.Rows) > 0 && !m.collapsed[g.Key]
}

func (m *Model) rebuild() {
	m.lines = m.lines[:0]
	for gi, g := range m.groups {
		m.lines = append(m.lines, line{kind: KindHeader, group: gi, row: -1, id: headerID(g.Key)})
		if !m.expanded(g) {
			continue
		}
		for ri, r := range g.Rows {
			m.lines = append(m.lines, line{kind: KindRow, group: gi, row: ri, id: rowID(g.Key, r.ID)})
		}
		if g.More > 0 {
			m.lines = append(m.lines, line{kind: KindMore, group: gi, row: -1, id: moreID(g.Key)})
		}
	}
}

func (m *Model) restore(id string) {
	if id != "" {
		if i := slices.IndexFunc(m.lines, func(l line) bool { return l.id == id }); i >= 0 {
			m.cursor = i
			m.clampView()
			return
		}
		key := groupOf(id)
		if i := slices.IndexFunc(m.lines, func(l line) bool { return l.id == headerID(key) }); i >= 0 {
			m.cursor = i
			m.clampView()
			return
		}
	}
	m.cursor = min(m.cursor, max(0, len(m.lines)-1))
	if id != "" {
		m.cursor = 0
	}
	m.clampView()
}

// CursorID returns the ID of the line under the cursor ("" when empty).
func (m *Model) CursorID() string {
	if m.cursor < 0 || m.cursor >= len(m.lines) {
		return ""
	}
	return m.lines[m.cursor].id
}

// SetCursorID moves the cursor to the line with id, if present.
func (m *Model) SetCursorID(id string) bool {
	i := slices.IndexFunc(m.lines, func(l line) bool { return l.id == id })
	if i < 0 {
		return false
	}
	m.cursor = i
	m.clampView()
	return true
}

// Top moves the cursor to the first line.
func (m *Model) Top() {
	m.cursor = 0
	m.clampView()
}

// RowID returns the line ID of record id of group key (for SetCursorID).
func RowID(key, id string) string { return rowID(key, id) }

// HeaderID returns the line ID of the header of group key.
func HeaderID(key string) string { return headerID(key) }

// Cursor returns the group under the cursor, the record (nil on a header
// or a "more" line) and the kind of the line.
func (m *Model) Cursor() (*Group, *table.Row, Kind) {
	if m.cursor < 0 || m.cursor >= len(m.lines) {
		return nil, nil, KindNone
	}
	l := m.lines[m.cursor]
	g := &m.groups[l.group]
	if l.kind == KindRow {
		return g, &g.Rows[l.row], KindRow
	}
	return g, nil, l.kind
}

// Collapsed reports whether the group is collapsed by the user.
func (m *Model) Collapsed(key string) bool { return m.collapsed[key] }

// Toggle collapses or expands the group under the cursor; the cursor moves
// to its header.
func (m *Model) Toggle() {
	g, _, _ := m.Cursor()
	if g == nil || g.State != Ready || len(g.Rows) == 0 {
		return
	}
	m.collapsed[g.Key] = !m.collapsed[g.Key]
	key := g.Key
	m.rebuild()
	m.SetCursorID(headerID(key))
}

// HandleKey moves the cursor (j/k/g/G/pgup/pgdown, J/K between group
// headers) and toggles the group on space. It reports whether the key was
// handled.
func (m *Model) HandleKey(k tea.KeyPressMsg) bool {
	last := max(0, len(m.lines)-1)
	switch k.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, last)
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = last
	case "pgdown", "ctrl+d":
		m.cursor = min(m.cursor+m.pageSize(), last)
	case "pgup", "ctrl+u":
		m.cursor = max(m.cursor-m.pageSize(), 0)
	case "J":
		for i := m.cursor + 1; i < len(m.lines); i++ {
			if m.lines[i].kind == KindHeader {
				m.cursor = i
				break
			}
		}
	case "K":
		for i := m.cursor - 1; i >= 0; i-- {
			if m.lines[i].kind == KindHeader {
				m.cursor = i
				break
			}
		}
	case "space":
		m.Toggle()
	default:
		return false
	}
	m.clampView()
	return true
}

func (m *Model) pageSize() int   { return max(1, m.bodyHeight()-1) }
func (m *Model) bodyHeight() int { return max(0, m.h-2) }

func (m *Model) clampView() {
	body := m.bodyHeight()
	if body == 0 {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+body {
		m.offset = m.cursor - body + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.lines)-body)))
}

func (m *Model) compute() []int {
	if m.layout == nil {
		var cells [][]string
		for _, g := range m.groups {
			for _, r := range g.Rows {
				cells = append(cells, r.Cells)
			}
		}
		m.layout = table.Layout(m.cols, cells, m.w)
	}
	return m.layout
}

// Visible returns the titles of the columns that fit (for tests).
func (m *Model) Visible() []string {
	var out []string
	for i, w := range m.compute() {
		if w >= 0 {
			out = append(out, m.cols[i].Title)
		}
	}
	return out
}

// View renders the column header and the visible lines, exactly h lines.
func (m *Model) View() string {
	if m.h <= 0 || m.w <= 0 {
		return ""
	}
	s := m.styles
	widths := m.compute()
	titles := make([]string, len(m.cols))
	for i, c := range m.cols {
		titles[i] = c.Title
	}
	out := []string{s.ColumnHeader.Render(noMarker + table.FormatLine(m.cols, widths, titles, true)), common.Rule(s, m.w)}
	end := min(m.offset+m.bodyHeight(), len(m.lines))
	for i := m.offset; i < end; i++ {
		out = append(out, m.renderLine(i, widths))
	}
	return common.FitHeight(strings.Join(out, "\n"), m.h)
}

func (m *Model) renderLine(i int, widths []int) string {
	s := m.styles
	l := m.lines[i]
	g := m.groups[l.group]
	sel := i == m.cursor
	var text string
	switch l.kind {
	case KindHeader:
		text = m.header(g, sel)
	case KindRow:
		r := g.Rows[l.row]
		switch {
		case sel:
			return s.RowSelected.Render(ansi.Truncate(marker+table.FormatLine(m.cols, widths, r.Cells, true), m.w, "…"))
		case r.Muted:
			return s.RowMuted.Render(noMarker + table.FormatLine(m.cols, widths, r.Cells, true))
		}
		return noMarker + table.FormatLine(m.cols, widths, r.Cells, false)
	case KindMore:
		text = m.moreText(g.More)
		if !sel {
			text = s.MoreRow.Render(text)
		}
	}
	if sel {
		return s.RowSelected.Render(ansi.Truncate(marker+ansi.Strip(text), m.w, "…"))
	}
	return ansi.Truncate(noMarker+text, m.w, "…")
}

func (m *Model) header(g Group, plain bool) string {
	s := m.styles
	var glyph, note string
	switch g.State {
	case Loading:
		glyph, note = s.StatusStyle("running").Render("◐"), s.Muted.Render(g.Note)
	case Failed:
		glyph, note = s.Error.Render("✗"), s.Error.Render(g.Note)
	case Empty:
		glyph, note = "▸", s.Muted.Render(g.Note)
	default:
		glyph = "▾"
		if len(g.Rows) == 0 || m.collapsed[g.Key] {
			glyph = "▸"
		}
		note = s.Muted.Render(g.Summary)
		if g.Summary != "" && plain {
			note = g.Summary
		}
	}
	text := glyph + " " + s.GroupHeader.Render(g.Title)
	if note != "" {
		text += "  " + note
	}
	return text
}
