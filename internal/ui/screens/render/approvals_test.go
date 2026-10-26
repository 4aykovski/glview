package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
)

func users(names ...string) []gitlab.User {
	out := make([]gitlab.User, len(names))
	for i, n := range names {
		out[i] = gitlab.User{Username: n}
	}
	return out
}

func rulesMR() gitlab.MergeRequest {
	return gitlab.MergeRequest{
		State: gitlab.MRStateOpened, Detailed: true, Reviewers: users("bob", "dima", "lena"),
		Approvals: gitlab.Approvals{Required: 2, Left: 1, ApprovedBy: users("bob", "olga"), Rules: []gitlab.ApprovalRule{
			{Name: "Legacy", Type: gitlab.ApprovalRegular, Required: 0, Approved: true, Eligible: users("dima")},
			{Name: "Security", Type: gitlab.ApprovalRegular, Required: 1, Invalid: true},
			{Name: "Security review", Type: gitlab.ApprovalRegular, Required: 1, Approved: true, ApprovedBy: users("lena"), Eligible: users("lena")},
			{Name: "Backend", Type: gitlab.ApprovalRegular, Required: 2, ApprovedBy: users("bob"), Eligible: users("bob", "anna.k", "egor", "dima", "lena", "olga", "kate")},
			{Name: "/docs/", Type: gitlab.ApprovalCodeOwner, Section: "Docs", Required: 1, Eligible: users("lena")},
			{Name: "All Members", Type: gitlab.ApprovalAnyApprover, Required: 1},
		}},
	}
}

func ruleNames(a ApprovalView) []string {
	var out []string
	for _, r := range a.Rules {
		out = append(out, r.Rule.Name)
	}
	return out
}

func TestShownRulesOrder(t *testing.T) {
	got := ruleNames(NewApprovalView(rulesMR(), "egor"))
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
		if got := RuleTitle(tc.r); got != tc.title {
			t.Errorf("RuleTitle(%+v) = %q, want %q", tc.r, got, tc.title)
		}
	}
	if got := RuleCount(gitlab.ApprovalRule{Required: 1, ApprovedBy: users("a", "b")}); got != "1/1" {
		t.Errorf("count = %s, want capped 1/1", got)
	}
}

func TestCandidatesSkipApproved(t *testing.T) {
	a := NewApprovalView(rulesMR(), "egor")
	got := Names(a.Rules[0].Cands)
	if got != "anna.k, egor, dima, lena, olga, kate" {
		t.Fatalf("candidates = %s", got)
	}
}

func TestFitNames(t *testing.T) {
	us := users("anna.k", "egor", "dima", "lena", "olga", "bob")
	for width, want := range map[int]int{80: 6, 30: 4, 20: 2, 10: 1, 1: 1} {
		if got := fitNames(us, width); got != want {
			t.Errorf("width %d: %d names, want %d", width, got, want)
		}
	}
	if fitNames(nil, 10) != 0 {
		t.Error("empty")
	}
	st := common.NewStyles(common.TerminalTheme(true))
	line, more := nameLine(st, "○", users("a-very-long-username", "x"), "", 12)
	if more != 1 || ansi.StringWidth(line) > 12 || !strings.Contains(line, "…") {
		t.Errorf("cut line = %q +%d", ansi.Strip(line), more)
	}
}

func TestNeedsMyApproval(t *testing.T) {
	mr := rulesMR()
	for me, want := range map[string]bool{"anna.k": true, "egor": true, "olga": false, "lena": true, "bob": false, "nobody": false, "": false} {
		if got := NewApprovalView(mr, me).Needs; got != want {
			t.Errorf("%q: Needs = %v, want %v", me, got, want)
		}
	}
	for _, st := range []gitlab.MRState{gitlab.MRStateMerged, gitlab.MRStateClosed} {
		m := rulesMR()
		m.State = st
		if NewApprovalView(m, "anna.k").Needs {
			t.Errorf("state %s must not ask", st)
		}
	}
	only := rulesMR()
	only.Approvals.Rules = []gitlab.ApprovalRule{
		{Name: "Any", Type: gitlab.ApprovalAnyApprover, Required: 1, Eligible: users("egor")},
		{Name: "Bad", Type: gitlab.ApprovalRegular, Required: 1, Invalid: true, Eligible: users("egor")},
	}
	if NewApprovalView(only, "egor").Needs {
		t.Error("any_approver and invalid rules do not ask")
	}
	mr.Approvals.ApprovedByMe = true
	if NewApprovalView(mr, "anna.k").Needs {
		t.Error("ApprovedByMe")
	}
}

func TestNeedsMyApprovalWithoutRules(t *testing.T) {
	mr := gitlab.MergeRequest{State: gitlab.MRStateOpened, Detailed: true, Reviewers: users("egor", "dima"),
		ReviewStates: map[string]gitlab.ReviewState{"dima": gitlab.ReviewRequestedChanges}}
	if !NewApprovalView(mr, "egor").Needs {
		t.Error("waiting reviewer must be asked")
	}
	if NewApprovalView(mr, "dima").Needs || NewApprovalView(mr, "anna.k").Needs {
		t.Error("changes requested / not a reviewer")
	}
	mr.ReviewStates["egor"] = gitlab.ReviewApproved
	if NewApprovalView(mr, "egor").Needs {
		t.Error("approved reviewer")
	}
}

func column(a ApprovalView, w int) []string {
	st := common.NewStyles(common.TerminalTheme(true))
	var out []string
	for _, l := range ApprovalColumn(st, i18n.Get(i18n.EN), a, w) {
		if got := ansi.StringWidth(l); got > w {
			panic("line wider than the column: " + ansi.Strip(l))
		}
		out = append(out, ansi.Strip(l))
	}
	return out
}

func TestApprovalColumnRules(t *testing.T) {
	got := column(NewApprovalView(rulesMR(), "egor"), 34)
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
		"  ✓ lena",
		"✓ olga",
		"A — all",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Nothing is cut and I am not asked: no hint, no warning.
	mr := rulesMR()
	mr.Approvals.Rules[3].Eligible = users("bob", "anna.k")
	got = column(NewApprovalView(mr, "bob"), 34)
	if got[0] != "○ Backend                      1/2" || got[len(got)-1] == "A — all" {
		t.Fatalf("lines = %v", got)
	}
}

func TestApprovalColumnReviewers(t *testing.T) {
	mr := gitlab.MergeRequest{State: gitlab.MRStateOpened, Detailed: true, Reviewers: users("bob", "dima", "lena"),
		ReviewStates: map[string]gitlab.ReviewState{"bob": gitlab.ReviewApproved, "dima": gitlab.ReviewRequestedChanges, "lena": gitlab.ReviewUnreviewed},
		Approvals:    gitlab.Approvals{Required: 2, Left: 1, ApprovedBy: users("bob", "olga")}}
	got := column(NewApprovalView(mr, "lena"), 34)
	want := []string{"needs your approval", "✓ bob", "✗ dima changes requested", "○ lena", "✓ olga", "1 more needed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
	empty := gitlab.MergeRequest{State: gitlab.MRStateOpened, Detailed: true}
	if got := column(NewApprovalView(empty, "lena"), 34); !reflect.DeepEqual(got, []string{"none"}) && len(got) != 1 {
		t.Fatalf("empty column = %q", got)
	}
}

// Before the details load (a list row) the column is the plain approver
// list: no reviewers, no warning.
func TestApprovalColumnBeforeDetails(t *testing.T) {
	mr := rulesMR()
	mr.Detailed = false
	mr.Approvals.Rules = nil
	got := column(NewApprovalView(mr, "anna.k"), 34)
	want := []string{"✓ bob", "✓ olga", "1 more needed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
	if NewApprovalView(mr, "lena").Needs {
		t.Error("no warning before the details")
	}
	mr.Approvals = gitlab.Approvals{}
	if got := column(NewApprovalView(mr, "lena"), 34); len(got) != 1 {
		t.Fatalf("empty = %q", got)
	}
}

func TestApproverRows(t *testing.T) {
	st := common.NewStyles(common.TerminalTheme(true))
	rows := ApproverRows(st, i18n.Get(i18n.EN), NewApprovalView(rulesMR(), "egor"))
	var got []string
	for _, r := range rows {
		line := ansi.Strip(r.Render(40))
		if r.Person {
			line = "  " + line
		}
		got = append(got, line)
	}
	want := []string{
		"○ Backend                            1/2",
		"  ✓ bob",
		"  ○ anna.k",
		"  ○ egor",
		"  ○ dima",
		"  ○ lena",
		"  ○ olga",
		"  ○ kate",
		"○ Docs · /docs/                      0/1",
		"  ○ lena",
		"○ All Members                        0/1",
		"  any eligible approver",
		"✗ Security                           0/1",
		"  invalid rule",
		"✓ Security review                    1/1",
		"  ✓ lena",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !NewApprovalView(rulesMR(), "egor").People() || NewApprovalView(gitlab.MergeRequest{Detailed: true}, "egor").People() {
		t.Error("People")
	}
}
