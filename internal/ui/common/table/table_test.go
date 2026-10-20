package table

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

func cols(lang string) []Column {
	t := map[string][]string{
		"en": {"IID", "Title", "Author", "Branch", "Pipeline", "Appr", "Thr", "Upd"},
		"ru": {"IID", "Заголовок", "Автор", "Ветка → target", "Пайплайн", "Апр", "Тр", "Обн"},
	}[lang]
	return []Column{
		{Title: t[0], Min: 5, Priority: 100},
		{Title: t[1], Min: 14, Flex: true, Priority: 90},
		{Title: t[2], Min: 10, Max: 14, Priority: 30},
		{Title: t[3], Min: 16, Max: 28, Priority: 20},
		{Title: t[4], Min: 12, Priority: 80},
		{Title: t[5], Min: 4, Priority: 60, AlignRight: true},
		{Title: t[6], Min: 3, Priority: 10, AlignRight: true},
		{Title: t[7], Min: 4, Priority: 70, AlignRight: true},
	}
}

func rows(n int) []Row {
	out := make([]Row, n)
	for i := range out {
		out[i] = Row{ID: fmt.Sprint(i), Cells: []string{
			fmt.Sprintf("!%d", 400+i), "Fix race in courier slot allocator when many couriers", "anna.k",
			"fix/slot-race → main", "✓ success", "1/2", "2", "4m",
		}}
	}
	return out
}

func newTable(w, h int, lang string, n int) *Model {
	m := New(common.NewStyles(common.TerminalTheme(true)))
	m.SetColumns(cols(lang))
	m.SetSize(w, h)
	m.SetRows(rows(n))
	return m
}

func TestWidthsFitAndLowPriorityColumnsDisappear(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		for _, w := range []int{80, 120, 200} {
			m := newTable(w, 10, lang, 5)
			for i, l := range strings.Split(m.View(), "\n") {
				if got := ansi.StringWidth(l); got > w {
					t.Errorf("%s/%d line %d is %d wide: %q", lang, w, i, got, ansi.Strip(l))
				}
			}
			vis := strings.Join(m.Visible(), ",")
			switch w {
			case 80:
				if strings.Contains(vis, cols(lang)[6].Title) { // Thr has the lowest priority
					t.Errorf("%s/80: lowest priority column must go first: %s", lang, vis)
				}
				if !strings.Contains(vis, cols(lang)[1].Title) || !strings.Contains(vis, "IID") {
					t.Errorf("%s/80: key columns lost: %s", lang, vis)
				}
			case 200:
				if len(m.Visible()) != 8 {
					t.Errorf("%s/200: all columns expected, got %s", lang, vis)
				}
			}
		}
	}
}

func TestDropOrderByPriority(t *testing.T) {
	m := newTable(60, 10, "en", 3)
	vis := m.Visible()
	for _, gone := range []string{"Thr", "Branch", "Author"} {
		for _, v := range vis {
			if v == gone {
				t.Errorf("%s must be dropped before %v", gone, vis)
			}
		}
	}
	if !strings.Contains(strings.Join(vis, ","), "Title") {
		t.Errorf("title must survive: %v", vis)
	}
}

func TestTruncatesWithEllipsis(t *testing.T) {
	m := newTable(80, 6, "en", 2)
	if !strings.Contains(ansi.Strip(m.View()), "…") {
		t.Fatalf("long title must be cut with an ellipsis:\n%s", ansi.Strip(m.View()))
	}
}

func TestCursorFollowsID(t *testing.T) {
	m := newTable(120, 10, "en", 5)
	m.HandleKey(uitest.KeyPress("j"))
	m.HandleKey(uitest.KeyPress("j"))
	if m.CursorID() != "2" {
		t.Fatalf("cursor id = %q", m.CursorID())
	}
	// A new row appears above the cursor.
	next := append([]Row{{ID: "new", Cells: []string{"!500", "new one"}}}, rows(5)...)
	m.SetRows(next)
	if m.CursorID() != "2" || m.Cursor() != 3 {
		t.Fatalf("cursor must stay on id 2: id=%q idx=%d", m.CursorID(), m.Cursor())
	}
	// The row vanishes: the index is clamped.
	m.SetRows(rows(2))
	if m.CursorID() != "1" {
		t.Fatalf("clamped cursor id = %q", m.CursorID())
	}
}

func TestKeysAndNearEnd(t *testing.T) {
	m := newTable(120, 8, "en", 20)
	press := func(k string) bool { return m.HandleKey(uitest.KeyPress(k)) }
	if press("k") {
		t.Fatal("k at the top must not move")
	}
	if !press("G") || m.Cursor() != 19 {
		t.Fatalf("G: %d", m.Cursor())
	}
	if press("j") {
		t.Fatal("j at the bottom must not move")
	}
	press("g")
	if m.Cursor() != 0 || m.NearEnd(5) {
		t.Fatal("top")
	}
	for range 14 {
		press("j")
	}
	if m.Cursor() != 14 || m.NearEnd(5) {
		t.Fatalf("cursor 14 of 20 is not near the end (5): %d", m.Cursor())
	}
	press("j")
	if !m.NearEnd(5) {
		t.Fatalf("cursor 15 of 20 is within 5 of the end")
	}
	press("pgup")
	if m.Cursor() >= 15 {
		t.Fatalf("pgup: %d", m.Cursor())
	}
	if !press("pgdown") {
		t.Fatal("pgdown must move")
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	m := newTable(120, 6, "en", 30) // header + 5 rows
	for range 12 {
		m.HandleKey(uitest.KeyPress("j"))
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "› !412") {
		t.Fatalf("selected row must be visible:\n%s", view)
	}
	if strings.Count(view, "\n")+1 != 6 {
		t.Fatalf("view must be exactly 6 lines:\n%s", view)
	}
}

func TestTailAndEmpty(t *testing.T) {
	m := newTable(120, 10, "en", 2)
	m.SetTail(TailLoading, "◐ loading more…")
	m.HandleKey(uitest.KeyPress("G"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "◐ loading more…") {
		t.Fatalf("tail missing:\n%s", v)
	}
	m.SetTail(TailError, "✗ could not load, r — retry")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "✗ could not load, r — retry") || !strings.Contains(v, "!401") {
		t.Fatalf("error tail must keep loaded rows:\n%s", v)
	}

	e := newTable(120, 10, "en", 0)
	e.SetEmpty("No open MRs\nPress / to change filters")
	if v := ansi.Strip(e.View()); !strings.Contains(v, "No open MRs") || !strings.Contains(v, "Press /") {
		t.Fatalf("empty state:\n%s", v)
	}
	e.SetTail(TailLoading, "◐ loading…")
	if v := ansi.Strip(e.View()); strings.Contains(v, "No open MRs") || !strings.Contains(v, "loading") {
		t.Fatalf("loading replaces the empty state:\n%s", v)
	}
}

func TestSelectedRowIsPlain(t *testing.T) {
	m := newTable(120, 6, "en", 2)
	m.SetRows([]Row{{ID: "a", Cells: []string{"!1", "\x1b[32mgreen\x1b[0m"}}})
	if strings.Contains(m.View(), "\x1b[32m") {
		t.Fatal("inner colors must not leak into the reversed selected row")
	}
}
