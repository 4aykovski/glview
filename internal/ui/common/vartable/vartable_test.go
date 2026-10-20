package vartable

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

func newModel() *Model {
	return New(common.NewStyles(common.TerminalTheme(true)), Labels{Key: "KEY", Value: "VALUE", Type: "type", Empty: "no variables", TypeEnv: "env_var", TypeFile: "file"})
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
