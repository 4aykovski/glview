package newmr

import (
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
)

func created(o *opened) *CreatedMsg {
	for _, m := range o.d.Msgs {
		if c, ok := m.(CreatedMsg); ok {
			return &c
		}
	}
	return nil
}

func TestTemplateDefaultFillsEmptyDescription(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "feat/surge")
	o.d.RequireView("Template", "Default")
	if got := o.f.Description(); got != "## What\n\n## Why\n" {
		t.Fatalf("description = %q", got)
	}
	if n := c.CallCount("Projects.MRTemplates"); n != 1 {
		t.Fatalf("template list requested %d times", n)
	}
}

func TestTemplateFailureAndNone(t *testing.T) {
	c := newFake()
	c.FailNext("Projects.MRTemplates", &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	o := open(t, c, i18n.EN, "feat/surge")
	o.d.RequireView("templates unavailable")
	if o.f.Description() != "" {
		t.Fatal("no template, no text")
	}

	c = newFake()
	c.SetMRTemplates(path)
	o = open(t, c, i18n.RU, "feat/surge")
	o.d.RequireView("без шаблона")
}

func TestTemplateReplaceAsksAfterEditing(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fDesc)
	o.typeText("my words")
	o.focusOn(fTemplate)
	o.d.Key("down", "enter") // "Bug"
	o.d.RequireView("Replace the description with the template “Bug”?")
	o.d.Key("esc")
	o.d.RequireNoView("Replace the description")
	if got := o.f.Description(); got == "## Steps\n" || !strings.Contains(got, "my words") {
		t.Fatalf("description = %q, cancel keeps it", got)
	}
	o.d.Key("enter") // highlighted "Bug" again, confirm
	o.d.Key("y")
	if got := o.f.Description(); got != "## Steps\n" {
		t.Fatalf("description = %q after Yes", got)
	}
}

func TestTemplateReplaceWithoutAskingWhenUntouched(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.focusOn(fTemplate)
	o.d.Key("down", "enter")
	o.d.RequireNoView("Replace the description")
	if got := o.f.Description(); got != "## Steps\n" {
		t.Fatalf("description = %q", got)
	}
	o.d.Key("up", "up", "enter") // no template: the text stays
	if o.f.Description() != "## Steps\n" {
		t.Fatal("no template does not touch the description")
	}
}

func TestAtMeFirstAndChips(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.focusOn(fAssignees)
	o.d.RequireView("› @me · egor", "anna.k")
	o.d.Key("enter") // @me
	o.d.RequireView("egor ✕")
	o.d.RequireNoView("@me")
	o.d.Key("enter") // anna.k: @me is gone from the list
	if got := names(o.f.assignees); got != "egor,anna.k" {
		t.Fatalf("assignees = %s", got)
	}
	o.d.Key("backspace")
	if got := names(o.f.assignees); got != "egor" {
		t.Fatalf("assignees = %s after backspace in an empty field", got)
	}
}

func names(h *hintInput) string {
	s := ""
	for i, n := range h.names() {
		if i > 0 {
			s += ","
		}
		s += n
	}
	return s
}

func TestMilestoneIsASingleValue(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.focusOn(fMilestone)
	o.d.RequireView("› 2026-Q4", "2026-Q3")
	o.d.Key("enter")
	if got := names(o.f.milestone); got != "2026-Q4" {
		t.Fatalf("milestone = %s", got)
	}
	o.d.RequireNoView("› 2026-Q3")
	o.typeText("zzz")
	if o.f.milestone.in.Value() != "" {
		t.Fatal("a chosen milestone takes no more text")
	}
	o.d.Key("backspace")
	if got := names(o.f.milestone); got != "" {
		t.Fatalf("milestone = %q after backspace", got)
	}
}

func TestLabelSuggestionsSkipChosen(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.focusOn(fLabels)
	o.d.Key("enter") // bug
	o.d.RequireView("› pricing")
	o.d.RequireNoView("› bug")
}

func TestSquashLockedByProject(t *testing.T) {
	c := newFake()
	c.AddProject(gitlab.Project{ProjectRef: project, DefaultBranch: "main", SquashOption: "always"})
	o := open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fSquash)
	o.d.RequireView("[x] squash · by project setting")
	o.d.Key("space")
	o.d.RequireView("[x] squash")

	c.AddProject(gitlab.Project{ProjectRef: project, DefaultBranch: "main", SquashOption: "never", RemoveSourceBranchAfterMerge: false})
	o = open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fSquash)
	o.d.Key("space")
	o.d.RequireView("[ ] squash · by project setting", "[ ] delete branch after merge")
}

func TestFlagsFollowTheProject(t *testing.T) {
	c := newFake()
	c.AddProject(gitlab.Project{ProjectRef: project, DefaultBranch: "main", RemoveSourceBranchAfterMerge: true, SquashOption: "default_on"})
	o := open(t, c, i18n.EN, "feat/surge")
	o.d.RequireView("[x] delete branch after merge", "[x] squash")
	o.focusOn(fSquash)
	o.d.Key("space")
	o.d.RequireView("[ ] squash")
}

func TestPreview(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.focusOn(fDraft)
	o.d.Key("space")
	o.focusOn(fAssignees)
	o.d.Key("enter") // @me
	o.focusOn(fReviewers)
	o.d.Key("down", "enter", "enter")
	o.focusOn(fLabels)
	o.d.Key("enter")
	o.d.RequireView("will be created: feat/surge → main · draft · assignee egor · reviewers 2 · labels 1")
	o.focusOn(fMilestone)
	o.d.Key("enter")
	o.d.RequireView("milestone 2026-Q4")
}

func TestCreateSuccess(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fDraft)
	o.d.Key("space")
	o.focusOn(fAssignees)
	o.d.Key("enter")
	o.focusOn(fLabels)
	o.d.Key("enter")
	o.focusOn(fMilestone)
	o.d.Key("enter")
	o.focusOn(fButton)
	o.d.Key("enter")
	calls := o.creates()
	if len(calls) != 1 {
		t.Fatalf("Create calls = %v", calls)
	}
	n := calls[0][1].(gitlab.NewMR)
	if n.SourceBranch != "feat/surge" || n.TargetBranch != "main" || n.Title != "Draft: Feat/surge" || n.Description != "## What\n\n## Why\n" ||
		len(n.AssigneeIDs) != 1 || n.AssigneeIDs[0] != 1 || len(n.Labels) != 1 || n.Labels[0] != "bug" || n.MilestoneID != 1 || !n.RemoveSourceBranch || n.Squash {
		t.Fatalf("NewMR = %+v", n)
	}
	cm := created(o)
	if cm == nil || cm.MR.IID != 1 || cm.Project != project {
		t.Fatalf("CreatedMsg = %+v", cm)
	}
	o.d.RequireView("MR !1 created", "platform/route-service › MRs › !1")
	o.d.RequireNoView("new MR")
	o.d.Key("esc")
	if o.d.Model.(*app.Model).Depth() != 1 {
		t.Fatal("esc from the new MR goes back to where the form was opened from")
	}
	page, _ := c.MergeRequests().List(t.Context(), project, gitlab.MRFilter{State: gitlab.MRStateOpened, SourceBranch: "feat/surge"}, "", 5)
	if len(page.Items) != 1 {
		t.Fatalf("the list lacks the new MR: %+v", page.Items)
	}
}

func TestCreateNeedsChanges(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/slots")
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireView("no changes: feat/slots is not ahead of main")
	if len(o.creates()) != 0 {
		t.Fatal("Create is not called for an empty comparison")
	}
	if o.f.creating || o.f.waiting {
		t.Fatal("the form stays unlocked")
	}
}

func TestCreateNeedsBranchOnServer(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/new")
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireView("branch feat/new is not on the server — git push first")
	if len(o.creates()) != 0 {
		t.Fatal("Create is not called")
	}
}

func TestCreateWaitsForTheCheck(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "feat/surge")
	c.SetDelay("Tags.Compare", 500_000_000) // longer than the test driver waits
	o.setBranches("feat/a", "main")
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireView("checking branches…")
	if len(o.creates()) != 0 {
		t.Fatal("nothing is created before the check is back")
	}
	o.d.Key("esc") // stops waiting, does not close
	o.d.RequireNoView("checking branches…")
	o.d.RequireView("new MR")
	if o.f.waiting {
		t.Fatal("still waiting")
	}
}

func TestCreateWithOpenMRAsksToOpenIt(t *testing.T) {
	c := newFake()
	c.AddMR(path, gitlab.MergeRequest{IID: 482, Title: "Fix slot race", State: gitlab.MRStateOpened, SourceBranch: "feat/surge", TargetBranch: "main"})
	o := open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireView("An MR from feat/surge is already open: !482", "Fix slot race", "Open it?")
	o.d.Key("enter") // "Yes" is the default
	if len(o.creates()) != 0 {
		t.Fatal("Create is not called when the existing MR is opened")
	}
	o.d.RequireView("platform/route-service › MRs › !482")
	o.d.RequireNoView("new MR")
}

func TestCreateWithOpenMRCancelStaysInTheForm(t *testing.T) {
	c := newFake()
	c.AddMR(path, gitlab.MergeRequest{IID: 482, Title: "Fix slot race", State: gitlab.MRStateOpened, SourceBranch: "feat/surge", TargetBranch: "main"})
	o := open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fButton)
	o.d.Key("enter", "n")
	o.d.RequireView("new MR", "From branch")
	if len(o.creates()) != 0 {
		t.Fatal("Create is not called")
	}
}

func TestCreateServerError(t *testing.T) {
	c := newFake()
	c.FailNext("MergeRequests.Create", &gitlab.Error{Kind: gitlab.ErrNotPossible, Status: 422, Message: "Source branch is invalid"})
	o := open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fTitle)
	o.typeText(" more")
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireView("Source branch is invalid", "new MR")
	if o.f.creating {
		t.Fatal("the form is locked after an error")
	}
	if got := o.f.TitleText(); got != "Feat/surge more" {
		t.Fatalf("title = %q, the values stay", got)
	}
	o.d.Key("enter") // second try goes through
	if len(o.creates()) != 2 {
		t.Fatalf("Create calls = %d", len(o.creates()))
	}
	o.d.RequireNoView("From branch")
}

func TestLockedWhileCreating(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "feat/surge")
	c.SetDelay("MergeRequests.Create", 500_000_000)
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireView("creating…")
	o.d.Key("esc", "tab", "x")
	o.d.RequireView("creating…", "new MR")
	if o.f.Focus() != fButton {
		t.Fatal("keys do nothing while the request is in flight")
	}
}

func TestDraftIsKeptAndRestored(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fTitle)
	o.typeText(" wip")
	o.focusOn(fAssignees)
	o.d.Key("enter")
	o.focusOn(fRemove)
	o.d.Key("space")
	o.d.Key("esc")
	o.d.RequireNoView("new MR")

	compares := c.CallCount("Tags.Compare")
	f2 := New(o.env, Options{Project: project, Repo: repoOn("feat/surge"), DefaultBranch: "main"})
	o.d.Send(app.PushMsg{Screen: f2})
	if got := f2.TitleText(); got != "Feat/surge wip" {
		t.Fatalf("title = %q", got)
	}
	if got := names(f2.assignees); got != "egor" {
		t.Fatalf("assignees = %q", got)
	}
	o.d.RequireView("[ ] delete branch after merge", "3 commits · 5 files")
	if c.CallCount("Tags.Compare") <= compares {
		t.Fatal("the branch check is asked again")
	}
}

func TestSuccessErasesTheDraft(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "feat/surge")
	o.focusOn(fTitle)
	o.typeText("!")
	o.d.Key("esc")
	f2 := New(o.env, Options{Project: project, Repo: repoOn("feat/surge"), DefaultBranch: "main"})
	o.d.Send(app.PushMsg{Screen: f2})
	o.f = f2
	o.focusOn(fButton)
	o.d.Key("enter")
	if len(o.creates()) != 1 {
		t.Fatal("not created")
	}
	f3 := New(o.env, Options{Project: project, Repo: repoOn("feat/surge"), DefaultBranch: "main"})
	if got := f3.TitleText(); got != "Feat/surge" {
		t.Fatalf("title = %q, the draft is gone after a success", got)
	}
}
