package common

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// MinWidth and MinHeight are the smallest supported terminal size.
const (
	MinWidth  = 80
	MinHeight = 24
)

// Header renders the top line: app name and context on the left, user and
// refresh state on the right.
func Header(s *Styles, context, right string, width int) string {
	left := s.AppName.Render("gitlab-tui") + "  " + context
	return spread(left, s.HeaderMuted.Render(right), width)
}

// Tab is one tab in the tab bar.
type Tab struct {
	Key   string // "1".."4"
	Label string
	Count int // shown after the label when > 0; -1 hides it
}

// Tabs renders the tab bar with the active tab highlighted.
func Tabs(s *Styles, tabs []Tab, active int) string {
	parts := make([]string, len(tabs))
	for i, t := range tabs {
		label := t.Key + " " + t.Label
		if t.Count > 0 {
			label += fmt.Sprintf(" %d", t.Count)
		}
		if i == active {
			parts[i] = s.TabActive.Render(label)
		} else {
			parts[i] = s.Tab.Render(label)
		}
	}
	return strings.Join(parts, " ")
}

// spread places left and right on one line of width, truncating left if needed.
func spread(left, right string, width int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > width {
		left = truncate(left, max(0, width-rw-1))
		lw = lipgloss.Width(left)
	}
	gap := max(1, width-lw-rw)
	return left + strings.Repeat(" ", gap) + right
}

// FitHeight pads or cuts s to exactly h lines.
func FitHeight(s string, h int) string {
	if h <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
