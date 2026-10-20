package project

import (
	"context"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

func addPipelines(c *fake.Client) {
	anna := &gitlab.User{Username: "anna.k"}
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{
		ID: 101, IID: 11, Status: gitlab.StatusFailed, Ref: "main", SHA: "0b7d3e9aaa", Source: "push", User: anna,
		CommitTitle: "Merge branch fix", Duration: 340 * time.Second, CreatedAt: now.Add(-time.Hour)},
		Jobs: []gitlab.Job{
			{ID: 1, Name: "build", Stage: "build", Status: gitlab.StatusSuccess},
			{ID: 2, Name: "test:integration", Stage: "test", Status: gitlab.StatusFailed},
			{ID: 3, Name: "deploy", Stage: "deploy", Status: gitlab.StatusSkipped},
		}})
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{
		ID: 102, IID: 12, Status: gitlab.StatusRunning, Ref: "feat/surge", SHA: "a91f2c0d4e", Source: "merge_request_event", User: anna,
		CommitTitle: "surge: add pricing", CreatedAt: now.Add(-3 * time.Minute),
		Stages: []gitlab.StageSummary{
			{Name: "build", Status: gitlab.StatusSuccess, Done: 1, Total: 1},
			{Name: "test", Status: gitlab.StatusRunning, Done: 2, Total: 3},
			{Name: "deploy", Status: gitlab.StatusCreated, Total: 1}}},
		Jobs: []gitlab.Job{{ID: 4, Name: "test:unit", Stage: "test", Status: gitlab.StatusRunning}}})
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{
		ID: 100, IID: 10, Status: gitlab.StatusSuccess, Ref: "main", SHA: "ffff", Source: "schedule", CreatedAt: now.Add(-5 * time.Hour)}})
}

func openPipelines(t *testing.T, client *fake.Client) (*uitest.Driver, *Project, *pipelinesTab) {
	t.Helper()
	d, _, scr := open(t, client, nil)
	d.Key("2")
	return d, scr, scr.tabs[1].(*pipelinesTab)
}

func TestPipelinesListAndStagePreview(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	d, _, tab := openPipelines(t, client)
	if len(tab.Items()) != 3 || tab.Items()[0].ID != 102 {
		t.Fatalf("newest first: %+v", tab.Items())
	}
	d.RequireView("Status", "ID", "Ref", "Source", "Commit", "User", "Dur", "Ago",
		"#102", "feat/surge", "merge_request_event", "a91f2c0 surge: add pricing", "anna.k", "◐ running",
		"✗ failed", "3 of 3")
	// Preview of the running pipeline: stage chain with counts.
	d.RequireView("✓ build → ◐ test 2/3 → ○ deploy")
	d.Key("j")
	d.RequireView("✓ build")
	if client.CallCount("Pipelines.Stages") < 2 {
		t.Fatalf("stages must be requested per selection: %d", client.CallCount("Pipelines.Stages"))
	}
}

func TestPipelinesFilterSentToClient(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	d, _, tab := openPipelines(t, client)
	d.Key("/")
	for _, r := range "ref:main status:failed" {
		d.Key(string(r))
	}
	d.Key("enter")
	var last fake.Call
	for _, c := range client.Calls() {
		if c.Method == "Pipelines.List" {
			last = c
		}
	}
	f := last.Args[1].(gitlab.PipelineFilter)
	if f.Ref != "main" || f.Status != "failed" {
		t.Fatalf("filter = %+v", f)
	}
	if len(tab.Items()) != 1 || tab.Items()[0].ID != 101 {
		t.Fatalf("items = %+v", tab.Items())
	}
	d.RequireView("ref:main", "status:failed")
	// "m" sets user to the current user.
	d.Key("m")
	if tab.filter.Username != "egor" {
		t.Fatalf("user = %q", tab.filter.Username)
	}
	d.RequireView("No pipelines, where")
}

func TestPipelinesPollingAdapts(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	d, _, tab := openPipelines(t, client)
	if got := tab.env().PollInterval(tab.hasActive()); got != 3*time.Second {
		t.Fatalf("a running pipeline on screen: %v", got)
	}
	n := client.CallCount("Pipelines.List")
	d.Send(common.PollTickMsg{Token: tab.poll.Current()})
	if client.CallCount("Pipelines.List") != n+1 {
		t.Fatal("tick must refresh")
	}
	client.UpdatePipeline(path, 102, func(p *gitlab.Pipeline) { p.Status = gitlab.StatusSuccess })
	d.Send(common.PollTickMsg{Token: tab.poll.Current()})
	if got := tab.env().PollInterval(tab.hasActive()); got != 30*time.Second {
		t.Fatalf("nothing active any more: %v", got)
	}
}

func TestPipelinesRetryAndCancelNeedConfirmation(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	d, _, _ := openPipelines(t, client)
	d.Key("R")
	d.RequireView("Retry failed jobs of pipeline #102?")
	d.Key("esc")
	if client.CallCount("Pipelines.Retry") != 0 {
		t.Fatal("cancel must not retry")
	}
	d.Key("R", "y")
	if client.CallCount("Pipelines.Retry") != 1 {
		t.Fatal("confirmed retry")
	}
	d.RequireView("retrying pipeline #102")

	d.Key("c")
	d.RequireView("Cancel pipeline #102?")
	d.Key("n") // answer "no"
	if client.CallCount("Pipelines.Cancel") != 0 {
		t.Fatal("no must not cancel")
	}
	d.Key("c", "y")
	if client.CallCount("Pipelines.Cancel") != 1 {
		t.Fatal("confirmed cancel")
	}
	d.RequireView("pipeline #102 canceled", "canceled")
}

func TestPipelinesLogKey(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	d, _, _ := openPipelines(t, client)
	d.Key("j") // #101 has a failed job
	d.Key("l")
	d.RequireView("#101 › test:integration")
	d.Key("esc", "j") // back to the list; #100 has no jobs
	d.Key("l")
	d.RequireView("no failed or running jobs")
}

func TestPipelinesLogKeyBringsAttempts(t *testing.T) {
	client := newFake(1)
	client.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{
		ID: 103, IID: 13, Status: gitlab.StatusFailed, Ref: "main", SHA: "0c1d2e3f4a", Source: "push", CreatedAt: now.Add(-time.Minute)},
		Jobs: []gitlab.Job{{ID: 12, Name: "test:unit", Stage: "test", Status: gitlab.StatusFailed,
			Attempts: []gitlab.Job{{ID: 11, Name: "test:unit", Stage: "test", Status: gitlab.StatusFailed}}}}})
	client.SetTrace(12, "second try\n")
	client.SetTrace(11, "first try\n")
	d, _, _ := openPipelines(t, client)
	n := client.CallCount("Pipelines.Get")
	d.Key("l")
	d.RequireView("#103 › test:unit", "attempt 2/2", "second try")
	if got := client.CallCount("Pipelines.Get"); got != n+1 {
		t.Fatalf("Pipelines.Get calls = %d, want one", got-n)
	}
	d.Key("[")
	d.RequireView("attempt 1/2", "first try")
	if got := client.CallCount("Pipelines.Get"); got != n+1 {
		t.Fatal("switching attempts needs no pipeline request")
	}
}

func TestPipelinesLogKeyReadsPipelineAnew(t *testing.T) {
	client := newFake(1)
	summary := gitlab.PipelineSummary{ID: 103, IID: 13, Status: gitlab.StatusFailed, Ref: "main", SHA: "0c1d2e3f4a", Source: "push", CreatedAt: now.Add(-time.Minute)}
	earlier := gitlab.Job{ID: 11, Name: "test:unit", Stage: "test", Status: gitlab.StatusFailed}
	client.AddPipeline(path, gitlab.Pipeline{PipelineSummary: summary,
		Jobs: []gitlab.Job{{ID: 12, Name: "test:unit", Stage: "test", Status: gitlab.StatusFailed, Attempts: []gitlab.Job{earlier}}}})
	client.SetTrace(12, "second try\n")
	client.SetTrace(11, "first try\n")
	d, scr, tab := openPipelines(t, client)
	// The Pipeline screen cached the pipeline before the job was retried.
	env := scr.sh.env
	env.Cache = cache.New()
	key := "pipe:" + tab.sh.ref.Key() + ":103"
	env.Cache.Set(key, gitlab.Pipeline{PipelineSummary: summary, Jobs: []gitlab.Job{earlier}})
	n := client.CallCount("Pipelines.Get")
	d.Key("l")
	if got := client.CallCount("Pipelines.Get"); got != n+1 {
		t.Fatalf("Pipelines.Get calls = %d, want a fresh one", got-n)
	}
	d.RequireView("#103 › test:unit", "attempt 2/2", "second try")
	pl, _, _ := cache.Get(context.Background(), env.Cache, key, time.Hour, func(context.Context) (gitlab.Pipeline, error) {
		return gitlab.Pipeline{}, nil
	})
	if len(pl.Jobs) != 1 || pl.Jobs[0].ID != 12 {
		t.Fatalf("the fresh final pipeline is cached: %+v", pl.Jobs)
	}
}

func TestPickLogJob(t *testing.T) {
	jobs := []gitlab.Job{{ID: 1, Status: gitlab.StatusRunning}, {ID: 2, Status: gitlab.StatusFailed}, {ID: 3, Status: gitlab.StatusFailed}}
	if j := pickLogJob(jobs); j == nil || j.ID != 2 {
		t.Fatalf("failed wins over running, first failed: %+v", j)
	}
	if j := pickLogJob(jobs[:1]); j == nil || j.ID != 1 {
		t.Fatal("running when nothing failed")
	}
	if pickLogJob([]gitlab.Job{{Status: gitlab.StatusSuccess}}) != nil {
		t.Fatal("nothing to show")
	}
}

func TestNewPipelineFormFromList(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	client.SetBranches(path, "main", "fix/slot-race")
	repo, _ := tempRepo(t)
	repo.Branch = "fix/slot-race"
	d, _, _ := open(t, client, repo)
	d.Key("2", "n")
	d.RequireView("Run pipeline", "fix/slot-race", "(current in git)")
	d.Key("esc")
	d.RequireNoView("Run pipeline")
}
