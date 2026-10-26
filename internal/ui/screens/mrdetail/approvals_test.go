package mrdetail

import (
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func users(names ...string) []gitlab.User {
	out := make([]gitlab.User, len(names))
	for i, n := range names {
		out[i] = gitlab.User{Username: n, Name: strings.ToUpper(n[:1]) + n[1:], WebURL: "https://gitlab.example.com/" + n}
	}
	return out
}

func rulesMR() gitlab.MergeRequest {
	mr := baseMR()
	mr.Reviewers = users("bob", "dima", "lena")
	mr.Approvals = gitlab.Approvals{Required: 2, Left: 1, ApprovedBy: users("bob", "olga"), Rules: []gitlab.ApprovalRule{
		{Name: "Legacy", Type: gitlab.ApprovalRegular, Required: 0, Approved: true, Eligible: users("dima")},
		{Name: "Security", Type: gitlab.ApprovalRegular, Required: 1, Invalid: true},
		{Name: "Security review", Type: gitlab.ApprovalRegular, Required: 1, Approved: true, ApprovedBy: users("olga"), Eligible: users("olga")},
		{Name: "Backend", Type: gitlab.ApprovalRegular, Required: 2, ApprovedBy: users("bob"), Eligible: users("bob", "anna.k", "egor", "dima", "lena", "kate", "olga")},
		{Name: "/docs/", Type: gitlab.ApprovalCodeOwner, Section: "Docs", Required: 1, Eligible: users("lena")},
		{Name: "All Members", Type: gitlab.ApprovalAnyApprover, Required: 1},
	}}
	return mr
}

func TestApproversOverlayInDetail(t *testing.T) {
	d, _, _, _ := setup(t, rulesMR())
	d.Key("A")
	d.RequireView("Approvers · !479")
	d.RequireView("Docs · /docs/")
	d.Key("esc")
	d.RequireNoView("Approvers · !479")
}

func TestApproversNobodyToast(t *testing.T) {
	mr := baseMR()
	mr.Approvals = gitlab.Approvals{}
	d, _, _, _ := setup(t, mr)
	d.Key("A")
	d.RequireView("No approvers in the MR")
	d.RequireNoView("Approvers · !479")
}
