// Package vartable is the KEY | VALUE | type table for CI variables, used
// by the job play form and the Run pipeline form.
package vartable

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

// Labels are the translated texts of the table.
type Labels struct {
	Key, Value, Type, Empty string
	TypeEnv, TypeFile       string
}

const (
	colKey = iota
	colValue
	colType
)

// Model is the table. In navigation mode a adds a row, d deletes it, enter
// edits the cell under the cursor and space switches the type; while a cell
// is edited the model captures all keys.
type Model struct {
	styles *common.Styles
	labels Labels
	rows   []gitlab.CIVariable
	cursor int
	col    int

	editing bool
	isNew   bool // the row being edited was just added
	input   textinput.Model
}

// New creates an empty table.
func New(s *common.Styles, l Labels) *Model {
	return &Model{styles: s, labels: l, input: common.NewInput(s)}
}

// SetStyles and SetLabels apply a new theme or language.
func (m *Model) SetStyles(s *common.Styles) { m.styles = s; m.input.SetStyles(s.Input) }
func (m *Model) SetLabels(l Labels)         { m.labels = l }

// CapturesInput reports whether a cell is being edited.
func (m *Model) CapturesInput() bool { return m.editing }

// Len is the number of rows.
func (m *Model) Len() int { return len(m.rows) }

// Vars returns the variables with a non-empty key.
func (m *Model) Vars() []gitlab.CIVariable {
	var out []gitlab.CIVariable
	for _, r := range m.rows {
		if strings.TrimSpace(r.Key) != "" {
			out = append(out, r)
		}
	}
	return out
}

// Add appends a row (used for suggestions) and puts the cursor on it.
func (m *Model) Add(key, value string, typ gitlab.VarType) {
	if typ == "" {
		typ = gitlab.VarEnv
	}
	m.rows = append(m.rows, gitlab.CIVariable{Key: key, Value: value, Type: typ})
	m.cursor = len(m.rows) - 1
}

// Set replaces the value of key, adding the row when it is missing.
func (m *Model) Set(key, value string) {
	for i := range m.rows {
		if m.rows[i].Key == key {
			m.rows[i].Value = value
			m.cursor = i
			return
		}
	}
	m.Add(key, value, gitlab.VarEnv)
}

// Has reports whether a row with key exists.
func (m *Model) Has(key string) bool {
	for _, r := range m.rows {
		if r.Key == key {
			return true
		}
	}
	return false
}

func (m *Model) startEdit(col int, isNew bool) tea.Cmd {
	m.col, m.editing, m.isNew = col, true, isNew
	switch col {
	case colKey:
		m.input.SetValue(m.rows[m.cursor].Key)
	default:
		m.input.SetValue(m.rows[m.cursor].Value)
	}
	m.input.CursorEnd()
	return m.input.Focus()
}

// Update handles a key press and reports whether the key was used.
func (m *Model) Update(msg tea.Msg) (used bool, cmd tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return false, nil
	}
	if m.editing {
		switch k.String() {
		case "esc":
			m.finishEdit(false)
		case "enter":
			if m.col == colKey && m.isNew {
				m.finishEdit(true)
				return true, m.startEdit(colValue, true)
			}
			m.finishEdit(true)
		default:
			m.input, cmd = m.input.Update(msg)
		}
		return true, cmd
	}
	switch k.String() {
	case "a":
		m.rows = append(m.rows, gitlab.CIVariable{Type: gitlab.VarEnv})
		m.cursor = len(m.rows) - 1
		return true, m.startEdit(colKey, true)
	case "d":
		if len(m.rows) > 0 {
			m.rows = append(m.rows[:m.cursor], m.rows[m.cursor+1:]...)
			m.cursor = max(0, min(m.cursor, len(m.rows)-1))
		}
		return true, nil
	case "j", "down":
		m.cursor = min(m.cursor+1, max(0, len(m.rows)-1))
		return true, nil
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
		return true, nil
	case "h", "left":
		m.col = max(m.col-1, 0)
		return true, nil
	case "l", "right":
		m.col = min(m.col+1, colType)
		return true, nil
	case "enter":
		if len(m.rows) == 0 {
			return true, nil
		}
		if m.col == colType {
			m.toggleType()
			return true, nil
		}
		return true, m.startEdit(m.col, false)
	case "space":
		if len(m.rows) > 0 {
			m.toggleType()
		}
		return true, nil
	}
	return false, nil
}

func (m *Model) toggleType() {
	r := &m.rows[m.cursor]
	if r.Type == gitlab.VarFile {
		r.Type = gitlab.VarEnv
	} else {
		r.Type = gitlab.VarFile
	}
}

func (m *Model) finishEdit(commit bool) {
	m.editing = false
	m.input.Blur()
	if commit && m.cursor < len(m.rows) {
		if m.col == colKey {
			m.rows[m.cursor].Key = strings.TrimSpace(m.input.Value())
		} else {
			m.rows[m.cursor].Value = m.input.Value()
		}
		return
	}
	// Cancelled edit of a fresh row: drop the empty row.
	if m.isNew && m.cursor < len(m.rows) && m.rows[m.cursor].Key == "" && m.rows[m.cursor].Value == "" {
		m.rows = append(m.rows[:m.cursor], m.rows[m.cursor+1:]...)
		m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	}
}

// View renders the table in the given width.
func (m *Model) View(width int) string {
	s := m.styles
	keyW := max(12, width/4)
	typeW := 8
	valW := max(10, width-keyW-typeW-6)
	line := func(k, v, t string, sel bool) string {
		row := fit(k, keyW) + "  " + fit(v, valW) + "  " + fit(t, typeW)
		if sel {
			return s.Accent.Render("› ") + row
		}
		return "  " + row
	}
	out := []string{s.ColumnHeader.Render("  " + fit(m.labels.Key, keyW) + "  " + fit(m.labels.Value, valW) + "  " + fit(m.labels.Type, typeW))}
	if len(m.rows) == 0 {
		out = append(out, "  "+s.Muted.Render(m.labels.Empty))
	}
	for i, r := range m.rows {
		k, v, t := r.Key, r.Value, m.labels.TypeEnv
		if r.Type == gitlab.VarFile {
			t = m.labels.TypeFile
		}
		if m.editing && i == m.cursor {
			in := m.input.View()
			switch m.col {
			case colKey:
				k = in
			default:
				v = in
			}
		} else if i == m.cursor && !m.editing {
			switch m.col {
			case colKey:
				k = s.Bold.Render(k)
			case colValue:
				v = s.Bold.Render(v)
			default:
				t = s.Bold.Render(t)
			}
		}
		out = append(out, line(k, v, t, i == m.cursor))
	}
	return strings.Join(out, "\n")
}

// fit clips s to w cells and pads it to exactly w.
func fit(s string, w int) string { return render.Pad(render.Clip(s, w), w) }
