package common

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/i18n"
)

func footerBindings(m *i18n.Messages) []Binding {
	k := m.Keys
	return []Binding{
		NewBinding([]string{"j"}, "j/k", k.Down, PriorityHigh),
		NewBinding([]string{"enter"}, "", k.Open, PriorityHigh),
		NewBinding([]string{"a"}, "", "approve", PriorityNormal),
		NewBinding([]string{"M"}, "", "merge", PriorityNormal),
		NewBinding([]string{"b"}, "", "rebase", PriorityNormal),
		NewBinding([]string{"C"}, "", "checkout", PriorityNormal),
		NewBinding([]string{"o"}, "", k.Browser, PriorityLow),
		NewBinding([]string{"p"}, "", k.Project, PriorityLow),
		NewBinding([]string{"F"}, "", k.Search, PriorityLow),
		NewBinding([]string{"?"}, "", k.Help, PriorityHelp),
	}
}

func TestFooterFitsAndKeepsHelp(t *testing.T) {
	s := NewStyles(TerminalTheme(true))
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		m := i18n.Get(lang)
		for _, w := range []int{80, 120} {
			for _, right := range []string{"", "✓ approved !479"} {
				out := RenderFooter(s, footerBindings(m), right, w)
				plain := ansi.Strip(out)
				if lipgloss.Width(out) > w {
					t.Errorf("%s w=%d right=%q: width %d > %d: %q", lang, w, right, lipgloss.Width(out), w, plain)
				}
				if !strings.Contains(plain, "? "+m.Keys.Help) {
					t.Errorf("%s w=%d: help missing: %q", lang, w, plain)
				}
				if !strings.HasPrefix(plain, "j/k") {
					t.Errorf("%s w=%d: highest priority must stay first: %q", lang, w, plain)
				}
				if right != "" && !strings.HasSuffix(plain, right) {
					t.Errorf("%s w=%d: right block missing: %q", lang, w, plain)
				}
			}
		}
	}
}

func TestFooterDropsLowPriorityFirst(t *testing.T) {
	s := NewStyles(TerminalTheme(true))
	m := i18n.Get(i18n.EN)
	plain := ansi.Strip(RenderFooter(s, footerBindings(m), "", 60))
	if strings.Contains(plain, "search GitLab") {
		t.Errorf("low priority kept at 60 cols: %q", plain)
	}
	if !strings.Contains(plain, "open") {
		t.Errorf("high priority dropped: %q", plain)
	}
	if !strings.HasSuffix(plain, "? help") {
		t.Errorf("help must be last: %q", plain)
	}
}

func TestFooterHiddenBindings(t *testing.T) {
	s := NewStyles(TerminalTheme(true))
	b := NewBinding([]string{"C"}, "", "checkout", PriorityHigh)
	b.Hidden = true
	plain := ansi.Strip(RenderFooter(s, []Binding{b}, "", 120))
	if strings.Contains(plain, "checkout") {
		t.Fatalf("hidden binding shown: %q", plain)
	}
}

func TestGen(t *testing.T) {
	a, b := NewGen(), NewGen()
	ta := a.Next()
	tb := b.Next()
	if ta == tb {
		t.Fatal("tokens of different screens must differ")
	}
	if !a.Valid(ta) || a.Valid(tb) {
		t.Fatal("validity")
	}
	a.Next()
	if a.Valid(ta) || !a.Owns(ta) {
		t.Fatal("old token must be invalid but owned")
	}
}

func TestFitHeight(t *testing.T) {
	if got := FitHeight("a\nb\nc", 2); got != "a\nb" {
		t.Fatalf("cut: %q", got)
	}
	if got := FitHeight("a", 3); got != "a\n\n" {
		t.Fatalf("pad: %q", got)
	}
}
