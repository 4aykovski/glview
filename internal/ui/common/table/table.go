// Package table is the list table of project mode: columns with priorities
// that disappear on narrow terminals, a cursor that follows the row ID, a
// loading/error tail row and an empty state.
package table

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/ui/common"
)

const (
	gap      = 2 // spaces between columns
	markerW  = 2 // "› " before the first column
	pageStep = 10
	ellipsis = "…"
	marker   = "› "
	noMarker = "  "
)

// Column describes one column. Titles and cells are already translated and
// (cells) styled by the caller.
type Column struct {
	Title    string
	Min, Max int  // width bounds; Max 0 = unbounded
	Flex     bool // takes the space left over, and shrinks first
	// Priority decides which columns survive a narrow terminal: the lowest
	// priority goes first (ties: the rightmost).
	Priority   int
	AlignRight bool
}

// Row is one table row. ID identifies the row across reloads.
type Row struct {
	ID    string
	Cells []string
	Muted bool
}

// TailKind is the state of the pseudo-row after the last real row.
type TailKind int

const (
	TailNone TailKind = iota
	TailLoading
	TailError
)

// Model is the table state.
type Model struct {
	styles *common.Styles
	cols   []Column
	rows   []Row

	cursor int // index into rows
	offset int // first visible line (rows and tail)
	w, h   int // h includes the header line

	tail     TailKind
	tailText string
	empty    string

	layout []int // widths of the visible columns, -1 = hidden
}

// New creates an empty table.
func New(s *common.Styles) *Model { return &Model{styles: s} }

// SetStyles switches the theme.
func (m *Model) SetStyles(s *common.Styles) { m.styles = s }

// SetColumns replaces the columns (language change).
func (m *Model) SetColumns(cols []Column) {
	m.cols = cols
	m.layout = nil
}

// SetSize sets the total area, header included.
func (m *Model) SetSize(w, h int) {
	m.w, m.h = w, h
	m.layout = nil
	m.clampView()
}

// SetEmpty sets the text shown when there are no rows.
func (m *Model) SetEmpty(text string) { m.empty = text }

// SetTail sets the pseudo-row after the last row.
func (m *Model) SetTail(kind TailKind, text string) {
	m.tail, m.tailText = kind, text
	m.clampView()
}

// SetRows replaces the rows, keeping the cursor on the same row ID when it
// is still present (otherwise the index is clamped).
func (m *Model) SetRows(rows []Row) {
	id := m.CursorID()
	m.rows = rows
	m.layout = nil
	if id != "" {
		if i := slices.IndexFunc(rows, func(r Row) bool { return r.ID == id }); i >= 0 {
			m.cursor = i
		}
	}
	m.cursor = min(m.cursor, max(0, len(rows)-1))
	m.clampView()
}

// Len returns the number of rows.
func (m *Model) Len() int { return len(m.rows) }

// Cursor returns the cursor index (0 when empty).
func (m *Model) Cursor() int { return m.cursor }

// CursorID returns the ID of the row under the cursor ("" when empty).
func (m *Model) CursorID() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	return m.rows[m.cursor].ID
}

// SetCursorID moves the cursor to the row with id, if present.
func (m *Model) SetCursorID(id string) bool {
	i := slices.IndexFunc(m.rows, func(r Row) bool { return r.ID == id })
	if i < 0 {
		return false
	}
	m.cursor = i
	m.clampView()
	return true
}

// NearEnd reports whether the cursor is within n rows of the last row.
func (m *Model) NearEnd(n int) bool {
	return len(m.rows) > 0 && m.cursor >= len(m.rows)-n
}

// HandleKey moves the cursor on j/k/↓/↑/g/G/pgup/pgdown and reports
// whether the cursor moved.
func (m *Model) HandleKey(k tea.KeyPressMsg) bool {
	before := m.cursor
	last := max(0, len(m.rows)-1)
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
	}
	m.clampView()
	return m.cursor != before
}

func (m *Model) pageSize() int { return max(1, m.bodyHeight()-1) }

// bodyHeight is the number of lines available for rows and the tail: the
// total minus the header and the rule under it.
func (m *Model) bodyHeight() int { return max(0, m.h-2) }

func (m *Model) lines() int {
	n := len(m.rows)
	if m.tail != TailNone {
		n++
	}
	return n
}

// clampView keeps the cursor inside the visible window.
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
	// Show the tail when the cursor is on the last row.
	if m.tail != TailNone && m.cursor == len(m.rows)-1 && m.cursor+1 >= m.offset+body {
		m.offset = m.cursor + 2 - body
	}
	m.offset = max(0, min(m.offset, max(0, m.lines()-body)))
}

// compute lays out the visible columns for the current width.
func (m *Model) compute() []int {
	if m.layout != nil {
		return m.layout
	}
	cells := make([][]string, len(m.rows))
	for i, r := range m.rows {
		cells[i] = r.Cells
	}
	m.layout = Layout(m.cols, cells, m.w)
	return m.layout
}

// Layout computes the widths of the columns for rows of cells in width
// cells (the row marker included): -1 marks a hidden column. Columns of the
// lowest priority disappear first when even their minimum widths do not
// fit; then flex columns, and after them the others, shrink to their
// minimum; spare width goes to the flex columns.
func Layout(cols []Column, rows [][]string, width int) []int {
	n := len(cols)
	widths := make([]int, n)
	visible := make([]bool, n)
	for i, c := range cols {
		visible[i] = true
		w := ansi.StringWidth(c.Title)
		for _, r := range rows {
			if i < len(r) {
				w = max(w, ansi.StringWidth(r[i]))
			}
		}
		w = max(w, c.Min)
		if c.Max > 0 {
			w = min(w, c.Max)
		}
		widths[i] = w
	}

	avail := width - markerW
	minTotal := func() int {
		total, cnt := 0, 0
		for i, c := range cols {
			if visible[i] {
				total += max(1, c.Min)
				cnt++
			}
		}
		if cnt > 1 {
			total += gap * (cnt - 1)
		}
		return total
	}
	for minTotal() > avail {
		drop := -1
		for i, c := range cols {
			if !visible[i] {
				continue
			}
			if drop < 0 || c.Priority <= cols[drop].Priority {
				drop = i
			}
		}
		if drop < 0 {
			break
		}
		visible[drop] = false
		if countTrue(visible) <= 1 {
			break
		}
	}

	total := func() int {
		t, cnt := 0, 0
		for i := range cols {
			if visible[i] {
				t += widths[i]
				cnt++
			}
		}
		if cnt > 1 {
			t += gap * (cnt - 1)
		}
		return t
	}
	// Too wide: shrink flex columns, then the others, never below Min.
	for pass := 0; pass < 2 && total() > avail; pass++ {
		for total() > avail {
			pick := -1
			for i, c := range cols {
				if !visible[i] || widths[i] <= max(1, c.Min) || (pass == 0 && !c.Flex) {
					continue
				}
				if pick < 0 || widths[i] > widths[pick] {
					pick = i
				}
			}
			if pick < 0 {
				break
			}
			widths[pick]--
		}
	}
	// Too narrow: hand the rest to the flex columns.
	if extra := avail - total(); extra > 0 {
		var flex []int
		for i, c := range cols {
			if visible[i] && c.Flex {
				flex = append(flex, i)
			}
		}
		for k := 0; extra > 0 && len(flex) > 0; k++ {
			widths[flex[k%len(flex)]]++
			extra--
		}
	}

	out := make([]int, n)
	for i := range cols {
		if visible[i] {
			out[i] = widths[i]
		} else {
			out[i] = -1
		}
	}
	return out
}

func countTrue(b []bool) int {
	n := 0
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

// Visible returns the titles of the columns that fit (for tests and hints).
func (m *Model) Visible() []string {
	var out []string
	for i, w := range m.compute() {
		if w >= 0 {
			out = append(out, m.cols[i].Title)
		}
	}
	return out
}

func fit(cell string, w int, right bool) string {
	cell = ansi.Truncate(cell, w, ellipsis)
	pad := w - ansi.StringWidth(cell)
	if pad <= 0 {
		return cell
	}
	if right {
		return strings.Repeat(" ", pad) + cell
	}
	return cell + strings.Repeat(" ", pad)
}

func (m *Model) line(cells []string, plain bool) string {
	return FormatLine(m.cols, m.compute(), cells, plain)
}

// FormatLine lays cells out in the widths from Layout (without the row
// marker); plain strips the cells' styles (selected and muted rows).
func FormatLine(cols []Column, widths []int, cells []string, plain bool) string {
	parts := make([]string, 0, len(widths))
	for i, w := range widths {
		if w < 0 {
			continue
		}
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		if plain {
			cell = ansi.Strip(cell)
		}
		parts = append(parts, fit(cell, w, cols[i].AlignRight))
	}
	return strings.Join(parts, strings.Repeat(" ", gap))
}

// View renders the header and the visible rows, exactly h lines.
func (m *Model) View() string {
	if m.h <= 0 || m.w <= 0 {
		return ""
	}
	s := m.styles
	titles := make([]string, len(m.cols))
	for i, c := range m.cols {
		titles[i] = c.Title
	}
	out := []string{s.ColumnHeader.Render(noMarker + m.line(titles, true)), common.Rule(s, m.w)}

	body := m.bodyHeight()
	switch {
	case len(m.rows) == 0 && m.tail == TailNone && m.empty != "":
		for _, l := range strings.Split(m.empty, "\n") {
			out = append(out, ansi.Truncate(l, m.w, ellipsis))
		}
	default:
		end := min(m.offset+body, m.lines())
		for i := m.offset; i < end; i++ {
			if i >= len(m.rows) {
				out = append(out, m.renderTail())
				continue
			}
			r := m.rows[i]
			switch {
			case i == m.cursor:
				out = append(out, s.RowSelected.Render(marker+m.line(r.Cells, true)))
			case r.Muted:
				out = append(out, s.RowMuted.Render(noMarker+m.line(r.Cells, true)))
			default:
				out = append(out, noMarker+m.line(r.Cells, false))
			}
		}
	}
	return common.FitHeight(strings.Join(out, "\n"), m.h)
}

func (m *Model) renderTail() string {
	text := ansi.Truncate(m.tailText, m.w-markerW, ellipsis)
	switch m.tail {
	case TailError:
		return noMarker + m.styles.Error.Render(text)
	default:
		return noMarker + m.styles.MoreRow.Render(text)
	}
}
