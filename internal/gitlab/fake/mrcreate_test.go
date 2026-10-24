package fake

import (
	"context"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestFakeMRCreate(t *testing.T) {
	c := New()
	ctx := context.Background()
	p := gitlab.ProjectRef{FullPath: "platform/route-service"}
	c.AddMR(p.FullPath, gitlab.MergeRequest{IID: 482, Title: "Older", State: gitlab.MRStateOpened})
	c.SetMembers(p.FullPath, gitlab.User{ID: 3, Username: "anna.k"})
	mr, err := c.MergeRequests().Create(ctx, p, gitlab.NewMR{
		SourceBranch: "feat/surge", TargetBranch: "main", Title: "Draft: Fix", ReviewerIDs: []int64{3}, AssigneeIDs: []int64{1},
	})
	if err != nil || mr.IID != 483 || !mr.Draft || mr.Reviewers[0].Username != "anna.k" || mr.Assignees[0].Username != "egor" {
		t.Fatalf("mr = %+v, err = %v", mr, err)
	}
	got, err := c.MergeRequests().Get(ctx, p, 483)
	if err != nil || got.SourceBranch != "feat/surge" {
		t.Fatalf("get = %+v, err = %v", got, err)
	}
	page, _ := c.MergeRequests().List(ctx, p, gitlab.MRFilter{State: gitlab.MRStateOpened, SourceBranch: "feat/surge"}, "", 5)
	if len(page.Items) != 1 {
		t.Fatalf("list = %+v", page.Items)
	}
}

func TestFakeMRTemplates(t *testing.T) {
	c := New()
	ctx := context.Background()
	p := gitlab.ProjectRef{FullPath: "platform/route-service"}
	c.SetMRTemplates(p.FullPath, "Default", "## What\n", "Bug", "## Steps\n")
	names, _ := c.Projects().MRTemplates(ctx, p)
	if len(names) != 2 || names[1] != "Bug" {
		t.Fatalf("names = %v", names)
	}
	if body, err := c.Projects().MRTemplate(ctx, p, "Bug"); err != nil || body != "## Steps\n" {
		t.Fatalf("body = %q, err = %v", body, err)
	}
}
