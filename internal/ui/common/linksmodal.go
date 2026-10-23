package common

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
)

// LinksModal is the list of the MR links behind the L overlay (SPEC 3.5,
// "MR links"): the selection and the drawing. Keys and actions (open, copy)
// belong to the app overlay that wraps it.
type LinksModal struct {
	links []gitlab.Link
	sel   int
	listH int // list lines shown by the last View (for half pages)
}

// NewLinksModal returns the list over links with the first one selected.
func NewLinksModal(links []gitlab.Link) *LinksModal {
	return &LinksModal{links: links}
}

// SetLinks replaces the list, keeping the selection on the same URL when
// it is still there.
func (m *LinksModal) SetLinks(links []gitlab.Link) {
	var cur string
	if l, ok := m.Selected(); ok {
		cur = l.URL
	}
	m.links = links
	m.sel = min(m.sel, max(0, len(links)-1))
	for i, l := range links {
		if l.URL == cur {
			m.sel = i
			break
		}
	}
}

// Len is the number of links.
func (m *LinksModal) Len() int { return len(m.links) }

// Selected is the link under the cursor.
func (m *LinksModal) Selected() (gitlab.Link, bool) {
	if m.sel < 0 || m.sel >= len(m.links) {
		return gitlab.Link{}, false
	}
	return m.links[m.sel], true
}

func (m *LinksModal) any(int) bool { return true }

// Move handles a navigation key and reports whether it was one.
func (m *LinksModal) Move(key string) bool {
	n := len(m.links)
	switch key {
	case "j", "down", "ctrl+n":
		m.sel = ListMove(n, m.sel, 1, m.any)
	case "k", "up", "ctrl+p":
		m.sel = ListMove(n, m.sel, -1, m.any)
	case "g", "home":
		m.sel = 0
	case "G", "end":
		m.sel = max(0, n-1)
	case "ctrl+d", "pgdown":
		m.sel = ListPage(n, m.sel, 1, m.listH, m.any)
	case "ctrl+u", "pgup":
		m.sel = ListPage(n, m.sel, -1, m.listH, m.any)
	default:
		return false
	}
	return true
}

// View draws the overlay on a screen of width x height.
func (m *LinksModal) View(s *Styles, title, hints string, width, height int) string {
	w := ListModalWidth(width)
	inner := w - 4
	head := lipgloss.PlaceHorizontal(inner, lipgloss.Left, s.ModalTitle.Render(title))
	listH := ListModalRows(height)
	m.listH = listH
	var rows []string
	for i := ListStart(m.sel, listH); i < len(m.links) && len(rows) < listH; i++ {
		rows = append(rows, m.row(s, i, inner))
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}
	counter := ""
	if len(m.links) > 0 {
		counter = fmt.Sprintf("%d/%d", m.sel+1, len(m.links))
	}
	return ListModalPlain(s, w, head, rows, hints, counter)
}

// row draws "text — url" (url muted), or just url when the text is the
// URL, cut with "…" on the right to w columns.
func (m *LinksModal) row(s *Styles, i, w int) string {
	l := m.links[i]
	sel := i == m.sel
	marker := "  "
	if sel {
		marker = s.Cursor.Render("› ")
	}
	room := max(1, w-lipgloss.Width(marker))
	var body string
	switch left := l.Text + " — "; {
	case l.Text == l.URL:
		body = s.Muted.Render(xansi.Truncate(l.URL, room, "…"))
	case lipgloss.Width(left) >= room:
		body = xansi.Truncate(left+l.URL, room, "…")
	default:
		body = left + s.Muted.Render(xansi.Truncate(l.URL, room-lipgloss.Width(left), "…"))
	}
	line := marker + body
	if sel {
		line += strings.Repeat(" ", max(0, w-lipgloss.Width(line)))
		return s.RowSelected.Render(line)
	}
	return line
}
