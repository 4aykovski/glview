package joblog

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the job log: the log, the
// search, the help and an empty log fit 80×24 and 120×40 in en and ru.
func TestFitsTerminal(t *testing.T) {
	d, _, _, _ := setup(t, gitlab.StatusFailed, sample)
	d.RequireFits()
	d.Key("/", "F", "A", "I", "L")
	d.RequireFits()
	d.Key("enter", "esc", "?")
	d.RequireFits()
	d.Key("esc", "z")
	d.RequireFits()

	d, _, _, _ = setup(t, gitlab.StatusSkipped, "")
	d.RequireFits()
	d, _, _, _ = setup(t, gitlab.StatusRunning, sample)
	d.RequireFits()
}
