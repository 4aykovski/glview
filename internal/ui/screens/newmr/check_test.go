package newmr

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
)

func netErr() error { return &gitlab.Error{Kind: gitlab.ErrServer, Status: 500} }

func TestDuplicateIsPerPairOfBranches(t *testing.T) {
	c := newFake()
	c.AddMR(path, gitlab.MergeRequest{IID: 482, Title: "Fix slot race", State: gitlab.MRStateOpened, SourceBranch: "feat/surge", TargetBranch: "release-1"})
	o := open(t, c, i18n.EN, "feat/surge")
	o.d.RequireView("3 commits · 5 files")
	o.d.RequireNoView("already open")
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireNoView("Open it?")
	if len(o.creates()) != 1 {
		t.Fatalf("Create called %d times, an MR into another branch is no duplicate", len(o.creates()))
	}
}

func TestNothingIsComparedUntilTheBranchIsOnTheServer(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "feat/new")
	o.d.RequireView("git push first")
	if n := c.CallCount("Tags.Compare") + c.CallCount("MergeRequests.List"); n != 0 {
		t.Fatalf("Compare and the MR lookup ran %d times for a branch absent on the server", n)
	}
	c = newFake()
	open(t, c, i18n.EN, "feat/surge")
	if c.CallCount("Tags.Compare") != 1 || c.CallCount("MergeRequests.List") != 1 {
		t.Fatalf("Compare %d, List %d, want one each", c.CallCount("Tags.Compare"), c.CallCount("MergeRequests.List"))
	}
}

func TestNetworkFailureIsNotReportedAsNotPushed(t *testing.T) {
	c := newFake()
	c.FailNext("Projects.Branches", netErr())
	c.FailNext("Tags.Compare", netErr())
	o := open(t, c, i18n.EN, "feat/surge")
	o.d.RequireView("comparison unavailable")
	o.d.RequireNoView("git push first")
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireNoView("git push first")
	if len(o.creates()) != 0 {
		t.Fatal("Create is not called when the comparison is unavailable")
	}
}

func TestAtMeWaitsForTheUser(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.env.User = gitlab.User{} // not loaded yet
	o.focusOn(fReviewers)
	o.d.RequireView("anna.k")
	o.d.RequireNoView("@me")
	o.d.Key("enter") // anna.k
	o.focusOn(fButton)
	o.d.Key("enter")
	calls := o.creates()
	if len(calls) != 1 {
		t.Fatalf("Create calls = %v", calls)
	}
	n := calls[0][1].(gitlab.NewMR)
	if len(n.ReviewerIDs) != 1 || n.ReviewerIDs[0] == 0 || len(n.AssigneeIDs) != 0 {
		t.Fatalf("NewMR = %+v, an unknown user id 0 is never sent", n)
	}
}

func TestDuplicateLookupFailureIsShownAndDoesNotBlock(t *testing.T) {
	c := newFake()
	c.FailNext("MergeRequests.List", netErr())
	o := open(t, c, i18n.EN, "feat/surge")
	o.d.RequireView("3 commits · 5 files", "could not check for an open MR")
	o.focusOn(fButton)
	o.d.Key("enter")
	if len(o.creates()) != 1 {
		t.Fatalf("Create called %d times, GitLab answers a duplicate itself", len(o.creates()))
	}
}
