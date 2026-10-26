package mrdetail

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestGoldenOverview(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.RequireGoldenSizes("mrdetail-overview")
}

func TestGoldenDiscussions(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Key("tab", "tab")
	d.RequireGoldenSizes("mrdetail-discussions")
}

func TestGoldenMergeDialog(t *testing.T) {
	mr := baseMR()
	mr.MergeStatus = "mergeable"
	mr.Squash = true
	d, _, _, _ := setup(t, mr)
	d.Key("M")
	d.RequireGoldenSizes("mrdetail-merge")
}

func TestGoldenMerged(t *testing.T) {
	d, _, _, _ := setup(t, mergedMR())
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.RequireGolden("mrdetail-merged-120x40")
}

func TestGoldenDiscussionsRich(t *testing.T) {
	d, _, _, client := setup(t, baseMR())
	client.SetDiscussions(path, 479, richDiscussions(t))
	d.Key("r", "tab", "tab")
	d.RequireGoldenSizes("mrdetail-discussions-rich")
}

func TestGoldenCommits(t *testing.T) {
	mr := baseMR()
	mr.SHA = testCommits()[0].SHA
	d, _, _, client := setup(t, mr)
	client.SetMRCommits(path, 479, testCommits())
	d.Key("shift+tab")
	d.RequireGoldenSizes("mrdetail-commits")
}

// rulesMRNotMe is rulesMR where the fake user (egor) is not a candidate.
func rulesMRNotMe() gitlab.MergeRequest {
	mr := rulesMR()
	for i := range mr.Approvals.Rules {
		for j, u := range mr.Approvals.Rules[i].Eligible {
			if u.Username == "egor" {
				mr.Approvals.Rules[i].Eligible[j].Username = "oleg"
			}
		}
	}
	return mr
}

func reviewersMR(me string) gitlab.MergeRequest {
	mr := baseMR()
	mr.Reviewers = users("bob", "dima", me)
	mr.ReviewStates = map[string]gitlab.ReviewState{"bob": gitlab.ReviewApproved, "dima": gitlab.ReviewRequestedChanges, me: gitlab.ReviewUnreviewed}
	mr.Approvals = gitlab.Approvals{Required: 2, Left: 1, ApprovedBy: users("bob", "olga")}
	return mr
}

func TestGoldenApprovalRules(t *testing.T) {
	d, _, _, _ := setup(t, rulesMRNotMe())
	d.RequireGoldenSizes("mrdetail-approvals-rules")
	d.Key("ctrl+l")
	d.RequireGoldenSizes("mrdetail-approvals-rules-ru")
}

func TestGoldenApprovalRulesMine(t *testing.T) {
	d, _, _, _ := setup(t, rulesMR())
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.RequireGolden("mrdetail-approvals-rules-mine-120x40")
}

func TestGoldenApprovalReviewers(t *testing.T) {
	d, _, _, _ := setup(t, reviewersMR("lena"))
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.RequireGolden("mrdetail-approvals-reviewers-120x40")
}

func TestGoldenApprovalReviewersMine(t *testing.T) {
	d, _, _, _ := setup(t, reviewersMR("egor"))
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.RequireGolden("mrdetail-approvals-reviewers-mine-120x40")
}
