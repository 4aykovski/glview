package common

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Drawing blocks shared by the screens (SPEC §3.0, docs/design). They only
// use styles built from the Theme; NO_COLOR keeps the line characters.

// Rule is a horizontal line of the given width in the Border color.
func Rule(s *Styles, width int) string {
	if width <= 0 {
		return ""
	}
	return s.Rule.Render(strings.Repeat("─", width))
}

// VRule is a vertical line of the given height.
func VRule(s *Styles, height int) string {
	if height <= 0 {
		return ""
	}
	return s.Rule.Render(strings.TrimSuffix(strings.Repeat("│\n", height), "\n"))
}

// ChipRole picks the border color of a chip.
type ChipRole int

const (
	ChipNeutral ChipRole = iota
	ChipSuccess
	ChipWarn
	ChipError
	ChipRunning
)

// Chip renders text in a rounded box (three lines) whose border has the
// color of the role.
func Chip(s *Styles, text string, role ChipRole) string {
	st, ok := s.Chips[role]
	if !ok {
		st = s.Chips[ChipNeutral]
	}
	return st.Render(text)
}

// ChipRow joins chips horizontally with a space between them.
func ChipRow(chips ...string) string {
	parts := make([]string, 0, 2*len(chips))
	for i, c := range chips {
		if i > 0 {
			parts = append(parts, " ")
		}
		parts = append(parts, c)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// BoxKind is the look of a framed box.
type BoxKind int

const (
	BoxNormal   BoxKind = iota // Border color
	BoxActive                  // Accent color
	BoxDashed                  // dashed lines, Border color
	BoxWarning                 // Warning color (unresolved threads)
	BoxSelected                // thick lines, Accent color: the current item among colored boxes
)

// dashedBorder draws ╌ and ╎ between square corners.
var dashedBorder = lipgloss.Border{
	Top: "╌", Bottom: "╌", Left: "╎", Right: "╎",
	TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘",
}

// Box frames content in a box of exactly w columns and h lines (borders
// included). Content is clipped to the inner area.
func Box(s *Styles, content string, w, h int, kind BoxKind) string {
	if w < 2 || h < 2 {
		return ""
	}
	st := s.Boxes[kind]
	inner := lipgloss.NewStyle().Width(w - 2).Height(h - 2).MaxWidth(w - 2).MaxHeight(h - 2)
	return st.Render(inner.Render(content))
}

// FilterBarText renders a filter line from ready-made left text (its "/"
// included) and right-aligned muted text.
func FilterBarText(s *Styles, left, right string, width int) string {
	return spread(left, s.Muted.Render(right), width)
}

// FilterBar renders the filter line: a "/" prefix in Accent, key:value pairs
// (keys Muted) and right-aligned text.
func FilterBar(s *Styles, pairs [][2]string, extra, right string, width int) string {
	parts := []string{s.Accent.Render("/")}
	for _, kv := range pairs {
		parts = append(parts, s.FilterKey.Render(kv[0]+":")+s.FilterValue.Render(kv[1]))
	}
	if extra != "" {
		parts = append(parts, extra)
	}
	left := strings.Join(parts, "  ")
	return spread(left, s.Muted.Render(right), width)
}
