package common

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func plainStyles() *Styles { return NewStyles(TerminalTheme(true)) }

func TestRuleAndVRule(t *testing.T) {
	s := plainStyles()
	if got := ansi.Strip(Rule(s, 12)); got != strings.Repeat("─", 12) {
		t.Fatalf("rule = %q", got)
	}
	if Rule(s, 0) != "" || VRule(s, 0) != "" {
		t.Fatal("zero size is empty")
	}
	v := ansi.Strip(VRule(s, 3))
	if v != "│\n│\n│" {
		t.Fatalf("vrule = %q", v)
	}
}

func TestChip(t *testing.T) {
	s := plainStyles()
	out := ansi.Strip(Chip(s, "● mergeable", ChipSuccess))
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("chip must be 3 lines:\n%s", out)
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") || !strings.Contains(lines[1], "● mergeable") || !strings.HasPrefix(lines[2], "╰") {
		t.Fatalf("chip:\n%s", out)
	}
	if lipgloss.Width(lines[0]) != lipgloss.Width(lines[1]) || lipgloss.Width(lines[1]) != lipgloss.Width(lines[2]) {
		t.Fatal("chip lines must have equal widths")
	}
	row := ansi.Strip(ChipRow(Chip(s, "a", ChipNeutral), Chip(s, "b", ChipError)))
	if len(strings.Split(row, "\n")) != 3 || !strings.Contains(row, "│ a │ │ b │") {
		t.Fatalf("row:\n%s", row)
	}
}

func TestBoxSizesAndBorders(t *testing.T) {
	s := plainStyles()
	for _, kind := range []BoxKind{BoxNormal, BoxActive, BoxDashed} {
		out := ansi.Strip(Box(s, "hello\nworld\nextra line that is far too long for the box", 20, 5, kind))
		lines := strings.Split(out, "\n")
		if len(lines) != 5 {
			t.Fatalf("kind %d: height %d\n%s", kind, len(lines), out)
		}
		for _, l := range lines {
			if lipgloss.Width(l) != 20 {
				t.Fatalf("kind %d: width %d: %q", kind, lipgloss.Width(l), l)
			}
		}
		if !strings.Contains(lines[1], "hello") {
			t.Fatalf("content missing:\n%s", out)
		}
	}
	dashed := ansi.Strip(Box(s, "x", 6, 3, BoxDashed))
	if !strings.Contains(dashed, "╌") || !strings.Contains(dashed, "╎") {
		t.Fatalf("dashed box:\n%s", dashed)
	}
	normal := ansi.Strip(Box(s, "x", 6, 3, BoxNormal))
	if !strings.Contains(normal, "─") || !strings.Contains(normal, "│") {
		t.Fatalf("normal box:\n%s", normal)
	}
	if Box(s, "x", 1, 1, BoxNormal) != "" {
		t.Fatal("too small")
	}
}

func TestBoxActiveUsesAccent(t *testing.T) {
	s := plainStyles()
	if Box(s, "x", 6, 3, BoxActive) == Box(s, "x", 6, 3, BoxNormal) {
		t.Fatal("active box must differ from the normal one by color")
	}
}

func TestBoxWarningUsesWarningColor(t *testing.T) {
	s := NewStyles(TerminalTheme(false)) // light theme: Warning and Accent differ
	w := Box(s, "x", 6, 3, BoxWarning)
	if w == Box(s, "x", 6, 3, BoxNormal) || w == Box(s, "x", 6, 3, BoxActive) {
		t.Fatal("warning box must differ from normal and active by color")
	}
	if ansi.Strip(w) != ansi.Strip(Box(s, "x", 6, 3, BoxNormal)) {
		t.Fatal("warning box must keep the normal frame characters")
	}
}

func TestBoxSelectedIsThick(t *testing.T) {
	s := plainStyles()
	out := ansi.Strip(Box(s, "x", 6, 3, BoxSelected))
	if !strings.HasPrefix(out, "┏") || !strings.Contains(out, "┛") {
		t.Fatalf("selected box must use the thick border:\n%s", out)
	}
}

func TestFilterBar(t *testing.T) {
	s := plainStyles()
	out := ansi.Strip(FilterBar(s, [][2]string{{"ref", "any"}, {"status", "any"}}, "", "20 of 1 482", 60))
	if !strings.HasPrefix(out, "/  ref:any  status:any") || !strings.HasSuffix(out, "20 of 1 482") || lipgloss.Width(out) != 60 {
		t.Fatalf("bar = %q (%d)", out, lipgloss.Width(out))
	}
}

func TestBlocksWithNilColorsDoNotPanic(t *testing.T) {
	s := NewStyles(Theme{})
	_ = Rule(s, 5)
	_ = Chip(s, "x", ChipWarn)
	_ = Box(s, "x", 5, 3, BoxActive)
	// Without color the characters still draw the frame (NO_COLOR).
	if got := Box(s, "x", 5, 3, BoxNormal); !strings.Contains(got, "─") {
		t.Fatalf("frame lost: %q", got)
	}
}
