package common

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
)

func testLinks() []gitlab.Link {
	return []gitlab.Link{
		{Text: "ABC-123", URL: "https://tracker.example.com/browse/ABC-123"},
		{Text: "#12", URL: "https://gitlab.example.com/platform/route-service/-/issues/12"},
		{Text: "https://docs.example.com/x", URL: "https://docs.example.com/x"},
	}
}

func TestLinksModalMove(t *testing.T) {
	m := NewLinksModal(testLinks())
	m.Move("j")
	if l, _ := m.Selected(); l.Text != "#12" {
		t.Fatalf("after j: %+v", l)
	}
	m.Move("G")
	if l, _ := m.Selected(); l.Text != "https://docs.example.com/x" {
		t.Fatalf("after G: %+v", l)
	}
	m.Move("j") // stays at the end
	m.Move("k")
	m.Move("g")
	if l, _ := m.Selected(); l.Text != "ABC-123" {
		t.Fatalf("after g: %+v", l)
	}
	if m.Move("x") {
		t.Fatal("x is not a move")
	}
}

func TestLinksModalView(t *testing.T) {
	s := NewStyles(TerminalTheme(true))
	m := NewLinksModal(testLinks())
	out := xansi.Strip(m.View(s, "Links · !479", "j/k move", 120, 40))
	for _, want := range []string{"Links · !479", "ABC-123 — https://tracker.example.com/browse/ABC-123", "1/3", "j/k move"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
	// text == url: one column.
	if strings.Contains(out, "https://docs.example.com/x — ") {
		t.Errorf("text equal to url must not repeat:\n%s", out)
	}
	narrow := xansi.Strip(m.View(s, "Links", "", 50, 24))
	if !strings.Contains(narrow, "…") {
		t.Errorf("long rows must be cut with …:\n%s", narrow)
	}
}

func TestLinksModalSetLinksKeepsSelection(t *testing.T) {
	m := NewLinksModal(testLinks())
	m.Move("j")
	m.SetLinks([]gitlab.Link{{Text: "new", URL: "https://x.example.com/new"}, testLinks()[1]})
	if l, _ := m.Selected(); l.Text != "#12" {
		t.Fatalf("selection moved: %+v", l)
	}
	m.SetLinks(nil)
	if _, ok := m.Selected(); ok || m.Len() != 0 {
		t.Fatal("empty list has no selection")
	}
}
