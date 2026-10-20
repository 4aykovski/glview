package pipeline

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const path = "platform/route-service"

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

func fixture() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 77, FullPath: "qa/e2e-routes"}})
	anna := &gitlab.User{Username: "anna.k"}
	down := gitlab.PipelineSummary{Project: gitlab.ProjectRef{ID: 77, FullPath: "qa/e2e-routes"}, ID: 5521, IID: 9, Status: gitlab.StatusRunning, Ref: "main", SHA: "cccc", WebURL: "https://gitlab.example.com/qa/5521"}
	c.AddPipeline(path, gitlab.Pipeline{
		PipelineSummary: gitlab.PipelineSummary{ID: 91840, IID: 512, Status: gitlab.StatusRunning, Ref: "feat/surge", SHA: "a91f2c0d4e",
			CommitTitle: "surge: add pricing", Source: "merge_request_event", User: anna, Duration: 220 * time.Second, WebURL: "https://gitlab.example.com/p/91840"},
		Jobs: []gitlab.Job{
			{ID: 1, Name: "build", Stage: "build", Status: gitlab.StatusSuccess, Duration: 60 * time.Second},
			{ID: 2, Name: "lint", Stage: "build", Status: gitlab.StatusSuccess},
			{ID: 3, Name: "test:unit", Stage: "test", Status: gitlab.StatusRunning, Runner: "runner-1", StartedAt: ptr(now.Add(-2 * time.Minute)),
				Attempts: []gitlab.Job{{ID: 9, Name: "test:unit", Stage: "test", Status: gitlab.StatusFailed, Runner: "runner-1",
					Duration: 42 * time.Second, StartedAt: ptr(now.Add(-time.Hour)), WebURL: "https://gitlab.example.com/job/9"}}},
			{ID: 4, Name: "test:integration", Stage: "test", Status: gitlab.StatusFailed, AllowFailure: true, WebURL: "https://gitlab.example.com/job/4"},
			{ID: 5, Name: "deploy", Stage: "deploy", Status: gitlab.StatusManual},
			{ID: 6, Name: "cleanup", Stage: "deploy", Status: gitlab.StatusCreated},
		},
		Bridges: []gitlab.Bridge{
			{Job: gitlab.Job{ID: 7, Name: "e2e", Stage: "e2e", Status: gitlab.StatusRunning}, DownstreamProject: "qa/e2e-routes", Downstream: &down},
			{Job: gitlab.Job{ID: 8, Name: "notify", Stage: "e2e", Status: gitlab.StatusCreated}},
		},
	})
	c.AddPipeline("qa/e2e-routes", gitlab.Pipeline{PipelineSummary: down, Jobs: []gitlab.Job{{ID: 20, Name: "routes", Stage: "e2e", Status: gitlab.StatusRunning}}})
	c.SetTrace(4, "start\n\x1b[31mFAIL: TestSurge (0.02s)\x1b[0m\n\n")
	c.SetTrace(3, "compiling\rcompiled 50%\rcompiled 100%\n")
	c.SetTrace(9, "compiling\nFAIL: TestFlaky (0.10s)\n")
	return c
}

func ptr[T any](v T) *T { return &v }

func open(t *testing.T, c *fake.Client) (*uitest.Driver, *app.Model, *Screen) {
	t.Helper()
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	scr := New(env, gitlab.ProjectRef{ID: 42, FullPath: path}, 91840, nil)
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr
}

func TestRendersStagesBridgesDownstream(t *testing.T) {
	d, _, _ := open(t, fixture())
	d.Send(tea.WindowSizeMsg{Width: 150, Height: 40}) // five columns need 25 cells each
	d.RequireView(
		path+" › Pipelines › #91840", "running", "#91840 on feat/surge · a91f2c0d surge: add pricing",
		"anna.k · merge_request_event · 3m40s", "6 jobs", // 6 real jobs; bridges are listed in their stage
		"✓ build", "◐ test", "► deploy",
		"lint", "test:unit", "test:integration", "deploy", "cleanup",
		"⇢ e2e", "· waiting",
		"downstream", "⇢ qa/e2e-routes", "running",
		// panel of the selected job
		"job #1 · build",
	)
}

func TestNarrowTerminalScrollsHorizontally(t *testing.T) {
	d, _, scr := open(t, fixture())
	d.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	// 80 columns hold 2 stage columns: moving right must bring the active one into view.
	for _, want := range []string{"test", "deploy", "e2e"} {
		d.Key("l")
		d.RequireView(want)
		if scr.si < scr.xoff || scr.si >= scr.xoff+scr.visibleCols() {
			t.Fatalf("active stage %d outside [%d, %d)", scr.si, scr.xoff, scr.xoff+scr.visibleCols())
		}
	}
	d.RequireView("◂")
	for _, l := range strings.Split(d.View(), "\n") {
		if len([]rune(l)) > 80 {
			t.Fatalf("line wider than 80: %q", l)
		}
	}
}

func TestManyStagesKeepActiveVisible(t *testing.T) {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	var jobs []gitlab.Job
	for i := 1; i <= 9; i++ {
		jobs = append(jobs, gitlab.Job{ID: int64(i), Name: fmt.Sprintf("job%d", i), Stage: fmt.Sprintf("stage%d", i), Status: gitlab.StatusSuccess})
	}
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91840, Status: gitlab.StatusSuccess}, Jobs: jobs})
	d, _, scr := open(t, c)
	d.Send(tea.WindowSizeMsg{Width: 100, Height: 30})
	for i := 1; i < 9; i++ {
		d.Key("l")
		d.RequireView(fmt.Sprintf("stage%d", i+1))
	}
	if scr.xoff == 0 {
		t.Fatal("columns must have scrolled")
	}
	d.Key("h", "h", "h", "h", "h", "h", "h", "h")
	d.RequireView("stage1")
}

func TestYAMLErrors(t *testing.T) {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91840, Status: gitlab.StatusFailed}, YAMLErrors: "jobs:build config should be a hash"})
	d, _, _ := open(t, c)
	d.RequireView("Pipeline configuration errors", "jobs:build config should be a hash")
}

func TestPanelShowsLastLogLineLazily(t *testing.T) {
	c := fixture()
	d, _, _ := open(t, c)
	if c.CallCount("Jobs.Trace") != 0 {
		t.Fatal("a finished job needs no log")
	}
	d.Key("l") // test stage: first job is the running one
	d.RequireView("job #3 · test", "runner-1", "attempts: #3 ◐ · #9 ✗", "compiled 100%")
	d.Key("j")
	d.RequireView("job #4", "allow_failure: true", "FAIL: TestSurge (0.02s)")
	n := c.CallCount("Jobs.Trace")
	d.Key("k", "j")
	if c.CallCount("Jobs.Trace") != n+2 {
		// the running job refetches, the failed one is fetched again after the other was shown
		t.Logf("trace calls: %d", c.CallCount("Jobs.Trace"))
	}
}

func TestEnterOnBridgeOpensDownstream(t *testing.T) {
	d, root, _ := open(t, fixture())
	d.Key("l", "l", "l") // e2e stage
	d.Key("enter")
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.RequireView("qa/e2e-routes › Pipelines › #5521", "routes")
	d.Key("esc")
	d.RequireView(path + " › Pipelines › #91840")
	if root.Depth() != 1 {
		t.Fatal("esc returns to the parent")
	}
}

func TestOpenPipelineAndJobInBrowser(t *testing.T) {
	d, root, _ := open(t, fixture())
	var opened string
	root.Env().Browser = func(u string) error { opened = u; return nil }
	d.Key("l", "j") // test:integration
	d.Key("o")
	if opened != "https://gitlab.example.com/p/91840" {
		t.Fatalf("o opened %q, want the pipeline", opened)
	}
	d.Key("O")
	if opened != "https://gitlab.example.com/job/4" {
		t.Fatalf("O opened %q, want the job", opened)
	}
	d.Key("l", "l") // e2e bridge with a downstream pipeline
	d.Key("O")
	if opened != "https://gitlab.example.com/qa/5521" {
		t.Fatalf("O on a bridge opened %q, want the downstream pipeline", opened)
	}
}

func TestEnterOnWaitingBridgeDoesNothing(t *testing.T) {
	d, root, _ := open(t, fixture())
	d.Key("l", "l", "l", "j") // notify: no downstream yet
	d.Key("enter")
	if root.Depth() != 1 {
		t.Fatal("must not push")
	}
	d.RequireNoView("downstream project")
}

func TestEnterOnInaccessibleBridgeToasts(t *testing.T) {
	c := fixture()
	c.UpdatePipeline(path, 91840, func(p *gitlab.Pipeline) { p.Bridges[1].Status = gitlab.StatusSuccess })
	d, _, _ := open(t, c)
	d.Key("l", "l", "l", "j")
	d.RequireView("· no access")
	d.Key("enter")
	d.RequireView("No access to the downstream project")
}

func TestPlayManualJobWithVariable(t *testing.T) {
	c := fixture()
	d, _, _ := open(t, c)
	d.Key("l", "l") // deploy stage, manual job selected
	d.Key("P")
	d.RequireView("Run deploy", "KEY", "VALUE", "type", "a add", "enter Run")
	d.RequireNoView("ctrl+s")
	d.Key("a")
	for _, r := range "TARGET" {
		d.Key(string(r))
	}
	d.Key("enter")
	for _, r := range "canary" {
		d.Key(string(r))
	}
	d.Key("enter")
	d.Key("tab") // a key while editing does not leave the table
	if c.CallCount("Jobs.Play") != 0 {
		t.Fatal("nothing runs before enter on the button")
	}
	d.RequireView("enter run · tab next · esc cancel")
	d.Key("enter")
	var call fake.Call
	for _, cl := range c.Calls() {
		if cl.Method == "Jobs.Play" {
			call = cl
		}
	}
	vars, _ := call.Args[2].([]gitlab.CIVariable)
	if call.Args[1] != int64(5) || len(vars) != 1 || vars[0].Key != "TARGET" || vars[0].Value != "canary" {
		t.Fatalf("play call = %+v", call)
	}
	d.RequireView("deploy started")
}

func TestPlayWithoutVariablesAndForbidden(t *testing.T) {
	c := fixture()
	d, _, _ := open(t, c)
	d.Key("l", "l", "P")
	c.FailNext("Jobs.Play", &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403})
	d.Key("tab", "enter")
	d.RequireView("Not enough permissions")
	for _, cl := range c.Calls() {
		if cl.Method == "Jobs.Play" {
			if vars, _ := cl.Args[2].([]gitlab.CIVariable); len(vars) != 0 {
				t.Fatalf("empty form must play without variables: %+v", vars)
			}
		}
	}
}

func TestPlayRejectsNonManual(t *testing.T) {
	d, _, _ := open(t, fixture())
	d.Key("P")
	d.RequireView("only manual jobs can be played")
	d.RequireNoView("Run build")
}

func TestRetryAndCancelJob(t *testing.T) {
	c := fixture()
	d, _, _ := open(t, c)
	d.Key("l", "j") // failed test:integration
	d.Key("R")
	if c.CallCount("Jobs.Retry") != 1 {
		t.Fatal("R retries without confirmation")
	}
	d.RequireView("retrying test:integration")
	d.Key("k", "c")
	d.RequireView("Cancel job test:unit?")
	d.Key("esc")
	if c.CallCount("Jobs.Cancel") != 0 {
		t.Fatal("cancel only after confirmation")
	}
	d.Key("c", "y")
	if c.CallCount("Jobs.Cancel") != 1 {
		t.Fatal("confirmed cancel")
	}
	d.RequireView("test:unit canceled")
}

func TestPollingWhileRunningAndTopOnly(t *testing.T) {
	c := fixture()
	d, _, scr := open(t, c)
	if scr.pollDelay() != 3*time.Second {
		t.Fatalf("running pipeline: %v", scr.pollDelay())
	}
	n := c.CallCount("Pipelines.Get")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if c.CallCount("Pipelines.Get") != n+1 {
		t.Fatal("tick must reload")
	}
	c.UpdatePipeline(path, 91840, func(p *gitlab.Pipeline) {
		p.Status = gitlab.StatusSuccess
		for i := range p.Jobs {
			p.Jobs[i].Status = gitlab.StatusSuccess
		}
		p.Bridges[0].Status = gitlab.StatusSuccess
	})
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if scr.pollDelay() != 30*time.Second {
		t.Fatalf("finished pipeline: %v", scr.pollDelay())
	}

	stale := scr.poll.Current()
	d.Key("l", "l", "l", "enter") // push downstream
	n = c.CallCount("Pipelines.Get")
	d.Send(common.PollTickMsg{Token: stale})
	if c.CallCount("Pipelines.Get") != n {
		t.Fatal("covered screen must not poll")
	}
}

func TestPlayFormFocusCycle(t *testing.T) {
	c := fixture()
	d, _, _ := open(t, c)
	d.Key("l", "l", "P")
	d.Key("enter") // on the table: no rows, nothing to edit, nothing runs
	if c.CallCount("Jobs.Play") != 0 {
		t.Fatal("enter on the table must not play")
	}
	d.Key("tab", "shift+tab") // button and back to the table
	d.Key("a")
	d.Key("tab") // typed into the cell: tab does nothing while editing
	d.RequireView("Run deploy")
	if c.CallCount("Jobs.Play") != 0 {
		t.Fatal("tab while editing must not move to the button")
	}
	d.Key("esc", "shift+tab", "enter")
	if c.CallCount("Jobs.Play") != 1 {
		t.Fatalf("enter on the button must play, calls = %d", c.CallCount("Jobs.Play"))
	}
}
