package common

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The list overlays (the picker p, the global search F, the project
// filter P) share one frame: a title line, the input, the list between two
// rules and a footer with the hints and a counter on the right. The
// helpers below draw it and move the selection over its lines.

const (
	listModalMaxW   = 84
	listModalMaxH   = 24
	defaultHalfPage = 5 // before the first View
)

// ListModalWidth is the outer width of a list overlay on a screen of the
// given width.
func ListModalWidth(width int) int { return min(listModalMaxW, width-4) }

// ListModalRows is the number of list lines of a list overlay on a screen
// of the given height.
func ListModalRows(height int) int { return max(3, min(listModalMaxH, height-4)-6) }

// ListModal draws a list overlay of outer width w. rows are the rendered
// list lines (the caller pads them to ListModalRows); counter goes to the
// right of the hints.
func ListModal(s *Styles, w int, head, input string, rows []string, hints, counter string) string {
	inner := w - 4
	foot := s.Muted.Render(hints)
	if counter != "" {
		gap := max(1, inner-lipgloss.Width(foot)-lipgloss.Width(counter))
		foot += strings.Repeat(" ", gap) + s.Muted.Render(counter)
	}
	body := head + "\n" + input + "\n" + Rule(s, inner) + "\n" + strings.Join(rows, "\n") + "\n" + Rule(s, inner) + "\n" + foot
	return s.Modal.Width(w).Render(body)
}

// ListStart is the first line shown in a list of h lines so that line sel
// is visible.
func ListStart(sel, h int) int {
	if sel >= h {
		return sel - h + 1
	}
	return 0
}

// ListMove returns the nearest line after sel in direction d (1 or -1) for
// which ok holds, or sel if there is none.
func ListMove(n, sel, d int, ok func(int) bool) int {
	for i := sel + d; i >= 0 && i < n; i += d {
		if ok(i) {
			return i
		}
	}
	return sel
}

// ListPage moves sel half of h visible lines down (d > 0) or up; every
// line counts. It lands on the nearest line for which ok holds in the
// direction of the move, or before it at the end of the list. h = 0 (no
// View yet) moves five lines.
func ListPage(n, sel, d, h int, ok func(int) bool) int {
	if n == 0 {
		return sel
	}
	step := max(1, h/2)
	if h == 0 {
		step = defaultHalfPage
	}
	target := min(max(sel+d*step, 0), n-1)
	for _, dir := range []int{d, -d} {
		for i := target; i >= 0 && i < n; i += dir {
			if ok(i) {
				return i
			}
		}
	}
	return sel
}

// Highlight renders text with the characters at the matched byte offsets
// (a fuzzy match) in Accent and bold.
func Highlight(s *Styles, text string, matched []int) string {
	if len(matched) == 0 {
		return text
	}
	hit := make(map[int]bool, len(matched))
	for _, i := range matched {
		hit[i] = true
	}
	style := s.Accent.Bold(true)
	var b strings.Builder
	for i, r := range text {
		if hit[i] {
			b.WriteString(style.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
