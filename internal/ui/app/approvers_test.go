package app

import (
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
)

func people(names ...string) []gitlab.User {
	out := make([]gitlab.User, len(names))
	for i, n := range names {
		out[i] = gitlab.User{Username: n, Name: strings.ToUpper(n[:1]) + n[1:], WebURL: "https://gitlab.example.com/" + n}
	}
	return out
}

func approversMR() gitlab.MergeRequest {
	return gitlab.MergeRequest{
		IID: 479, State: gitlab.MRStateOpened, Detailed: true,
		Approvals: gitlab.Approvals{Rules: []gitlab.ApprovalRule{
			{Name: "Backend", Type: gitlab.ApprovalRegular, Required: 2, ApprovedBy: people("bob"), Eligible: people("bob", "anna.k", "dima")},
			{Name: "Docs", Type: gitlab.ApprovalCodeOwner, Required: 1, Eligible: people("lena")},
		}},
	}
}

func selectedName(t *testing.T, o *ApproversOverlay) string {
	t.Helper()
	r, ok := o.modal.Selected()
	if !ok {
		t.Fatal("no person selected")
	}
	return r.Rule + "/" + r.User.Username
}

func TestApproversOverlayMoveSkipsRuleRows(t *testing.T) {
	o := NewApproversOverlay(NewEnv(i18n.EN), "k", "!479", approversMR())
	var got []string
	got = append(got, selectedName(t, o))
	for range 4 {
		o.Update(keyPress("j"))
		got = append(got, selectedName(t, o))
	}
	// Backend: bob (approved), anna.k, dima; the Docs header is skipped; Docs: lena; the end stays.
	want := []string{"regular||Backend/bob", "regular||Backend/anna.k", "regular||Backend/dima", "code_owner||Docs/lena", "code_owner||Docs/lena"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("path = %v, want %v", got, want)
		}
	}
	o.Update(keyPress("k"))
	o.Update(keyPress("k"))
	o.Update(keyPress("k"))
	o.Update(keyPress("k"))
	if selectedName(t, o) != want[0] {
		t.Fatalf("k stops at the first person: %s", selectedName(t, o))
	}
	o.Update(keyPress("G"))
	if selectedName(t, o) != want[3] {
		t.Fatalf("G = %s", selectedName(t, o))
	}
	o.Update(keyPress("g"))
	if selectedName(t, o) != want[0] {
		t.Fatalf("g = %s", selectedName(t, o))
	}
}

func TestApproversOverlayRefreshKeepsSelection(t *testing.T) {
	o := NewApproversOverlay(NewEnv(i18n.EN), "k", "!479", approversMR())
	o.Update(keyPress("j"))
	o.Update(keyPress("j")) // dima
	mr := approversMR()
	mr.Approvals.Rules[0].Eligible = people("olga", "bob", "anna.k", "dima") // a new first candidate
	o.Update(ApproversRefreshMsg{Key: "other", MR: gitlab.MergeRequest{}})
	o.Update(ApproversRefreshMsg{Key: "k", MR: mr})
	if got := selectedName(t, o); got != "regular||Backend/dima" {
		t.Fatalf("selection moved: %s", got)
	}
	// dima approved: the pair is gone from the candidates, found by the username in the same rule.
	mr.Approvals.Rules[0].ApprovedBy = people("bob", "dima")
	o.Update(ApproversRefreshMsg{Key: "k", MR: mr})
	if got := selectedName(t, o); got != "regular||Backend/dima" {
		t.Fatalf("selection lost: %s", got)
	}
	// dima left every rule: back to the first person.
	mr.Approvals.Rules[0].ApprovedBy = people("bob")
	mr.Approvals.Rules[0].Eligible = people("bob", "anna.k")
	o.Update(ApproversRefreshMsg{Key: "k", MR: mr})
	if got := selectedName(t, o); got != "regular||Backend/bob" {
		t.Fatalf("selection = %s, want the first person", got)
	}
}

func TestApproversOverlayActions(t *testing.T) {
	env := NewEnv(i18n.EN)
	var opened []string
	env.Browser = func(u string) error { opened = append(opened, u); return nil }
	o := NewApproversOverlay(env, "k", "!479", approversMR())
	o.Update(keyPress("j"))
	next, cmd := o.Update(keyPress("enter"))
	if next != nil || cmd == nil {
		t.Fatal("enter closes and opens the profile")
	}
	cmd()
	if len(opened) != 1 || opened[0] != "https://gitlab.example.com/anna.k" {
		t.Fatalf("opened = %v", opened)
	}
	// A person without a profile link: enter does nothing.
	mr := approversMR()
	mr.Approvals.Rules[0].ApprovedBy[0].WebURL = ""
	o = NewApproversOverlay(env, "k", "!479", mr)
	if next, cmd := o.Update(keyPress("enter")); next == nil || cmd != nil {
		t.Fatalf("enter without a link must do nothing: %v %v", next, cmd)
	}
	if next, cmd := o.Update(keyPress("y")); next != nil || cmd == nil {
		t.Fatalf("y must copy and close: %v %v", next, cmd)
	}
	if !strings.Contains(o.View(120, 40), "Approvers · !479") {
		t.Error("title missing")
	}
}

func TestOpenApproversToastWhenNobody(t *testing.T) {
	env := NewEnv(i18n.EN)
	if cmd := OpenApprovers(env, "k", "!1", gitlab.MergeRequest{IID: 1, Detailed: true}); cmd == nil {
		t.Fatal("a toast command is expected")
	} else if msg, ok := cmd().(ToastMsg); !ok || msg.Text != "No approvers in the MR" {
		t.Fatalf("msg = %#v", cmd())
	}
	if _, ok := OpenApprovers(env, "k", "!479", approversMR())().(OpenOverlayMsg); !ok {
		t.Fatal("an MR with people opens the overlay")
	}
}
