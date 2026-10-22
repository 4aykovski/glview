package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
	env.Catalog = app.NewCatalog(nil, statePath, func() string { return env.Host })
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
	if st := cache.LoadState(statePath).Recent(env.Host); len(st) != 1 || st[0] != "frontend/route-planner-ui" {
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

func TestTextFilterIgnoresAbsolutePath(t *testing.T) {
	// Every folder lives in /src: "src" must not match them all.
	d, _, _ := open(t, newFake(), testScope())
	d.Key("/", "s", "r", "c", "enter")
	d.RequireNoView("qa/e2e-routes", "platform/route-service", "egor/dotfiles")
	d.Key("backspace", "/", "e", "2", "e", "enter") // the folder name
	d.RequireView("qa/e2e-routes", "1 of 6")
	d.Key("backspace", "/", "r", "o", "u", "t", "e", "r", "-", "c", "enter") // the folder name of infra/router-configs
	d.RequireView("infra/router-configs", "1 of 6")
}

func TestRefreshForcesLocalBranchMR(t *testing.T) {
	client := newFake()
	env := newEnv(client)
	env.Cache = cache.New() // the MR of the branch is cached for 30 s
	d := uitest.New(t, app.New(env, New(env, source, testScope()), app.Options{}))
	d.RequireView("on fix/slot-race locally → !479 ✓")
	if n := client.CallCount("MergeRequests.List"); n != 1 {
		t.Fatalf("calls = %d", n)
	}
	d.Key("r")
	if n := client.CallCount("MergeRequests.List"); n != 2 {
		t.Fatalf("r must ask the MR of the local branch again, calls = %d", n)
	}
	d.RequireView("on fix/slot-race locally → !479 ✓")
}

func TestOpenAndCopyProjectURL(t *testing.T) {
	client := newFake()
	client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 7, FullPath: "platform/gone"}}) // no web URL
	env := newEnv(client)
	var opened string
	env.Browser = func(u string) error { opened = u; return nil }
	rows := append(testScope().Rows, localRow("/src/gone", "gone", "platform/gone", "main"))
	d := uitest.New(t, app.New(env, New(env, source, workspace.NewScope(source, rows)), app.Options{}))
	d.Key("o")
	if opened != "https://gitlab.example.com/platform/route-service" {
		t.Fatalf("o opened %q", opened)
	}
	d.Key("y")
	d.RequireView("copied: https://gitlab.example.com/platform/route-service")
	d.Key("/", "g", "o", "n", "e", "enter", "o")
	if opened != "https://gitlab.example.com/platform/gone" {
		t.Fatalf("without a web URL: %q", opened)
	}
	d.Key("backspace", "2", "o")
	if !strings.HasSuffix(opened, "/-/merge_requests") || strings.Contains(opened, "//-") {
		t.Fatalf("MRs tab o opened %q", opened)
	}
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

// countTab counts the rebuilds of a tab.
type countTab struct {
	tab
	n int
}

func (c *countTab) Rebuild() { c.n++; c.tab.Rebuild() }

func (c *countTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	t, cmd := c.tab.Update(msg)
	c.tab = t
	return c, cmd
}

func TestOverviewReportsRebuildOnlyProjects(t *testing.T) {
	client := newFake()
	d, _, scr := open(t, client, testScope())
	d.RequireView("on fix/slot-race locally → !479 ✓")
	mrs, pipes := &countTab{tab: scr.tabs[TabMRs]}, &countTab{tab: scr.tabs[TabPipelines]}
	scr.tabs[TabMRs], scr.tabs[TabPipelines] = mrs, pipes
	asked := client.CallCount("MergeRequests.List")

	// A running overview reports the latest tag of every project, many times.
	scr.loadOverview(false, false) // its own reports are never listened to
	t.Cleanup(scr.cancel)
	tok := scr.ovGen.Current()
	for i := range 100 {
		p := projects[i%len(projects)].path
		ov := scr.sh.over[p]
		ov.LastTag, ov.TagKnown = fmt.Sprintf("v1.%d.0", i), true
		d.Send(batchMsg[gitlab.ProjectOverview]{tok: tok, res: gitlab.BatchResult[gitlab.ProjectOverview]{
			Groups: []gitlab.ProjectGroup[gitlab.ProjectOverview]{{Project: ov.ProjectRef, Items: []gitlab.ProjectOverview{ov}, Total: 1}},
		}})
	}
	d.Send(batchMsg[gitlab.ProjectOverview]{tok: tok, done: true})
	if mrs.n != 0 || pipes.n != 0 {
		t.Fatalf("tag reports rebuilt the group tabs: MRs %d, Pipelines %d", mrs.n, pipes.n)
	}
	if n := client.CallCount("MergeRequests.List") - asked; n > 1 {
		t.Fatalf("MR of the local branch asked %d times", n)
	}
	if !strings.Contains(lineWith(t, d, "platform/route-service"), "v1.96.0") {
		t.Fatalf("the Projects tab shows the last report:\n%s", d.View())
	}

	// A changed review count is shown by the MRs tab: it is rebuilt.
	ov := scr.sh.over["platform/route-service"]
	ov.ReviewMRs++
	d.Send(batchMsg[gitlab.ProjectOverview]{tok: tok, res: gitlab.BatchResult[gitlab.ProjectOverview]{
		Groups: []gitlab.ProjectGroup[gitlab.ProjectOverview]{{Project: ov.ProjectRef, Items: []gitlab.ProjectOverview{ov}, Total: 1}},
	}})
	if mrs.n != 1 {
		t.Fatalf("MRs rebuilds = %d", mrs.n)
	}
}

func TestGoldenProjects(t *testing.T) {
	d, _, _ := open(t, newFake(), testScope())
	d.RequireGoldenSizes("projects")
}
