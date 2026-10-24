package project

import (
	"testing"

	"github.com/4aykovski/glview/internal/ui/uitest"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the project screens and
// everything opened from them: at 80×24 and 120×40, in en and ru, no line is
// wider than the terminal and no view is taller (uitest.Driver.RequireFits).
func TestFitsTerminal(t *testing.T) {
	type step struct {
		name string
		keys []string
	}
	openMRs := func(t *testing.T) *uitest.Driver {
		d, _, _ := open(t, goldenFake(), goldenRepo(t))
		return d
	}
	openTagsTab := func(t *testing.T) *uitest.Driver {
		d, _, _ := openTags(t, tagsFake(), nil)
		return d
	}
	openIssuesTab := func(t *testing.T) *uitest.Driver { return openIssues(t, issuesFake(), nil).d }
	scenarios := []struct {
		name  string
		open  func(*testing.T) *uitest.Driver
		steps []step
	}{
		{"mrs", openMRs, []step{
			{"list", nil},
			{"help", []string{"?"}},
			{"filter", []string{"esc", "/"}},
			{"filter value suggestions", []string{"s", "t", "a", "t", "e", ":"}},
			{"filter error", []string{"x", "enter"}},
			{"detail", []string{"esc", "enter"}},
			{"detail help", []string{"?"}},
			{"detail discussions", []string{"esc", "tab"}},
			{"detail pipelines", []string{"tab"}},
			{"detail commits", []string{"tab"}},
			{"merge dialog", []string{"M"}},
			{"comment", []string{"esc", "c"}},
		}},
		{"pipelines", openMRs, []step{
			{"list", []string{"2"}},
			{"filter", []string{"/"}},
			{"confirm retry", []string{"esc", "R"}},
			{"confirm cancel", []string{"esc", "c"}},
			{"new pipeline form", []string{"esc", "n"}},
			{"pipeline screen", []string{"esc", "enter"}},
			{"pipeline help", []string{"?"}},
		}},
		{"issues", openIssuesTab, []step{
			{"list", nil},
			{"help", []string{"?"}},
			{"filter", []string{"esc", "/"}},
			{"confirm close", []string{"esc", "x"}},
			{"comment", []string{"esc", "c"}},
			{"issue screen", []string{"esc", "enter"}},
			{"issue help", []string{"?"}},
		}},
		{"tags", openTagsTab, []step{
			{"list", nil},
			{"help", []string{"?"}},
			{"search", []string{"esc", "/"}},
			{"confirm delete", []string{"esc", "D"}},
			{"new tag form", []string{"esc", "n"}},
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
