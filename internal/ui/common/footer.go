package common

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const footerGap = "  "

// RenderFooter lays out key hints on one line of the given width: bindings
// are added by descending priority while they fit, then shown in their
// declared order; the help binding (PriorityHelp) is always kept and placed
// last. right is shown right-aligned (toast or status) and has precedence
// over low-priority hints.
func RenderFooter(s *Styles, bindings []Binding, right string, width int) string {
	type item struct {
		idx  int
		b    Binding
		text string
		w    int
	}
	var items []item
	for i, b := range bindings {
		if !b.Visible() {
			continue
		}
		h := b.Help()
		text := s.Key.Render(h.Key) + " " + s.KeyDesc.Render(h.Desc)
		items = append(items, item{idx: i, b: b, text: text, w: lipgloss.Width(text)})
	}

	rightW := lipgloss.Width(right)
	avail := width
	if rightW > 0 {
		avail -= rightW + len(footerGap)
	}

	byPrio := append([]item(nil), items...)
	sort.SliceStable(byPrio, func(i, j int) bool { return byPrio[i].b.Priority > byPrio[j].b.Priority })

	chosen := map[int]bool{}
	used := 0
	for _, it := range byPrio {
		need := it.w
		if len(chosen) > 0 {
			need += len(footerGap)
		}
		if used+need > avail && it.b.Priority < PriorityHelp {
			continue
		}
		chosen[it.idx] = true
		used += need
	}

	var left, help []string
	for _, it := range items {
		if !chosen[it.idx] {
			continue
		}
		if it.b.Priority >= PriorityHelp {
			help = append(help, it.text)
		} else {
			left = append(left, it.text)
		}
	}
	line := strings.Join(append(left, help...), footerGap)

	if rightW > 0 {
		pad := width - lipgloss.Width(line) - rightW
		if pad < len(footerGap) {
			// Not even the right block fits next to the hints: drop the hints.
			if rightW > width {
				return truncate(right, width)
			}
			pad = width - rightW
			line = ""
		}
		line += strings.Repeat(" ", pad) + right
	}
	return s.Footer.Render(line)
}

// truncate cuts a styled string to w cells, adding an ellipsis.
func truncate(s string, w int) string { return ansi.Truncate(s, w, "…") }
