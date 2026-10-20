package fake

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestFakeRecordsAndScripts(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	ctx := context.Background()

	if p, err := c.Projects().Get(ctx, gitlab.ProjectRef{FullPath: "a/b"}); err != nil || p.ID != 1 {
		t.Fatalf("get: %+v %v", p, err)
	}
	if _, err := c.Projects().Get(ctx, gitlab.ProjectRef{FullPath: "x/y"}); !errors.Is(err, gitlab.ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}

	c.FailNext("Users.Current", &gitlab.Error{Kind: gitlab.ErrUnauthorized, Status: 401})
	if _, err := c.Users().Current(ctx); !errors.Is(err, gitlab.ErrUnauthorized) {
		t.Fatalf("scripted error: %v", err)
	}
	if u, err := c.Users().Current(ctx); err != nil || u.Username != "egor" {
		t.Fatalf("error must be consumed once: %+v %v", u, err)
	}
	if n := c.CallCount("Projects.Get"); n != 2 {
		t.Fatalf("Projects.Get calls = %d", n)
	}
}

func TestFakeDelayRespectsContext(t *testing.T) {
	c := New()
	c.SetDelay("Users.Current", time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Users().Current(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("delay ignored context")
	}
}

func TestFakeMergeRequestFiltersAndPaging(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	me := c.User
	other := gitlab.User{Username: "anna"}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 5; i++ {
		mr := gitlab.MergeRequest{IID: i, Title: "MR " + string(rune('A'+i)), State: gitlab.MRStateOpened, Author: other,
			UpdatedAt: base.Add(time.Duration(i) * time.Hour)}
		if i == 2 {
			mr.Reviewers = []gitlab.User{me}
		}
		if i == 3 {
			mr.Author = me
			mr.Draft = true
		}
		if i == 4 {
			mr.State = gitlab.MRStateMerged
		}
		c.AddMR("a/b", mr)
	}
	ctx := context.Background()
	ref := gitlab.ProjectRef{FullPath: "a/b"}

	page, err := c.MergeRequests().List(ctx, ref, gitlab.MRFilter{State: gitlab.MRStateOpened}, "", 2)
	if err != nil || len(page.Items) != 2 || page.Total != 4 || page.NextCursor != "2" {
		t.Fatalf("page 1: %+v %v", page, err)
	}
	if page.Items[0].IID != 5 {
		t.Fatalf("newest first, got !%d", page.Items[0].IID)
	}
	page, _ = c.MergeRequests().List(ctx, ref, gitlab.MRFilter{State: gitlab.MRStateOpened}, "2", 2)
	if len(page.Items) != 2 || page.NextCursor != "" {
		t.Fatalf("page 2: %+v", page)
	}
	page, _ = c.MergeRequests().List(ctx, ref, gitlab.MRFilter{State: gitlab.MRStateOpened, Reviewer: "@me"}, "", 20)
	if len(page.Items) != 1 || page.Items[0].IID != 2 {
		t.Fatalf("reviewer filter: %+v", page.Items)
	}
	page, _ = c.MergeRequests().List(ctx, ref, gitlab.MRFilter{State: gitlab.MRStateOpened, Mine: true}, "", 20)
	if len(page.Items) != 2 {
		t.Fatalf("mine filter: %+v", page.Items)
	}
	yes := true
	page, _ = c.MergeRequests().List(ctx, ref, gitlab.MRFilter{State: gitlab.MRStateAll, Draft: &yes}, "", 20)
	if len(page.Items) != 1 || page.Items[0].IID != 3 {
		t.Fatalf("draft filter: %+v", page.Items)
	}

	if err := c.MergeRequests().Approve(ctx, ref, 2, "sha"); err != nil {
		t.Fatal(err)
	}
	mr, _ := c.MergeRequests().Get(ctx, ref, 2)
	if !mr.Approvals.ApprovedByMe {
		t.Fatal("approve must set ApprovedByMe")
	}
	if got := c.Calls()[len(c.Calls())-2]; got.Method != "MergeRequests.Approve" || got.Args[2] != "sha" {
		t.Fatalf("journal: %+v", got)
	}
}

func TestFakeMembersAndLabelsFilterBySubstring(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	c.SetMembers("a/b", gitlab.User{Username: "anna.k", Name: "Anna"}, gitlab.User{Username: "bob", Name: "Bob"})
	c.SetLabels("a/b", "backend", "bug", "docs")
	ref := gitlab.ProjectRef{FullPath: "a/b"}
	us, _ := c.Projects().Members(context.Background(), ref, "an")
	if len(us) != 1 || us[0].Username != "anna.k" {
		t.Fatalf("members = %+v", us)
	}
	ls, _ := c.Projects().Labels(context.Background(), ref, "b")
	if len(ls) != 2 {
		t.Fatalf("labels = %v", ls)
	}
	if all, _ := c.Projects().Labels(context.Background(), ref, ""); len(all) != 3 {
		t.Fatalf("empty search lists all: %v", all)
	}
}

func TestFakeMergeFillsMergeFields(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	c.AddMR("a/b", gitlab.MergeRequest{IID: 1, State: gitlab.MRStateOpened})
	mr, err := c.MergeRequests().Merge(context.Background(), gitlab.ProjectRef{FullPath: "a/b"}, 1, gitlab.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if mr.MergedAt.IsZero() || mr.MergedBy == nil || mr.MergedBy.Username != c.User.Username {
		t.Errorf("merge fields = %v %+v", mr.MergedAt, mr.MergedBy)
	}
}

func TestFakeRecheckMergeStatus(t *testing.T) {
	c := New()
	ref := gitlab.ProjectRef{ID: 1, FullPath: "a/b"}
	c.AddProject(gitlab.Project{ProjectRef: ref})
	c.AddMR("a/b", gitlab.MergeRequest{IID: 1, State: gitlab.MRStateOpened, MergeStatus: "unchecked"})
	c.AddMR("a/b", gitlab.MergeRequest{IID: 2, State: gitlab.MRStateOpened, MergeStatus: "mergeable"})
	c.SetRecheckResult("a/b", 1, "not_approved")
	ctx := context.Background()
	if st, err := c.MergeRequests().RecheckMergeStatus(ctx, ref, 1); err != nil || st != "checking" {
		t.Fatalf("recheck = %q %v", st, err)
	}
	mr, _ := c.MergeRequests().Get(ctx, ref, 1)
	if mr.MergeStatus != "not_approved" {
		t.Errorf("status after Get = %q", mr.MergeStatus)
	}
	if st, _ := c.MergeRequests().RecheckMergeStatus(ctx, ref, 2); st != "mergeable" {
		t.Errorf("other status must stay, got %q", st)
	}
	if c.CallCount("MergeRequests.RecheckMergeStatus") != 2 {
		t.Error("calls must be recorded")
	}
}

func TestFakeRefSearchAnchorsAndLimit(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	c.SetTags("a/b", gitlab.Tag{Name: "v1.14.2"}, gitlab.Tag{Name: "v1.14.20"}, gitlab.Tag{Name: "rc-v1.14.2"})
	ref := gitlab.ProjectRef{FullPath: "a/b"}
	ctx := context.Background()
	for search, want := range map[string]int{"v1.14.2": 3, "^v1.14.2": 2, "v1.14.2$": 2, "^v1.14.2$": 1} {
		if page, _ := c.Tags().List(ctx, ref, search, "", 20); len(page.Items) != want {
			t.Errorf("tags %q = %+v, want %d", search, page.Items, want)
		}
	}
	names := make([]string, 25)
	for i := range names {
		names[i] = "feat/b" + string(rune('a'+i))
	}
	c.SetBranches("a/b", names...)
	if bs, _ := c.Projects().Branches(ctx, ref, ""); len(bs) != 20 {
		t.Fatalf("branches = %d, want the REST page size 20", len(bs))
	}
}

func TestFakePipelineScope(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	c.SetTags("a/b", gitlab.Tag{Name: "v1.14.2"})
	c.AddPipeline("a/b", gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 1, Ref: "v1.14.2"}})
	c.AddPipeline("a/b", gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 2, Ref: "main"}})
	ref := gitlab.ProjectRef{FullPath: "a/b"}
	ctx := context.Background()
	tags, _ := c.Pipelines().List(ctx, ref, gitlab.PipelineFilter{Scope: "tags"}, "", 10)
	branches, _ := c.Pipelines().List(ctx, ref, gitlab.PipelineFilter{Scope: "branches"}, "", 10)
	if len(tags.Items) != 1 || tags.Items[0].ID != 1 || len(branches.Items) != 1 || branches.Items[0].ID != 2 {
		t.Fatalf("tags = %+v, branches = %+v", tags.Items, branches.Items)
	}
	// Other scopes are not filters of the fake, as in the GraphQL client.
	other, _ := c.Pipelines().List(ctx, ref, gitlab.PipelineFilter{Scope: "finished"}, "", 10)
	if len(other.Items) != 2 {
		t.Fatalf("scope finished = %+v", other.Items)
	}
}

func TestFakeMRCommits(t *testing.T) {
	c := New()
	ref := gitlab.ProjectRef{ID: 1, FullPath: "a/b"}
	c.AddProject(gitlab.Project{ProjectRef: ref})
	c.AddMR("a/b", gitlab.MergeRequest{IID: 1, State: gitlab.MRStateOpened})
	ctx := context.Background()
	if got, err := c.MergeRequests().Commits(ctx, ref, 1); err != nil || len(got) != 0 {
		t.Fatalf("no commits yet: %+v %v", got, err)
	}
	commits := []gitlab.CommitSummary{
		{SHA: "b2c3d4e5f6", ShortSHA: "b2c3d4e5", Title: "slot: fix race", AuthorName: "Anna"},
		{SHA: "a1b2c3d4e5", ShortSHA: "a1b2c3d4", Title: "slot: add test", AuthorName: "Egor"},
	}
	c.SetMRCommits("a/b", 1, commits)
	got, err := c.MergeRequests().Commits(ctx, ref, 1)
	if err != nil || len(got) != 2 || got[0].ShortSHA != "b2c3d4e5" {
		t.Fatalf("commits = %+v %v", got, err)
	}
	got[0].Title = "changed"
	if again, _ := c.MergeRequests().Commits(ctx, ref, 1); again[0].Title != "slot: fix race" {
		t.Error("result must be a copy")
	}
	c.FailNext("MergeRequests.Commits", &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	if _, err := c.MergeRequests().Commits(ctx, ref, 1); !errors.Is(err, gitlab.ErrServer) {
		t.Fatalf("scripted error: %v", err)
	}
	if n := c.CallCount("MergeRequests.Commits"); n != 4 {
		t.Errorf("calls = %d", n)
	}
}
