package vartable

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

func newModel() *Model {
	return New(common.NewStyles(common.TerminalTheme(true)), Labels{Key: "KEY", Value: "VALUE", Type: "type", Empty: "no variables", TypeEnv: "env_var", TypeFile: "file", KeyPlaceholder: "KEY", ValuePlaceholder: "value"})
}

func keys(m *Model, ks ...string) {
	for _, k := range ks {
		m.Update(uitest.KeyPress(k))
	}
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(uitest.KeyPress(string(r)))
	}
}

func TestAddEditFlow(t *testing.T) {
	m := newModel()
	if m.CapturesInput() {
		t.Fatal("not editing initially")
	}
	keys(m, "a")
	if !m.CapturesInput() {
		t.Fatal("a must start editing the key")
	}
	typeText(m, "TARGET") // letters are text here, not shortcuts
	keys(m, "enter")      // commits the key and moves to the value
	if !m.CapturesInput() {
		t.Fatal("the value must be edited next")
	}
	typeText(m, "canary")
	keys(m, "enter")
	if m.CapturesInput() {
		t.Fatal("done editing")
	}
	vars := m.Vars()
	if len(vars) != 1 || vars[0].Key != "TARGET" || vars[0].Value != "canary" || vars[0].Type != gitlab.VarEnv {
		t.Fatalf("vars = %+v", vars)
	}
	v := ansi.Strip(m.View(80))
	if !strings.Contains(v, "TARGET") || !strings.Contains(v, "canary") || !strings.Contains(v, "env_var") {
		t.Fatalf("view:\n%s", v)
	}
}

func TestTypeToggleDeleteAndCancel(t *testing.T) {
	m := newModel()
	m.Add("CFG", "x", gitlab.VarEnv)
	keys(m, "space")
	if m.Vars()[0].Type != gitlab.VarFile {
		t.Fatal("space must switch the type")
	}
	keys(m, "a", "esc") // cancelled fresh row disappears
	if m.Len() != 1 {
		t.Fatalf("rows = %d", m.Len())
	}
	keys(m, "d")
	if m.Len() != 0 {
		t.Fatal("d must delete")
	}
	if !strings.Contains(ansi.Strip(m.View(80)), "no variables") {
		t.Fatal("empty hint")
	}
}

func TestEditExistingCellAndEmptyKeysDropped(t *testing.T) {
	m := newModel()
	m.Add("A", "1", gitlab.VarEnv)
	keys(m, "l", "enter") // value column
	typeText(m, "2")
	keys(m, "enter")
	if m.Vars()[0].Value != "12" {
		t.Fatalf("value = %q", m.Vars()[0].Value)
	}
	m.Add("", "orphan", gitlab.VarEnv)
	if len(m.Vars()) != 1 {
		t.Fatal("rows without a key are not variables")
	}
	if !m.Has("A") || m.Has("B") {
		t.Fatal("Has")
	}
	m.Set("A", "9")
	m.Set("B", "8")
	if m.Vars()[0].Value != "9" || len(m.Vars()) != 2 {
		t.Fatalf("Set: %+v", m.Vars())
	}
}

// selected returns the text inside the selection style of the view (the
// terminal theme renders the selection in reverse video).
func selected(t *testing.T, view string) string {
	t.Helper()
	for _, l := range strings.Split(view, "\n") {
		if i := strings.Index(l, "\x1b[7m"); i >= 0 {
			rest := l[i+len("\x1b[7m"):]
			if j := strings.Index(rest, "\x1b[m"); j >= 0 {
				return rest[:j]
			}
		}
	}
	t.Fatalf("no selected cell in view:\n%q", view)
	return ""
}

// caretVisible reports whether the view has the text input caret: a
// reverse-video cell in the cursor color that is not clipped away (a
// clipped caret leaves its escape codes without the cell).
func caretVisible(view string) bool { return caretRe.MatchString(view) }

var caretRe = regexp.MustCompile("\x1b\\[7;33m[^\x1b]")

func TestSelectedEmptyCellIsVisible(t *testing.T) {
	m := newModel()
	m.Add("", "", gitlab.VarEnv)
	m.width = 80
	// Key column: the placeholder is drawn inside the selection, across the column.
	keyW, valW, typeW := colWidths(80)
	if got := selected(t, m.View(80)); strings.TrimSpace(got) != "KEY" || len(got) != keyW {
		t.Fatalf("selected key cell = %q (want KEY padded to %d)", got, keyW)
	}
	keys(m, "l")
	if got := selected(t, m.View(80)); strings.TrimSpace(got) != "value" || len(got) != valW {
		t.Fatalf("selected value cell = %q", got)
	}
	keys(m, "l")
	if got := selected(t, m.View(80)); strings.TrimSpace(got) != "env_var" || len(got) != typeW {
		t.Fatalf("selected type cell = %q", got)
	}
}

func TestEmptyCellsShowMutedPlaceholders(t *testing.T) {
	m := newModel()
	m.Add("A", "", gitlab.VarEnv)
	m.Add("", "", gitlab.VarEnv) // cursor moves to the second row
	v := m.View(80)
	// The first row's empty value is a muted placeholder, not selected.
	if !strings.Contains(v, "\x1b[90mvalue\x1b[m") {
		t.Fatalf("muted placeholder missing:\n%q", v)
	}
	plain := ansi.Strip(v)
	if strings.Count(plain, "value") != 2 || !strings.Contains(plain, "KEY") {
		t.Fatalf("placeholders:\n%s", plain)
	}
	if len(m.Vars()) != 1 {
		t.Fatal("placeholders are not values")
	}
}

func TestCaretVisibleRightAfterAddAndEnter(t *testing.T) {
	m := newModel()
	m.View(80)
	keys(m, "a")
	if !caretVisible(m.View(80)) {
		t.Fatalf("caret must be visible right after a:\n%q", m.View(80))
	}
	typeText(m, "K")
	keys(m, "enter") // moves on to the value
	if !caretVisible(m.View(80)) {
		t.Fatal("caret must be visible in the value cell")
	}
	keys(m, "enter")
	keys(m, "h", "enter") // edit the existing key again
	if !caretVisible(m.View(80)) {
		t.Fatal("caret must be visible right after enter")
	}
}

func TestCaretVisibleInLongValue(t *testing.T) {
	m := newModel()
	m.Add("A", strings.Repeat("x", 200), gitlab.VarEnv)
	m.View(80)
	keys(m, "l", "enter")
	v := m.View(80)
	if !caretVisible(v) {
		t.Fatalf("the input must scroll to keep the caret visible:\n%q", v)
	}
	if w := ansi.StringWidth(strings.Split(v, "\n")[1]); w > 80 {
		t.Fatalf("row is %d wide", w)
	}
}
