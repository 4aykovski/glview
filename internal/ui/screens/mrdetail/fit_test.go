package mrdetail

import (
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

// TestFitsTerminal is the SPEC §12.3 invariant for MR detail: every tab, the
// merge dialog, the links, the comment form and the help fit 80×24 and
// 120×40 in en and ru, also with a long title and a conflict.
func TestFitsTerminal(t *testing.T) {
	long := baseMR()
	long.Title = strings.Repeat("Fix the race in the courier slot allocator ", 4)
	long.SourceBranch = "feature/" + strings.Repeat("very-long-branch-name-", 4)
	long.MergeStatus, long.HasConflicts = "conflict", true
	long.Labels = []string{"bug", "backend", "needs-review", "customer-impact", "regression"}
	for name, mr := range map[string]gitlab.MergeRequest{"base": baseMR(), "long": long} {
		t.Run(name, func(t *testing.T) {
			d, _, _, _ := setup(t, mr)
			d.RequireFits()
			for _, keys := range [][]string{{"tab"}, {"j"}, {"tab"}, {"tab"}, {"tab"}, {"M"}, {"esc", "L"}, {"esc", "c"}, {"esc", "?"}, {"esc", "b"}} {
				d.Key(keys...)
				d.RequireFits()
			}
		})
	}
}
