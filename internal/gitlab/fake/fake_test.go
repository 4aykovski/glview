package fake

import (
	"context"
	"errors"
	"slices"
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
		{SHA: "b2c3d4e5f6", ShortSHA: "b2c3d4e5", Title: "slot: fix race", AuthorName: "Anna", WebURL: "https://gitlab.example.com/a/b/-/commit/b2c3d4e5f6"},
		{SHA: "a1b2c3d4e5", ShortSHA: "a1b2c3d4", Title: "slot: add test", AuthorName: "Egor"},
	}
	c.SetMRCommits("a/b", 1, commits)
	got, err := c.MergeRequests().Commits(ctx, ref, 1)
	if err != nil || len(got) != 2 || got[0].ShortSHA != "b2c3d4e5" || got[0].WebURL != "https://gitlab.example.com/a/b/-/commit/b2c3d4e5f6" {
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

func TestMRLinksOnlyInDetails(t *testing.T) {
	c := New()
	c.AddMR("platform/route-service", gitlab.MergeRequest{
		IID: 479, Title: "ABC-123: fix", State: gitlab.MRStateOpened,
		TitleLinks: []gitlab.Link{{Text: "ABC-123", URL: "https://tracker.example.com/browse/ABC-123"}},
	})
	p := gitlab.ProjectRef{FullPath: "platform/route-service"}
	page, err := c.MergeRequests().List(context.Background(), p, gitlab.MRFilter{State: gitlab.MRStateAll}, "", 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].TitleLinks != nil {
		t.Fatalf("list = %+v %v", page.Items, err)
	}
	mr, err := c.MergeRequests().Get(context.Background(), p, 479)
	if err != nil || len(mr.TitleLinks) != 1 || c.CallCount("MergeRequests.Get") != 1 {
		t.Fatalf("get = %+v %v", mr, err)
	}
}

func TestFakeTags(t *testing.T) {
	ctx := context.Background()
	c := New()
	p := gitlab.ProjectRef{FullPath: "platform/route-service"}
	c.AddProject(gitlab.Project{ProjectRef: p, DefaultBranch: "main"})
	c.SetTags(p.FullPath,
		gitlab.Tag{Name: "v1.14.2", Protected: true, Commit: gitlab.CommitSummary{ShortSHA: "9fa2c51"}},
		gitlab.Tag{Name: "v1.14.1"})
	c.AddPipeline(p.FullPath, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91761, Ref: "v1.14.2"}})
	c.AddPipeline(p.FullPath, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91790, Ref: "main"}})

	pipes, err := c.Tags().Pipelines(ctx, p, 100)
	if err != nil || len(pipes) != 1 || pipes[0].ID != 91761 {
		t.Fatalf("pipes = %+v, %v", pipes, err)
	}
	// Protected tags answer 403, duplicates 400, new tags come first.
	if err := c.Tags().Delete(ctx, p, "v1.14.2"); !errors.Is(err, gitlab.ErrForbidden) {
		t.Fatalf("delete protected: %v", err)
	}
	if _, err := c.Tags().Create(ctx, p, "v1.14.2", "main", ""); !errors.Is(err, gitlab.ErrNotPossible) {
		t.Fatalf("create duplicate: %v", err)
	}
	if _, err := c.Tags().Create(ctx, p, "v1.14.3", "main", "release: v1.14.3"); err != nil {
		t.Fatal(err)
	}
	page, _ := c.Tags().List(ctx, p, "", "", 20)
	if len(page.Items) != 3 || page.Items[0].Name != "v1.14.3" {
		t.Fatalf("items = %+v", page.Items)
	}
	if err := c.Tags().Delete(ctx, p, "v1.14.1"); err != nil {
		t.Fatal(err)
	}
	page, _ = c.Tags().List(ctx, p, "", "", 20)
	if len(page.Items) != 2 {
		t.Fatalf("items = %+v", page.Items)
	}
	// Workspace batches match the pipelines of the tags.
	for res := range c.Workspace().Tags(ctx, []gitlab.ProjectRef{p}, "", 5) {
		g := res.Groups[0]
		if g.Total != 2 || g.Items[1].Name != "v1.14.2" || g.Items[1].Pipeline == nil || g.Items[1].Pipeline.ID != 91761 {
			t.Fatalf("group = %+v", g)
		}
	}
	// Compare serves what was registered.
	c.SetCompare(p.FullPath, "v1.14.2", "main", gitlab.Comparison{Commits: []gitlab.CommitSummary{{ShortSHA: "aaa"}, {ShortSHA: "bbb"}}})
	cmp, err := c.Tags().Compare(ctx, p, "v1.14.2", "main")
	if err != nil || len(cmp.Commits) != 2 {
		t.Fatalf("cmp = %+v, %v", cmp, err)
	}
}

func TestFakeIssues(t *testing.T) {
	c := New()
	ctx := context.Background()
	p := gitlab.ProjectRef{FullPath: "platform/route-service"}
	c.AddProject(gitlab.Project{ProjectRef: p})
	t0 := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	c.AddIssue(p.FullPath, gitlab.Issue{IID: 1318, Title: "Surge coefficient ignores zone overrides", Assignees: []gitlab.User{c.User}, Labels: []string{"bug"}, Milestone: "2026-Q4", UpdatedAt: t0, MergeRequestsCount: 1})
	c.AddIssue(p.FullPath, gitlab.Issue{IID: 1312, Title: "Duplicate slot assignment under load", UpdatedAt: t0.Add(time.Hour)})
	c.AddIssue(p.FullPath, gitlab.Issue{IID: 1260, Title: "Old report", State: "closed", UpdatedAt: t0.Add(-time.Hour)})
	c.SetRelatedMRs(p.FullPath, 1318, gitlab.RelatedMR{IID: 479, Title: "Fix slot race", State: gitlab.MRStateOpened})

	page, err := c.Issues().List(ctx, p, gitlab.IssueFilter{}, "", 20)
	if err != nil || len(page.Items) != 2 || page.Items[0].IID != 1312 || page.Total != 2 {
		t.Fatalf("opened = %+v, err = %v", page, err)
	}
	for _, tc := range []struct {
		f    gitlab.IssueFilter
		want []int64
	}{
		{gitlab.IssueFilter{State: "all"}, []int64{1312, 1318, 1260}},
		{gitlab.IssueFilter{State: "closed"}, []int64{1260}},
		{gitlab.IssueFilter{Assignee: "@me"}, []int64{1318}},
		{gitlab.IssueFilter{Assignee: "none"}, []int64{1312}},
		{gitlab.IssueFilter{Milestone: "2026-Q4"}, []int64{1318}},
		{gitlab.IssueFilter{Milestone: "none"}, []int64{1312}},
		{gitlab.IssueFilter{Labels: []string{"bug"}}, []int64{1318}},
		{gitlab.IssueFilter{Search: "slot"}, []int64{1312}},
	} {
		page, _ := c.Issues().List(ctx, p, tc.f, "", 20)
		var got []int64
		for _, i := range page.Items {
			got = append(got, i.IID)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%+v: got %v, want %v", tc.f, got, tc.want)
		}
	}

	if _, err := c.Issues().Comment(ctx, p, 1318, "Беру"); err != nil {
		t.Fatal(err)
	}
	notes, _ := c.Issues().Notes(ctx, p, 1318, 50)
	ds, _ := c.Issues().Discussions(ctx, p, 1318)
	i, _ := c.Issues().Get(ctx, p, 1318)
	if len(notes) != 1 || notes[0].Body != "Беру" || notes[0].Author.Username != "egor" || len(ds) != 1 || i.UserNotesCount != 1 {
		t.Fatalf("notes = %+v, ds = %+v, issue = %+v", notes, ds, i)
	}
	if i, err = c.Issues().SetState(ctx, p, 1318, true); err != nil || i.State != "closed" {
		t.Fatalf("issue = %+v, err = %v", i, err)
	}
	if rel, _ := c.Issues().RelatedMRs(ctx, p, 1318); len(rel) != 1 || rel[0].IID != 479 {
		t.Fatalf("rel = %+v", rel)
	}
	c.FailNext("Issues.Comment", &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403})
	if _, err := c.Issues().Comment(ctx, p, 1318, "x"); !errors.Is(err, gitlab.ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	if _, err := c.Issues().Get(ctx, p, 1); !errors.Is(err, gitlab.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if n := c.CallCount("Issues.Comment"); n != 2 {
		t.Fatalf("calls = %d", n)
	}
}

func TestFakeMRCommentAndMilestones(t *testing.T) {
	c := New()
	ctx := context.Background()
	p := gitlab.ProjectRef{FullPath: "platform/route-service"}
	c.AddMR(p.FullPath, gitlab.MergeRequest{IID: 479, Title: "Fix slot race"})
	if _, err := c.MergeRequests().Comment(ctx, p, 479, "LGTM"); err != nil {
		t.Fatal(err)
	}
	ds, _ := c.MergeRequests().Discussions(ctx, p, 479)
	if len(ds) != 1 || ds[0].Notes[0].Body != "LGTM" {
		t.Fatalf("ds = %+v", ds)
	}
	if _, err := c.MergeRequests().Comment(ctx, p, 1, "x"); !errors.Is(err, gitlab.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	c.SetMilestones(p.FullPath, "2026-Q4", "2026-Q3", "backlog")
	ms, _ := c.Projects().Milestones(ctx, p, "2026")
	if !slices.Equal(ms, []gitlab.Milestone{{ID: 1, Title: "2026-Q4"}, {ID: 2, Title: "2026-Q3"}}) {
		t.Fatalf("ms = %v", ms)
	}
}

func TestFakeApproveUpdatesRules(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	ref := gitlab.ProjectRef{FullPath: "a/b"}
	egor, bob := gitlab.User{Username: "egor"}, gitlab.User{Username: "bob"}
	c.AddMR("a/b", gitlab.MergeRequest{
		IID: 1, State: gitlab.MRStateOpened, Reviewers: []gitlab.User{egor},
		ReviewStates: map[string]gitlab.ReviewState{"egor": gitlab.ReviewUnreviewed},
		Approvals: gitlab.Approvals{Required: 1, Left: 1, Rules: []gitlab.ApprovalRule{
			{Name: "Backend", Type: gitlab.ApprovalRegular, Required: 1, Eligible: []gitlab.User{egor, bob}},
			{Name: "Docs", Type: gitlab.ApprovalCodeOwner, Required: 1, Eligible: []gitlab.User{bob}},
		}},
	})
	ctx := context.Background()
	if err := c.MergeRequests().Approve(ctx, ref, 1, "sha"); err != nil {
		t.Fatal(err)
	}
	mr, _ := c.MergeRequests().Get(ctx, ref, 1)
	r := mr.Approvals.Rules
	if !r[0].Approved || len(r[0].ApprovedBy) != 1 || r[1].Approved || len(r[1].ApprovedBy) != 0 {
		t.Fatalf("after approve: %+v", r)
	}
	if mr.ReviewStates["egor"] != gitlab.ReviewApproved {
		t.Fatalf("state = %v", mr.ReviewStates)
	}
	if err := c.MergeRequests().Unapprove(ctx, ref, 1); err != nil {
		t.Fatal(err)
	}
	mr, _ = c.MergeRequests().Get(ctx, ref, 1)
	if mr.Approvals.Rules[0].Approved || len(mr.Approvals.Rules[0].ApprovedBy) != 0 || mr.ReviewStates["egor"] != gitlab.ReviewUnapproved {
		t.Fatalf("after unapprove: %+v %v", mr.Approvals.Rules, mr.ReviewStates)
	}
}

func TestFakeUnapproveWithoutRequiredApprovals(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	ref := gitlab.ProjectRef{FullPath: "a/b"}
	egor := gitlab.User{Username: "egor"}
	c.AddMR("a/b", gitlab.MergeRequest{
		IID: 1, State: gitlab.MRStateOpened, Reviewers: []gitlab.User{egor},
		ReviewStates: map[string]gitlab.ReviewState{"egor": gitlab.ReviewApproved},
		Approvals: gitlab.Approvals{ApprovedBy: []gitlab.User{egor}, Rules: []gitlab.ApprovalRule{
			{Name: "Any", Type: gitlab.ApprovalAnyApprover, Required: 1, Approved: true, ApprovedBy: []gitlab.User{egor}},
		}},
	})
	ctx := context.Background()
	if err := c.MergeRequests().Unapprove(ctx, ref, 1); err != nil {
		t.Fatal(err)
	}
	mr, _ := c.MergeRequests().Get(ctx, ref, 1)
	if mr.Approvals.Left != 0 || mr.Approvals.Rules[0].Approved || len(mr.Approvals.Rules[0].ApprovedBy) != 0 || mr.ReviewStates["egor"] != gitlab.ReviewUnapproved {
		t.Fatalf("after unapprove: %+v %v", mr.Approvals, mr.ReviewStates)
	}
}

func TestFakeListRowsCarryNoDetails(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 1, FullPath: "a/b"}})
	ref := gitlab.ProjectRef{FullPath: "a/b"}
	c.AddMR("a/b", gitlab.MergeRequest{
		IID: 1, State: gitlab.MRStateOpened,
		ReviewStates: map[string]gitlab.ReviewState{"bob": gitlab.ReviewApproved},
		Approvals:    gitlab.Approvals{Rules: []gitlab.ApprovalRule{{Name: "Backend", Required: 1}}},
	})
	ctx := context.Background()
	page, _ := c.MergeRequests().List(ctx, ref, gitlab.MRFilter{State: gitlab.MRStateAll}, "", 20)
	if r := page.Items[0]; r.Approvals.Rules != nil || r.ReviewStates != nil || r.Detailed {
		t.Fatalf("row carries details: %+v", r)
	}
	if mr, _ := c.MergeRequests().Get(ctx, ref, 1); len(mr.Approvals.Rules) != 1 || mr.ReviewStates == nil || !mr.Detailed {
		t.Fatalf("details lack rules: %+v", mr)
	}
}
