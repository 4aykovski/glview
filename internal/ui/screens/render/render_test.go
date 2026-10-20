package render

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
)

func TestMRStatusChip(t *testing.T) {
	cases := []struct {
		state  gitlab.MRState
		status string
		text   string
		role   MergeRole
	}{
		{gitlab.MRStateOpened, "mergeable", "● mergeable", RoleOK},
		{gitlab.MRStateOpened, "not_approved", "✗ not_approved", RoleBad},
		{gitlab.MRStateOpened, "", "○ —", RoleNeutral},
		{gitlab.MRStateMerged, "not_open", "✓ merged", RoleOK},
		{gitlab.MRStateClosed, "not_open", "○ closed", RoleNeutral},
		{gitlab.MRStateLocked, "not_open", "○ locked", RoleNeutral},
	}
	for _, c := range cases {
		text, role := MRStatusChip(&gitlab.MergeRequest{State: c.state, MergeStatus: c.status})
		if text != c.text || role != c.role {
			t.Errorf("%s/%s: got %q/%v, want %q/%v", c.state, c.status, text, role, c.text, c.role)
		}
	}
}

func TestApprovals(t *testing.T) {
	s := common.NewStyles(common.TerminalTheme(true))
	users := func(n int) []gitlab.User { return make([]gitlab.User, n) }
	cases := []struct {
		have, req int
		want      string
	}{
		{0, 0, "0"}, {2, 0, "2"}, {1, 2, "1/2"}, {2, 2, "2 ✓"}, {3, 1, "3 ✓"}, {0, 2, "0/2"},
	}
	for _, c := range cases {
		got := ansi.Strip(Approvals(s, gitlab.Approvals{Required: c.req, ApprovedBy: users(c.have)}))
		if got != c.want {
			t.Errorf("%d/%d: got %q, want %q", c.have, c.req, got, c.want)
		}
	}
}

func TestPipelineRef(t *testing.T) {
	for in, want := range map[string]string{
		"refs/merge-requests/482/merge": "!482",
		"refs/merge-requests/482/head":  "!482",
		"refs/merge-requests/482/train": "!482",
		"main":                          "main",
		"feat/surge":                    "feat/surge",
		"refs/merge-requests/x/merge":   "refs/merge-requests/x/merge",
		"refs/merge-requests/482/other": "refs/merge-requests/482/other",
		"refs/merge-requests/482":       "refs/merge-requests/482",
		"refs/tags/v1":                  "refs/tags/v1",
	} {
		if got := PipelineRef(in); got != want {
			t.Errorf("PipelineRef(%q) = %q, want %q", in, got, want)
		}
	}
}
