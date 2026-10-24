package workspace

import (
	"testing"

	"github.com/4aykovski/glview/internal/ui/uitest"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the workspace screen:
// every tab, collapsed and expanded, the filter, the project filter, the
// help and the confirm dialogs fit 80×24 and 120×40 in en and ru.
func TestFitsTerminal(t *testing.T) {
	type step struct {
		name string
		keys []string
	}
	collapsed := func(t *testing.T) *uitest.Driver {
		d, _, _ := open(t, newFake(), testScope())
		return d
	}
	expanded := func(t *testing.T) *uitest.Driver {
		d, _, _ := openExpanded(t, newFake(), testScope(), allPaths, allPaths)
		return d
	}
	group := func(t *testing.T) *uitest.Driver {
		client := newFake()
		d, _ := openGroup(t, client, 0)
		return d
	}
	scenarios := []struct {
		name  string
		open  func(*testing.T) *uitest.Driver
		steps []step
	}{
		{"collapsed", collapsed, []step{
			{"projects", nil},
			{"help", []string{"?"}},
			{"projects filter", []string{"esc", "/"}},
			{"mrs", []string{"esc", "2"}},
			{"mrs filter", []string{"/"}},
			{"mrs quick filter", []string{"esc", "m"}},
			{"pipelines", []string{"3"}},
			{"pipelines picker", []string{"P"}},
			{"tags", []string{"esc", "4"}},
			{"tags help", []string{"?"}},
		}},
		{"expanded", expanded, []step{
			{"projects", nil},
			{"mrs", []string{"2"}},
			{"mrs project filter", []string{"P"}},
			{"mrs filter", []string{"esc", "/"}},
			{"mrs comment", []string{"esc", "j", "c"}},
			{"pipelines", []string{"esc", "3"}},
			{"pipelines help", []string{"?"}},
			{"tags", []string{"esc", "4"}},
			{"tags project filter", []string{"P"}},
		}},
		{"group", group, []step{
			{"projects", nil},
			{"mrs", []string{"2"}},
			{"pipelines", []string{"3"}},
			{"tags", []string{"4"}},
		}},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			d := sc.open(t)
			for _, st := range sc.steps {
				d.Key(st.keys...)
				d.RequireFits()
				if t.Failed() {
					t.Fatalf("step %q", st.name)
				}
			}
		})
	}
}
