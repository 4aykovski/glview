package common

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/4aykovski/glview/internal/gitlab"
)

// ApproverRow is a line of the approvers overlay: a person (selectable) or
// a rule header / caption (not).
type ApproverRow struct {
	Rule   string      // identifies the rule of a person row; "" without rules
	User   gitlab.User // the person; zero for a header
	Person bool
	Render func(w int) string // the line without the cursor column, w columns wide
}

// ApproversModal is the list behind the A overlay (SPEC 3.5, "Оверлей A"):
// the selection and the drawing; keys and actions belong to the app overlay
// that wraps it. The cursor only stands on person rows.
type ApproversModal struct {
	rows  []ApproverRow
	sel   int
	listH int
}

// NewApproversModal returns the list with the first person selected.
func NewApproversModal(rows []ApproverRow) *ApproversModal {
	m := &ApproversModal{}
	m.SetRows(rows)
	return m
}

func (m *ApproversModal) person(i int) bool { return m.rows[i].Person }

func (m *ApproversModal) first() int {
	for i, r := range m.rows {
		if r.Person {
			return i
		}
	}
	return 0
}

func (m *ApproversModal) last() int {
	for i := len(m.rows) - 1; i >= 0; i-- {
		if m.rows[i].Person {
			return i
		}
	}
	return 0
}

// SetRows replaces the list. The selection stays on the same (rule,
// username) pair, else on the same username, else goes to the first person.
func (m *ApproversModal) SetRows(rows []ApproverRow) {
	cur, ok := m.Selected()
	m.rows = rows
	m.sel = m.first()
	if !ok {
		return
	}
	byName := -1
	for i, r := range rows {
		if !r.Person || r.User.Username != cur.User.Username {
			continue
		}
		if r.Rule == cur.Rule {
			m.sel = i
			return
		}
		if byName < 0 {
			byName = i
		}
	}
	if byName >= 0 {
		m.sel = byName
	}
}

// People is the number of person rows.
func (m *ApproversModal) People() int {
	n := 0
	for _, r := range m.rows {
		if r.Person {
			n++
		}
	}
	return n
}

// Selected is the person under the cursor.
func (m *ApproversModal) Selected() (ApproverRow, bool) {
	if m.sel < 0 || m.sel >= len(m.rows) || !m.rows[m.sel].Person {
		return ApproverRow{}, false
	}
	return m.rows[m.sel], true
}

// Move handles a navigation key and reports whether it was one.
func (m *ApproversModal) Move(key string) bool {
	n := len(m.rows)
	switch key {
	case "j", "down", "ctrl+n":
		m.sel = ListMove(n, m.sel, 1, m.person)
	case "k", "up", "ctrl+p":
		m.sel = ListMove(n, m.sel, -1, m.person)
	case "g", "home":
		m.sel = m.first()
	case "G", "end":
		m.sel = m.last()
	case "ctrl+d", "pgdown":
		m.sel = ListPage(n, m.sel, 1, m.listH, m.person)
	case "ctrl+u", "pgup":
		m.sel = ListPage(n, m.sel, -1, m.listH, m.person)
	default:
		return false
	}
	return true
}

// View draws the overlay on a screen of width x height.
func (m *ApproversModal) View(s *Styles, title, hints string, width, height int) string {
	w := ListModalWidth(width)
	inner := w - 4
	head := lipgloss.PlaceHorizontal(inner, lipgloss.Left, s.ModalTitle.Render(title))
	listH := ListModalRows(height)
	m.listH = listH
	var rows []string
	for i := ListStart(m.sel, listH); i < len(m.rows) && len(rows) < listH; i++ {
		rows = append(rows, m.row(s, i, inner))
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}
	counter := ""
	if n := m.People(); n > 0 {
		pos := 0
		for i := 0; i <= m.sel && i < len(m.rows); i++ {
			if m.rows[i].Person {
				pos++
			}
		}
		counter = fmt.Sprintf("%d/%d", pos, n)
	}
	return ListModalPlain(s, w, head, rows, hints, counter)
}

// row draws a header at the left edge and a person after the two columns
// of the cursor.
func (m *ApproversModal) row(s *Styles, i, w int) string {
	r := m.rows[i]
	if !r.Person {
		return r.Render(w)
	}
	marker := "  "
	sel := i == m.sel
	if sel {
		marker = s.Cursor.Render("› ")
	}
	line := marker + r.Render(w-2)
	if sel {
		line += strings.Repeat(" ", max(0, w-lipgloss.Width(line)))
		return s.RowSelected.Render(line)
	}
	return line
}
