package project

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/gitremote"
)

// goldenFake holds the fixed data of the golden snapshots: six open MRs and
// four pipelines of project platform/route-service.
func goldenFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, DefaultBranch: "main"})
	users := []string{"anna.k", "bob", "maria.s"}
	titles := []string{
		"Fix race in courier slot allocator", "Add surge pricing to estimator", "Bump grpc to 1.64",
		"Refactor dispatch queue", "Fix flaky TestSlotRelease", "Document the route-service API",
	}
	for i, title := range titles {
		n := int64(i + 1)
		mr := gitlab.MergeRequest{
			IID: 470 + n, Title: title, State: gitlab.MRStateOpened, Author: gitlab.User{Username: users[i%3]},
			SourceBranch: fmt.Sprintf("feat/topic-%d", n), TargetBranch: "main", SHA: "a91f2c0d4e",
			UpdatedAt: now.Add(-time.Duration(n*7) * time.Minute), MergeStatus: "mergeable",
			WebURL:      fmt.Sprintf("https://gitlab.example.com/%s/-/merge_requests/%d", path, 470+n),
			Description: "Fixes the allocation race.\n\nMore details below.",
			Approvals:   gitlab.Approvals{Required: 2, ApprovedBy: []gitlab.User{{Username: "bob"}}},
			Discussions: gitlab.DiscussionStats{Total: 3, Unresolved: int(n % 3)},
			Diff:        &gitlab.DiffStats{Additions: 48, Deletions: 12, Files: 3},
		}
		switch i {
		case 0:
			mr.Draft = true
			mr.HeadPipeline = &gitlab.PipelineSummary{ID: 91840, Status: gitlab.StatusRunning, Duration: 220 * time.Second}
		case 1:
			mr.MergeStatus, mr.HasConflicts = "conflict", true
			mr.HeadPipeline = &gitlab.PipelineSummary{ID: 91830, Status: gitlab.StatusFailed, Duration: 340 * time.Second}
		case 2:
			mr.Approvals = gitlab.Approvals{ApprovedBy: []gitlab.User{{Username: "bob"}, {Username: "anna.k"}}}
			mr.HeadPipeline = &gitlab.PipelineSummary{ID: 91820, Status: gitlab.StatusSuccess, Duration: 410 * time.Second}
		case 3:
			mr.MergeStatus = "not_approved"
			mr.Approvals = gitlab.Approvals{Required: 2}
		}
		c.AddMR(path, mr)
	}
	anna := &gitlab.User{Username: "anna.k"}
	add := func(id int64, st gitlab.PipelineStatus, ref, sha, title, src string, dur time.Duration, ago time.Duration) {
		c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{
			ID: id, IID: id - 91000, Status: st, Ref: ref, SHA: sha, CommitTitle: title, Source: src, User: anna, Duration: dur,
			CreatedAt: now.Add(-ago), WebURL: fmt.Sprintf("https://gitlab.example.com/%s/-/pipelines/%d", path, id),
			Stages: []gitlab.StageSummary{
				{Name: "build", Status: gitlab.StatusSuccess, Done: 2, Total: 2},
				{Name: "test", Status: st, Done: 2, Total: 3},
				{Name: "deploy", Status: gitlab.StatusCreated, Total: 1},
			}}})
	}
	add(91840, gitlab.StatusRunning, "refs/merge-requests/482/head", "a91f2c0d4e", "surge: add pricing", "merge_request_event", 220*time.Second, 4*time.Minute)
	add(91839, gitlab.StatusFailed, "main", "0b7d3e9aaa", "Merge branch fix/slots", "push", 340*time.Second, 40*time.Minute)
	add(91838, gitlab.StatusSuccess, "main", "ffee001122", "Bump grpc to 1.64", "push", 410*time.Second, 3*time.Hour)
	add(91837, gitlab.StatusSuccess, "release/1.14", "12ab34cd56", "Nightly build", "schedule", 600*time.Second, 20*time.Hour)
	return c
}

func goldenRepo(t *testing.T) *gitremote.Repo {
	r, _ := tempRepo(t)
	r.Branch = "fix/slot-race"
	return r
}

func TestGoldenMRs(t *testing.T) {
	d, _, _ := open(t, goldenFake(), goldenRepo(t))
	d.RequireGoldenSizes("mrs")
}

func TestGoldenPipelines(t *testing.T) {
	d, _, _ := open(t, goldenFake(), goldenRepo(t))
	d.Key("2")
	d.RequireGoldenSizes("pipelines")
}

func TestGoldenMRsMerged(t *testing.T) {
	c := goldenFake()
	for i, st := range []gitlab.MRState{gitlab.MRStateMerged, gitlab.MRStateClosed} {
		n := int64(i + 1)
		c.AddMR(path, gitlab.MergeRequest{
			IID: 460 + n, Title: fmt.Sprintf("Old change %d", n), State: st, MergeStatus: "not_open",
			Author: gitlab.User{Username: "bob"}, SourceBranch: fmt.Sprintf("old/topic-%d", n), TargetBranch: "main",
			UpdatedAt:   now.Add(-time.Duration(n) * 24 * time.Hour),
			Approvals:   gitlab.Approvals{ApprovedBy: []gitlab.User{{Username: "anna.k"}}},
			Description: "Already done.",
		})
	}
	d, _, _ := open(t, c, goldenRepo(t))
	d.Key("/")
	for _, r := range "state:all " {
		d.Key(string(r))
	}
	d.Key("enter")
	for range 6 {
		d.Key("j")
	}
	d.RequireGolden("mrs-merged")
}

// TestGoldenHelp: the key list is taller than 80×24, so it scrolls.
func TestGoldenHelp(t *testing.T) {
	d, _, _ := open(t, goldenFake(), goldenRepo(t))
	d.Key("?")
	d.RequireGoldenSizes("help-mrs")
}

func TestHelpScrollsAtMinimumSize(t *testing.T) {
	d, _, _ := open(t, goldenFake(), goldenRepo(t))
	d.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	d.Key("?")
	d.RequireView("j/k scroll", "1–15/")
	d.RequireNoView("EVERYWHERE")
	first := d.View()
	d.Key("G")
	d.RequireView("EVERYWHERE")
	if d.View() == first {
		t.Fatal("G must scroll to the end")
	}
	d.Key("g", "esc")
	d.RequireNoView("Keys")

	// A terminal tall enough shows everything without scrolling.
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 60})
	d.Key("?")
	d.RequireNoView("j/k scroll")
	d.RequireView("EVERYWHERE")
}
