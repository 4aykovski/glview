package workspace

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/uitest"
	"github.com/4aykovski/glview/internal/workspace"
)

func TestStartOnProjectsTab(t *testing.T) {
	client := newFake()
	d, _, scr := open(t, client, testScope())
	d.RequireView("~/src · workspace · 5 GitLab projects", "1 Projects 5", "2 MRs", "4 Tags",
		"github.com/egor/dotfiles · not GitLab", "skipped without .git: notes, tmp")
	if scr.Current() != TabProjects {
		t.Fatal("Projects is the default tab")
	}
	if n := client.CallCount("Workspace.ProjectsOverview"); n != 1 {
		t.Fatalf("overview calls = %d", n)
	}
	// Rows by activity, the foreign folder last.
	order := []string{"route-service", "routing-engine", "route-planner-ui", "router-configs", "e2e-routes", "dotfiles"}
	prev := -1
	for _, name := range order {
		i := lineIndex(d, " "+name+" ")
		if i <= prev {
			t.Fatalf("%s is out of order:\n%s", name, d.View())
		}
		prev = i
	}
	row := lineWith(t, d, "platform/route-service")
	for _, want := range []string{"fix/slot-race", "4 ·2", "► manual", "v1.14.2", "1h"} {
		if !strings.Contains(row, want) {
			t.Fatalf("row lacks %q: %q", want, row)
		}
	}
	if client.CallCount("Workspace.MergeRequests") != 0 || client.CallCount("Workspace.Pipelines") != 0 {
		t.Fatal("group tabs load lazily")
	}
}

func TestNullProjectIsGreyNoAccess(t *testing.T) {
	client := newFake()
	scope := testScope()
	rows := append([]workspace.Project{localRow("/src/gone", "gone", "platform/gone", "main")}, scope.Rows...)
	d, _, _ := open(t, client, workspace.NewScope(source, rows))
	row := lineWith(t, d, "platform/gone")
	if !strings.Contains(row, "no access") {
		t.Fatalf("row = %q", row)
	}
	d.RequireView("platform/route-service", "► manual")
}

func TestPreviewLocalBranchMR(t *testing.T) {
	client := newFake()
	d, _, _ := open(t, client, testScope())
	d.RequireView("route-service  → platform/route-service  · origin: git@gitlab.example.com:platform/route-service.git",
		"4 open MRs (2 waiting for your review)", "main #91790 ► manual", "on fix/slot-race locally → !479 ✓")
	calls := 0
	for _, c := range client.Calls() {
		if c.Method == "MergeRequests.List" {
			calls++
			f := c.Args[1].(gitlab.MRFilter)
			if c.Args[0] != "platform/route-service" || f.SourceBranch != "fix/slot-race" {
				t.Fatalf("local MR request = %+v", c.Args)
			}
		}
	}
	if calls != 1 {
		t.Fatalf("only the selected row asks for its MR, calls = %d", calls)
	}
	d.Key("j") // routing-engine on main: no open MR from it
	d.RequireView("on main locally · no open MR")
	if client.CallCount("MergeRequests.List") != 2 {
		t.Fatal("the next row asks once")
	}
	d.Key("k")
	if client.CallCount("MergeRequests.List") != 2 {
		t.Fatal("a known row is not asked again")
	}
}

func TestEnterOpensProjectAndEscReturns(t *testing.T) {
	client := newFake()
	env := newEnv(client)
	statePath := filepath.Join(t.TempDir(), "state.json")
	env.Catalog = app.NewCatalog(nil, statePath)
	scr := New(env, source, testScope())
	root := app.New(env, scr, app.Options{})
	d := uitest.New(t, root)
	d.Key("j", "j") // route-planner-ui
	d.Key("enter")
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	p, ok := root.Top().(*project.Project)
	if !ok || p.Path() != "frontend/route-planner-ui" || p.Current() != project.TabMRs {
		t.Fatalf("top = %T", root.Top())
	}
	d.RequireView("~/src › frontend/route-planner-ui · gitlab.example.com", "feat/map-layers", "Map layers: courier heatmap toggle")
	if st := cache.LoadState(statePath); len(st.Recent) != 1 || st.Recent[0] != "frontend/route-planner-ui" {
		t.Fatalf("state = %+v", st)
	}
	d.Key("esc")
	if root.Depth() != 1 {
		t.Fatal("esc returns to the workspace")
	}
	if !strings.HasPrefix(lineWith(t, d, "route-planner-ui"), "›") {
		t.Fatalf("cursor kept:\n%s", d.View())
	}
	d.Key("G", "enter") // the foreign folder opens nothing
	if root.Depth() != 1 {
		t.Fatal("a non-GitLab row opens nothing")
	}
}

func TestTextFilter(t *testing.T) {
	d, _, _ := open(t, newFake(), testScope())
	d.Key("/", "p", "l", "a", "t", "enter")
	d.RequireView("platform/route-service", "platform/routing-engine", "2 of 6")
	d.RequireNoView("qa/e2e-routes", "egor/dotfiles")
	d.Key("backspace")
	d.RequireView("qa/e2e-routes")
}

func TestEmptyWorkspace(t *testing.T) {
	client := newFake()
	rows := []workspace.Project{
		foreignRow("dotfiles", "github.com", "egor/dotfiles"),
		foreignRow("notes-site", "github.com", "egor/notes-site"),
		noGit("tmp"),
	}
	d, _, _ := open(t, client, workspace.NewScope(source, rows))
	d.RequireView("No GitLab projects in ~/src", "scanned 3 folders, of them 2 with another remote, 1 without .git",
		"p pick a project", "github.com/egor/dotfiles · not GitLab", "github.com/egor/notes-site")
	if client.CallCount("Workspace.ProjectsOverview") != 0 {
		t.Fatal("nothing to ask GitLab about")
	}
	d.Key("ctrl+l")
	d.RequireView("В ~/src нет GitLab-проектов", "просмотрено 3 папки, из них 2 с чужим remote, 1 без .git")
}

func TestSnapshotDrawsAtOnceAndKnownRootRefs(t *testing.T) {
	client := newFake()
	disk := &cache.Disk{Dir: t.TempDir()}

	env := newEnv(client)
	env.Disk = disk
	d := uitest.New(t, app.New(env, New(env, source, testScope()), app.Options{}))
	d.RequireView("v1.14.2")
	if _, _, ok := disk.LoadWorkspace(env.Host, source); !ok {
		t.Fatal("the overview is saved after a complete run")
	}

	// Second start: the snapshot is drawn while the overview is slow.
	client2 := newFake()
	client2.SetDelay("Workspace.ProjectsOverview", time.Second)
	env2 := newEnv(client2)
	env2.Disk = disk
	d2 := uitest.New(t, app.New(env2, New(env2, source, testScope()), app.Options{}))
	row := lineWith(t, d2, "platform/route-service")
	if !strings.Contains(row, "► manual") || !strings.Contains(row, "v1.14.2") {
		t.Fatalf("snapshot row = %q", row)
	}
	calls := client2.Calls()
	var opts gitlab.OverviewOptions
	for _, c := range calls {
		if c.Method == "Workspace.ProjectsOverview" {
			opts = c.Args[1].(gitlab.OverviewOptions)
		}
	}
	if opts.RootRefs["platform/route-service"] != "main" || len(opts.RootRefs) != 5 {
		t.Fatalf("root refs = %v", opts.RootRefs)
	}
}

func TestRefreshForcesTagsAndPolls(t *testing.T) {
	client := newFake()
	d, _, scr := open(t, client, testScope())
	d.Key("r")
	var last gitlab.OverviewOptions
	for _, c := range client.Calls() {
		if c.Method == "Workspace.ProjectsOverview" {
			last = c.Args[1].(gitlab.OverviewOptions)
		}
	}
	if !last.ForceTags || client.CallCount("Workspace.ProjectsOverview") != 2 {
		t.Fatalf("r: %+v", last)
	}
	d.Send(scr.PollMsg())
	if client.CallCount("Workspace.ProjectsOverview") != 3 {
		t.Fatal("a poll tick reloads the overview")
	}
}

func TestGoldenProjects(t *testing.T) {
	d, _, _ := open(t, newFake(), testScope())
	d.RequireGoldenSizes("projects")
}
