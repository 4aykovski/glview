package grouped

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/table"
)

func key(k string) tea.KeyPressMsg {
	switch k {
	case "space":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "})
	}
	r := []rune(k)[0]
	kk := tea.Key{Code: r, Text: k}
	if r >= 'A' && r <= 'Z' {
		kk.Code, kk.ShiftedCode, kk.Mod = r+('a'-'A'), r, tea.ModShift
	}
	return tea.KeyPressMsg(kk)
}

func cols() []table.Column {
	return []table.Column{
		{Title: "IID", Min: 5, Priority: 100},
		{Title: "Title", Min: 20, Flex: true, Priority: 90},
		{Title: "Author", Min: 8, Max: 16, Priority: 40},
		{Title: "You", Min: 8, Priority: 30},
		{Title: "Pipeline", Min: 10, Priority: 80},
		{Title: "Appr", Min: 4, Priority: 70, AlignRight: true},
		{Title: "Upd", Min: 4, Priority: 60, AlignRight: true},
	}
}

func rows(prefix string, n int) []table.Row {
	var out []table.Row
	for i := 1; i <= n; i++ {
		out = append(out, table.Row{ID: fmt.Sprint(i), Cells: []string{
			fmt.Sprintf("!%d", i), prefix + " merge request title", "anna.k", "reviewer", "✓ success", "1/2", "4m"}})
	}
	return out
}

func groups() []Group {
	return []Group{
		{Key: "platform/route-service", Title: "platform/route-service", Summary: "6 open · showing 3", Rows: rows("route", 3), More: 3},
		{Key: "frontend/route-planner-ui", Title: "frontend/route-planner-ui", Summary: "2 open", Rows: rows("ui", 2)},
		{Key: "qa/e2e-routes", Title: "qa/e2e-routes", State: Empty, Note: "no open"},
		{Key: "platform/gone", Title: "platform/gone", State: Failed, Note: "no access"},
		{Key: "infra/x", Title: "infra/x", State: Loading, Note: "loading…"},
	}
}

func newModel(w int) *Model {
	m := New(common.NewStyles(common.TerminalTheme(true)))
	m.SetColumns(cols())
	m.SetMoreText(func(n int) string { return fmt.Sprintf("↳ %d more · f filter by project", n) })
	m.SetSize(w, 20)
	m.SetGroups(groups())
	return m
}

func TestViewWidths(t *testing.T) {
	for _, w := range []int{80, 120} {
		m := newModel(w)
		v := ansi.Strip(m.View())
		for _, want := range []string{"▾ platform/route-service  6 open · showing 3", "↳ 3 more · f filter by project",
			"▸ qa/e2e-routes  no open", "✗ platform/gone  no access", "◐ infra/x  loading…", "!1"} {
			if !strings.Contains(v, want) {
				t.Fatalf("w=%d: no %q:\n%s", w, want, v)
			}
		}
		for _, l := range strings.Split(v, "\n") {
			if ansi.StringWidth(l) > w {
				t.Fatalf("w=%d: line too wide: %q", w, l)
			}
		}
		vis := strings.Join(m.Visible(), ",")
		if w == 120 && vis != "IID,Title,Author,You,Pipeline,Appr,Upd" {
			t.Fatalf("120: %s", vis)
		}
	}
}

func TestCursorAndCollapsedSurviveSetGroups(t *testing.T) {
	m := newModel(120)
	if _, _, k := m.Cursor(); k != KindHeader {
		t.Fatal("starts on the first header")
	}
	m.HandleKey(key("j"))
	m.HandleKey(key("j"))
	g, r, k := m.Cursor()
	if k != KindRow || g.Key != "platform/route-service" || r.ID != "2" {
		t.Fatalf("cursor = %v %v %v", g.Key, r, k)
	}
	m.HandleKey(key("J")) // next header
	g, _, k = m.Cursor()
	if k != KindHeader || g.Key != "frontend/route-planner-ui" {
		t.Fatalf("J → %s %v", g.Key, k)
	}
	m.HandleKey(key("space"))
	if !m.Collapsed("frontend/route-planner-ui") {
		t.Fatal("space collapses")
	}
	m.HandleKey(key("K"))
	m.HandleKey(key("j"))
	m.HandleKey(key("j")) // route-service !2
	// New data in another order, route-service lost !1: cursor stays on !2.
	gs := groups()
	gs[0], gs[1] = gs[1], gs[0]
	gs[1].Rows = gs[1].Rows[1:]
	m.SetGroups(gs)
	g, r, _ = m.Cursor()
	if g.Key != "platform/route-service" || r == nil || r.ID != "2" {
		t.Fatalf("cursor after update = %v %v", g.Key, r)
	}
	if !m.Collapsed("frontend/route-planner-ui") || strings.Contains(ansi.Strip(m.View()), "ui merge request") {
		t.Fatal("collapsed group stays collapsed")
	}
	// The record disappears: the cursor goes to its group's header.
	gs[1].Rows = gs[1].Rows[1:]
	m.SetGroups(gs)
	g, _, k = m.Cursor()
	if g.Key != "platform/route-service" || k != KindHeader {
		t.Fatalf("cursor = %v %v", g.Key, k)
	}
	// Space on an empty group does nothing.
	m.HandleKey(key("G"))
	m.HandleKey(key("space"))
	if m.Collapsed("infra/x") {
		t.Fatal("a loading group cannot be collapsed")
	}
}
