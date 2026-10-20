package runpipeline

import (
	"os"
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

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

type host struct{}

func (host) Init() tea.Cmd                          { return nil }
func (h host) Update(tea.Msg) (app.Screen, tea.Cmd) { return h, nil }
func (host) View() string                           { return "host" }
func (host) Title() string                          { return "host" }
func (host) KeyMap() common.KeyMap                  { return common.KeyMap{} }
func (host) SetSize(int, int)                       {}

func open(t *testing.T, c *fake.Client, initial, local string) (*uitest.Driver, *app.Model, *Form) {
	t.Helper()
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	root := app.New(env, host{}, app.Options{})
	d := uitest.New(t, root)
	f := New(env, gitlab.ProjectRef{ID: 42, FullPath: path}, initial, local)
	d.Send(app.OpenOverlayMsg{Overlay: f})
	d.Run(f.Init())
	return d, root, f
}

func newFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	c.SetBranches(path, "main", "feat/surge", "feat/slots", "fix/race")
	c.SetTags(path, gitlab.Tag{Name: "v1.14.2"}, gitlab.Tag{Name: "v1.15.0"})
	return c
}

func typeText(d *uitest.Driver, s string) {
	for _, r := range s {
		d.Key(string(r))
	}
}

func TestLocalBranchComesFirst(t *testing.T) {
	d, _, f := open(t, newFake(), "feat/surge", "feat/surge")
	if f.Ref() != "feat/surge" {
		t.Fatalf("ref = %q", f.Ref())
	}
	d.RequireView("Run pipeline", "Ref", "› feat/surge  (current in git)", "will be created: ref=feat/surge, variables=0")
	if len(f.cands) == 0 || f.cands[0].kind != kindCurrent {
		t.Fatalf("cands = %+v", f.cands)
	}
}

func TestDebounceOneSearchPerPause(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "main", "")
	for range 10 {
		d.Key("backspace")
	}
	c.ResetCalls()
	typeText(d, "feat/s")
	if c.CallCount("Projects.Branches") != 0 {
		t.Fatal("typing alone must not search")
	}
	// Ticks of the earlier keystrokes are stale; only the last one fires.
	d.Send(searchTickMsg{tok: f.inputTok - 1})
	if c.CallCount("Projects.Branches") != 0 {
		t.Fatal("a stale tick must be ignored")
	}
	d.Send(f.DebounceMsg())
	if c.CallCount("Projects.Branches") != 1 || c.CallCount("Tags.List") != 1 {
		t.Fatalf("branches=%d tags=%d", c.CallCount("Projects.Branches"), c.CallCount("Tags.List"))
	}
	for _, call := range c.Calls() {
		if call.Method == "Projects.Branches" && call.Args[1] != "feat/s" {
			t.Fatalf("search term = %v", call.Args)
		}
	}
	d.RequireView("feat/surge", "feat/slots")
	d.RequireNoView("fix/race")
}

func TestPickCandidateWithArrowsAndEnter(t *testing.T) {
	d, _, f := open(t, newFake(), "", "")
	d.Key("down", "down", "enter")
	if f.Ref() != f.cands[2].name {
		t.Fatalf("ref = %q, cands = %+v", f.Ref(), f.cands)
	}
}

func TestHintsPickValueAndCreate(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", gitlab.CIConfigVariable{Key: "DEPLOY_ENV", Value: "staging", ValueOptions: []string{"staging", "prod-canary"}}, gitlab.CIConfigVariable{Key: "SKIP_E2E"})
	d, root, f := open(t, c, "main", "")
	d.RequireView("From .gitlab-ci.yml:", "1 DEPLOY_ENV [staging, prod-canary]", "2 SKIP_E2E")
	d.Key("tab", "1")
	d.RequireView("Value for DEPLOY_ENV", "› staging")
	d.Key("enter")
	if v := f.Vars(); len(v) != 1 || v[0].Key != "DEPLOY_ENV" || v[0].Value != "staging" {
		t.Fatalf("vars = %+v", v)
	}
	d.RequireView("variables=1")
	d.Key("ctrl+s")
	var call fake.Call
	for _, cl := range c.Calls() {
		if cl.Method == "Pipelines.Create" {
			call = cl
		}
	}
	vars, _ := call.Args[2].([]gitlab.CIVariable)
	if call.Args[1] != "main" || len(vars) != 1 || vars[0].Key != "DEPLOY_ENV" || vars[0].Value != "staging" {
		t.Fatalf("create call = %+v", call)
	}
	if root.Depth() != 2 {
		t.Fatalf("the new pipeline screen must be on top, depth = %d", root.Depth())
	}
	d.RequireView(path+" › Pipelines › #", "pipeline #")
}

func TestHintWithoutOptionsAddsDefaultValue(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", gitlab.CIConfigVariable{Key: "SKIP_E2E", Value: "false"})
	d, _, f := open(t, c, "main", "")
	d.Key("tab", "1")
	if v := f.Vars(); len(v) != 1 || v[0].Key != "SKIP_E2E" || v[0].Value != "false" {
		t.Fatalf("vars = %+v", v)
	}
}

func TestHintsNotReadyRetriesThenGivesUp(t *testing.T) {
	c := newFake()
	for range 4 {
		c.FailNext("Projects.CIConfigVariables", gitlab.ErrNotReady)
	}
	d, _, f := open(t, c, "main", "")
	d.RequireView("loading hints")
	for attempt := 1; attempt <= 3; attempt++ {
		d.Send(hintsRetryMsg{tok: f.hintTok, attempt: attempt})
	}
	if c.CallCount("Projects.CIConfigVariables") != 4 {
		t.Fatalf("calls = %d, want 1 + retries after 1s, 2s, 4s", c.CallCount("Projects.CIConfigVariables"))
	}
	d.RequireView("hints unavailable")
	// The form still works.
	d.Key("ctrl+s")
	if c.CallCount("Pipelines.Create") != 1 {
		t.Fatal("the form must stay usable without hints")
	}
}

func TestHintsRecoverAfterRetry(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", gitlab.CIConfigVariable{Key: "A"})
	c.FailNext("Projects.CIConfigVariables", gitlab.ErrNotReady)
	d, _, f := open(t, c, "main", "")
	d.Send(hintsRetryMsg{tok: f.hintTok, attempt: 1})
	d.RequireView("1 A")
}

func TestEditVariablesAndRefRequired(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "", "")
	d.Key("ctrl+s")
	d.RequireView("Enter a branch or tag")
	if c.CallCount("Pipelines.Create") != 0 {
		t.Fatal("empty ref must not create")
	}
	typeText(d, "main")
	d.Key("tab", "a")
	typeText(d, "FOO")
	d.Key("enter")
	typeText(d, "bar")
	d.Key("enter", "ctrl+s")
	var vars []gitlab.CIVariable
	for _, cl := range c.Calls() {
		if cl.Method == "Pipelines.Create" {
			vars, _ = cl.Args[2].([]gitlab.CIVariable)
		}
	}
	if len(vars) != 1 || vars[0].Key != "FOO" || vars[0].Value != "bar" || f.Ref() != "main" {
		t.Fatalf("vars = %+v", vars)
	}
}

func TestEscClosesWithoutRunning(t *testing.T) {
	c := newFake()
	d, root, _ := open(t, c, "main", "")
	d.Key("esc")
	d.RequireNoView("Run pipeline")
	if c.CallCount("Pipelines.Create") != 0 || root.Depth() != 1 {
		t.Fatal("esc must not run anything")
	}
}
