package mrdetail

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
)

func users(names ...string) []gitlab.User {
	out := make([]gitlab.User, len(names))
	for i, n := range names {
		out[i] = gitlab.User{Username: n}
	}
	return out
}

func rulesMR() gitlab.MergeRequest {
	mr := baseMR()
	mr.Reviewers = users("bob", "dima", "lena")
	mr.Approvals = gitlab.Approvals{Required: 2, Left: 1, ApprovedBy: users("bob"), Rules: []gitlab.ApprovalRule{
		{Name: "Legacy", Type: gitlab.ApprovalRegular, Required: 0, Approved: true, Eligible: users("dima")},
		{Name: "Security", Type: gitlab.ApprovalRegular, Required: 1, Invalid: true},
		{Name: "Security review", Type: gitlab.ApprovalRegular, Required: 1, Approved: true, ApprovedBy: users("lena"), Eligible: users("lena")},
		{Name: "Backend", Type: gitlab.ApprovalRegular, Required: 2, ApprovedBy: users("bob"), Eligible: users("bob", "anna.k", "egor", "dima", "lena", "olga", "kate")},
		{Name: "/docs/", Type: gitlab.ApprovalCodeOwner, Section: "Docs", Required: 1, Eligible: users("lena")},
		{Name: "All Members", Type: gitlab.ApprovalAnyApprover, Required: 1},
	}}
	return mr
}

func TestShownRulesOrder(t *testing.T) {
	var got []string
	for _, r := range shownRules(rulesMR().Approvals.Rules) {
		got = append(got, r.Name)
	}
	want := []string{"Backend", "/docs/", "All Members", "Security", "Security review"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestRuleTitleAndCount(t *testing.T) {
	cases := []struct {
		r     gitlab.ApprovalRule
		title string
	}{
		{gitlab.ApprovalRule{Name: "/docs/", Type: gitlab.ApprovalCodeOwner, Section: "Docs"}, "Docs · /docs/"},
		{gitlab.ApprovalRule{Name: "/docs/", Type: gitlab.ApprovalCodeOwner, Section: "codeowners"}, "/docs/"},
		{gitlab.ApprovalRule{Name: "/docs/", Type: gitlab.ApprovalCodeOwner}, "/docs/"},
		{gitlab.ApprovalRule{Name: "Backend", Type: gitlab.ApprovalRegular, Section: "Docs"}, "Backend"},
	}
	for _, tc := range cases {
		if got := ruleTitle(tc.r); got != tc.title {
			t.Errorf("ruleTitle(%+v) = %q, want %q", tc.r, got, tc.title)
		}
	}
	if h, r := ruleCount(gitlab.ApprovalRule{Required: 1, ApprovedBy: users("a", "b")}); h != 1 || r != 1 {
		t.Errorf("count = %d/%d, want capped 1/1", h, r)
	}
}

func TestCandidatesSkipApproved(t *testing.T) {
	r := gitlab.ApprovalRule{ApprovedBy: users("bob"), Eligible: users("bob", "anna.k", "egor")}
	if got := candidates(r); !reflect.DeepEqual(got, []string{"anna.k", "egor"}) {
		t.Fatalf("candidates = %v", got)
	}
}

func TestFitNames(t *testing.T) {
	names := []string{"anna.k", "egor", "dima", "lena", "olga", "bob"}
	cases := []struct {
		width int
		shown []string
		more  int
	}{
		{80, names, 0},
		{30, names[:4], 2},
		{20, names[:2], 4},
		{10, names[:1], 5},
	}
	for _, tc := range cases {
		shown, more := fitNames(names, tc.width)
		if !reflect.DeepEqual(shown, tc.shown) || more != tc.more {
			t.Errorf("width %d: %v +%d, want %v +%d", tc.width, shown, more, tc.shown, tc.more)
		}
		if w := len(strings.Join(shown, ", ")) + len(" +") + len(string(rune('0'+more))); more > 0 && w > tc.width {
			t.Errorf("width %d: line is %d wide", tc.width, w)
		}
	}
	shown, more := fitNames([]string{"a-very-long-username", "x"}, 8)
	if len(shown) != 1 || more != 1 || ansi.StringWidth(shown[0]) > 5 || !strings.HasSuffix(shown[0], "…") {
		t.Errorf("cut name = %v +%d", shown, more)
	}
	if shown, more := fitNames(nil, 10); shown != nil || more != 0 {
		t.Errorf("empty = %v +%d", shown, more)
	}
}

func TestNeedsMyApproval(t *testing.T) {
	mr := rulesMR()
	if !needsMyApproval(mr, "anna.k") || !needsMyApproval(mr, "egor") {
		t.Error("candidate of an unmet rule must be asked")
	}
	if needsMyApproval(mr, "bob") {
		t.Error("bob already approved")
	}
	if needsMyApproval(mr, "olga") != true {
		t.Error("olga is a candidate of Backend")
	}
	if needsMyApproval(mr, "nobody") || needsMyApproval(mr, "") {
		t.Error("not a candidate")
	}
	// lena is a candidate of Docs, but approved Security review: still asked by Docs.
	if !needsMyApproval(mr, "lena") {
		t.Error("lena is a candidate of an unmet rule")
	}
	// Only any_approver and invalid rules left: nobody is asked.
	only := baseMR()
	only.Approvals.Rules = []gitlab.ApprovalRule{
		{Name: "Any", Type: gitlab.ApprovalAnyApprover, Required: 1, Eligible: users("egor")},
		{Name: "Bad", Type: gitlab.ApprovalRegular, Required: 1, Invalid: true, Eligible: users("egor")},
	}
	if needsMyApproval(only, "egor") {
		t.Error("any_approver and invalid rules do not ask")
	}
	mr.Approvals.ApprovedByMe = true
	if needsMyApproval(mr, "egor") {
		t.Error("ApprovedByMe")
	}
}

func TestNeedsMyApprovalWithoutRules(t *testing.T) {
	mr := baseMR()
	mr.Reviewers = users("egor", "dima")
	mr.ReviewStates = map[string]gitlab.ReviewState{"dima": gitlab.ReviewRequestedChanges}
	if !needsMyApproval(mr, "egor") {
		t.Error("waiting reviewer must be asked")
	}
	if needsMyApproval(mr, "dima") {
		t.Error("dima requested changes")
	}
	mr.ReviewStates["egor"] = gitlab.ReviewApproved
	if needsMyApproval(mr, "egor") {
		t.Error("approved reviewer")
	}
	if needsMyApproval(mr, "anna.k") {
		t.Error("not a reviewer")
	}
}

func TestApprovalLinesRules(t *testing.T) {
	d, _, scr, _ := setup(t, rulesMR())
	_ = d
	st, m := scr.env.Styles, scr.env.Msg.MR
	txt := approvalText{none: m.None, needYour: m.NeedYourApproval, anyApprover: m.AnyApprover, invalid: m.InvalidRule, changes: m.ChangesAsked, needMore: m.NeedApprovals}
	lines := approvalLines(st, txt, rulesMR(), "egor", sideW)
	var plain []string
	for _, l := range lines {
		plain = append(plain, ansi.Strip(l))
		if w := ansi.StringWidth(l); w > sideW {
			t.Errorf("line %q is %d wide, column is %d", ansi.Strip(l), w, sideW)
		}
	}
	want := []string{
		"needs your approval",
		"○ Backend                      1/2",
		"  ✓ bob",
		"  ○ anna.k, egor, dima, lena +2",
		"○ Docs · /docs/                0/1",
		"  ○ lena",
		"○ All Members                  0/1",
		"  any eligible approver",
		"✗ Security                     0/1",
		"  invalid rule",
		"✓ Security review              1/1",
	}
	if !reflect.DeepEqual(plain, want) {
		t.Fatalf("lines:\n%s\nwant:\n%s", strings.Join(plain, "\n"), strings.Join(want, "\n"))
	}
}

func TestApprovalLinesReviewers(t *testing.T) {
	mr := baseMR()
	mr.Reviewers = users("bob", "dima", "lena")
	mr.ReviewStates = map[string]gitlab.ReviewState{"bob": gitlab.ReviewApproved, "dima": gitlab.ReviewRequestedChanges, "lena": gitlab.ReviewUnreviewed}
	mr.Approvals = gitlab.Approvals{Required: 2, Left: 1, ApprovedBy: users("bob", "olga")}
	_, _, scr, _ := setup(t, mr)
	m := scr.env.Msg.MR
	txt := approvalText{none: m.None, needYour: m.NeedYourApproval, anyApprover: m.AnyApprover, invalid: m.InvalidRule, changes: m.ChangesAsked, needMore: m.NeedApprovals}
	var plain []string
	for _, l := range approvalLines(scr.env.Styles, txt, mr, "lena", sideW) {
		plain = append(plain, ansi.Strip(l))
	}
	want := []string{"needs your approval", "✓ bob", "✗ dima changes requested", "○ lena", "✓ olga", "1 more needed"}
	if !reflect.DeepEqual(plain, want) {
		t.Fatalf("lines = %q, want %q", plain, want)
	}

	mr.Reviewers, mr.ReviewStates, mr.Approvals = nil, nil, gitlab.Approvals{}
	got := approvalLines(scr.env.Styles, txt, mr, "lena", sideW)
	if len(got) != 1 || ansi.Strip(got[0]) != m.None {
		t.Fatalf("empty column = %q", got)
	}
}
