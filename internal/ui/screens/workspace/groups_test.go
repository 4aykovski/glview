package workspace

import (
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/grouped"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/uitest"
	"github.com/4aykovski/glview/internal/workspace"
)

// lastCall returns the arguments of the last call of method.
func lastCall(t *testing.T, calls interface{ CallsOf(string) [][]any }, method string) []any {
	t.Helper()
	all := calls.CallsOf(method)
	if len(all) == 0 {
		t.Fatalf("no %s calls", method)
	}
	return all[len(all)-1]
}

func TestGroupTabsLoadLazily(t *testing.T) {
	client := newFake()
	d, _, _ := openExpanded(t, client, testScope(), allPaths, nil)
	if client.CallCount("Workspace.MergeRequests") != 0 {
		t.Fatal("MRs are not asked before the tab is opened")
	}
	d.Key("2")
	if client.CallCount("Workspace.MergeRequests") != 1 {
		t.Fatalf("calls = %d", client.CallCount("Workspace.MergeRequests"))
	}
	d.RequireView("▾ platform/route-service  4 open · 2 waiting for you · showing 3", "!482", "Draft: add surge pricing",
		"↳ 1 more · f filter by project", "▾ infra/router-configs", "2 MRs 6")
	if next := lineAfter(t, d, "▾ infra/router-configs"); strings.TrimSpace(next) != "no open" {
		t.Fatalf("an empty expanded group: %q", next)
	}
	d.Key("1", "2")
	if client.CallCount("Workspace.MergeRequests") != 1 {
		t.Fatal("a loaded tab is not reloaded at once")
	}
	if client.CallCount("Workspace.Pipelines") != 0 {
		t.Fatal("pipelines load lazily too")
	}
}

func TestGroupsCollapsedByDefault(t *testing.T) {
	client := newFake()
	d, _, _ := open(t, client, testScope())
	d.Key("2")
	if n := client.CallCount("Workspace.MergeRequests"); n != 0 {
		t.Fatalf("collapsed groups are not asked: %d calls", n)
	}
	d.RequireView("▸ platform/route-service  4 open · 2 waiting for you", "▸ platform/routing-engine  1 open · 1 waiting for you",
		"▸ frontend/route-planner-ui  1 open", "▸ infra/router-configs  no open", "▸ qa/e2e-routes  no open", "2 MRs 6")
	d.RequireNoView("!482", "loading")
	d.Key("3")
	if n := client.CallCount("Workspace.Pipelines"); n != 0 {
		t.Fatalf("collapsed groups are not asked: %d calls", n)
	}
	d.RequireView("▸ platform/route-service  ► main #91790 · 5h", "▸ frontend/route-planner-ui  ◐ main #7001 · 3m",
		"▸ qa/e2e-routes  ✓ main #5501 · 2w")
	d.RequireNoView("3 Pipelines 6")
}

func TestGroupHeaderBeforeOverview(t *testing.T) {
	client := newFake()
	for _, p := range allPaths {
		client.SetProjectDelay(p, time.Second) // the overview never comes in this test
	}
	d, _, _ := open(t, client, testScope())
	d.Key("2")
	if !strings.HasSuffix(strings.TrimRight(lineWith(t, d, "▸ platform/route-service"), " "), "platform/route-service") {
		t.Fatalf("without the overview only the path: %q", lineWith(t, d, "▸ platform/route-service"))
	}
}

func TestExpandAsksOnlyThatGroupAndIsRemembered(t *testing.T) {
	client := newFake()
	env := newEnv(client)
	statePath := withState(t, env, nil, nil)
	scr := New(env, source, testScope())
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	d.Key("2", "space") // route-service
	d.RequireView("▾ platform/route-service  4 open · 2 waiting for you")
	if next := lineAfter(t, d, "▾ platform/route-service"); strings.TrimSpace(next) != "◐ loading…" {
		t.Fatalf("loading line: %q", next)
	}
	if client.CallCount("Workspace.MergeRequests") != 0 {
		t.Fatal("the request waits for the expand delay")
	}
	d.Key("J", "J", "space") // and frontend/route-planner-ui within the delay
	d.Send(scr.ExpandMsg())
	calls := client.CallsOf("Workspace.MergeRequests")
	if len(calls) != 1 {
		t.Fatalf("one request for both groups: %d", len(calls))
	}
	if paths := calls[0][0].([]string); strings.Join(paths, ",") != "platform/route-service,frontend/route-planner-ui" {
		t.Fatalf("asked %v", paths)
	}
	d.RequireView("▾ platform/route-service  4 open · 2 waiting for you · showing 3", "!482", "!311", "▸ platform/routing-engine")
	if line := lineWith(t, d, "frontend/route-planner-ui"); !strings.HasPrefix(line, "› ▾") {
		t.Fatalf("the cursor stays on the toggled header: %q", line)
	}
	// enter on a header collapses it, without a request.
	d.Key("enter")
	d.RequireView("▸ frontend/route-planner-ui  1 open")
	d.RequireNoView("!311")
	if client.CallCount("Workspace.MergeRequests") != 1 {
		t.Fatal("collapsing asks nothing")
	}
	// space on a record collapses its group.
	d.Key("g", "j", "space")
	d.RequireView("▸ platform/route-service  4 open")
	d.Key("space")
	d.Send(scr.ExpandMsg())
	if got := cache.LoadState(statePath).Expanded(env.Host, source, "mrs"); strings.Join(got, ",") != "platform/route-service" {
		t.Fatalf("state = %v", got)
	}

	// Next start: the group is expanded again in MRs, not in Pipelines.
	client2 := newFake()
	env2 := newEnv(client2)
	env2.Catalog = app.NewCatalog(nil, statePath, func() string { return env2.Host })
	d2 := uitest.New(t, app.New(env2, New(env2, source, testScope()), app.Options{}))
	d2.Key("2")
	if paths := lastCall(t, client2, "Workspace.MergeRequests")[0].([]string); strings.Join(paths, ",") != "platform/route-service" {
		t.Fatalf("restored: %v", paths)
	}
	d2.RequireView("▾ platform/route-service", "!482", "▸ frontend/route-planner-ui")
	d2.Key("3")
	if client2.CallCount("Workspace.Pipelines") != 0 {
		t.Fatal("Pipelines keep their own set")
	}
	d2.RequireView("▸ platform/route-service  ► main")
}

func TestPollAndRefreshAskOnlyExpanded(t *testing.T) {
	client := newFake()
	d, _, scr := openExpanded(t, client, testScope(), []string{"platform/routing-engine"}, nil)
	d.Key("2")
	mrs := scr.tabs[TabMRs].(*mrsTab)
	overviews := client.CallCount("Workspace.ProjectsOverview")
	check := func(what string) {
		t.Helper()
		if paths := lastCall(t, client, "Workspace.MergeRequests")[0].([]string); strings.Join(paths, ",") != "platform/routing-engine" {
			t.Fatalf("%s asks %v", what, paths)
		}
		overviews++
		if n := client.CallCount("Workspace.ProjectsOverview"); n != overviews {
			t.Fatalf("%s: overview calls = %d, want %d", what, n, overviews)
		}
		if o := lastCall(t, client, "Workspace.ProjectsOverview")[1].(gitlab.OverviewOptions); !o.SkipTags {
			t.Fatalf("%s: the headers need no tags", what)
		}
	}
	if paths := lastCall(t, client, "Workspace.MergeRequests")[0].([]string); strings.Join(paths, ",") != "platform/routing-engine" {
		t.Fatalf("first open asks %v", paths)
	}
	d.Send(common.PollTickMsg{Token: mrs.poll.Current()})
	check("poll")
	d.Key("r")
	check("r")
	if n := client.CallCount("Workspace.MergeRequests"); n != 3 {
		t.Fatalf("calls = %d", n)
	}
}
func TestBatchesInReverseOrderKeepGroups(t *testing.T) {
	client := newFake()
	d, _, scr := openExpanded(t, client, testScope(), allPaths, nil)
	mrs := scr.tabs[TabMRs].(*mrsTab)
	for _, p := range projects {
		client.SetProjectDelay(p.path, time.Second) // the real answers never come in this test
	}
	d.Key("2")
	order := []string{"platform/route-service", "platform/routing-engine", "frontend/route-planner-ui", "infra/router-configs", "qa/e2e-routes"}
	requireOrder := func() {
		t.Helper()
		prev := -1
		for _, p := range order {
			i := lineIndex(d, "▾ "+p)
			if i <= prev {
				t.Fatalf("%s out of order:\n%s", p, d.View())
			}
			prev = i
		}
	}
	if next := lineAfter(t, d, "▾ platform/route-service"); strings.TrimSpace(next) != "◐ loading…" {
		t.Fatalf("loading line: %q", next)
	}
	requireOrder()

	tok := mrs.full
	group := func(path string, items ...gitlab.MergeRequest) gitlab.ProjectGroup[gitlab.MergeRequest] {
		return gitlab.ProjectGroup[gitlab.MergeRequest]{Project: gitlab.ProjectRef{FullPath: path}, Items: items, Total: len(items)}
	}
	mr := func(iid int64, title string) gitlab.MergeRequest {
		return gitlab.MergeRequest{IID: iid, Title: title, State: gitlab.MRStateOpened, UpdatedAt: now}
	}
	// The second batch arrives first.
	d.Send(batchMsg[gitlab.MergeRequest]{tok: tok, res: gitlab.BatchResult[gitlab.MergeRequest]{Groups: []gitlab.ProjectGroup[gitlab.MergeRequest]{
		group("infra/router-configs"), group("qa/e2e-routes", mr(9, "e2e flake")),
	}}})
	d.RequireView("e2e flake", "▾ infra/router-configs")
	if next := lineAfter(t, d, "▾ platform/route-service"); strings.TrimSpace(next) != "◐ loading…" {
		t.Fatalf("loading line: %q", next)
	}
	if next := lineAfter(t, d, "▾ infra/router-configs"); strings.TrimSpace(next) != "no open" {
		t.Fatalf("empty line: %q", next)
	}
	requireOrder()
	d.Send(batchMsg[gitlab.MergeRequest]{tok: tok, res: gitlab.BatchResult[gitlab.MergeRequest]{Groups: []gitlab.ProjectGroup[gitlab.MergeRequest]{
		group("platform/route-service", mr(479, "Fix race")), group("platform/routing-engine"),
		group("frontend/route-planner-ui", mr(311, "Map layers")),
	}}})
	d.RequireNoView("loading…")
	requireOrder()
}

func TestNullProjectGroupNoAccess(t *testing.T) {
	client := newFake()
	rows := append([]workspace.Project{localRow("/src/gone", "gone", "platform/gone", "main")}, testScope().Rows...)
	d, _, _ := openExpanded(t, client, workspace.NewScope(source, rows), []string{"platform/gone", "platform/route-service"}, nil)
	d.Key("2")
	d.RequireView("✗ platform/gone  no access", "▾ platform/route-service")
	if paths := lastCall(t, client, "Workspace.MergeRequests")[0].([]string); len(paths) != 2 {
		t.Fatalf("asked %v", paths)
	}
	d.Key("3")
	d.RequireView("✗ platform/gone  no access", "#91790")
}

func TestMineFilter(t *testing.T) {
	client := newFake()
	d, _, _ := openExpanded(t, client, testScope(), allPaths, nil)
	d.Key("2", "m")
	f := lastCall(t, client, "Workspace.MergeRequests")[1].(gitlab.MRFilter)
	if !f.Mine || f.State != gitlab.MRStateOpened {
		t.Fatalf("filter = %+v", f)
	}
	d.RequireView("me: author/assignee/reviewer", "▾ platform/route-service  ≥3 open")
	if next := lineAfter(t, d, "▾ infra/router-configs"); strings.TrimSpace(next) != "no open MRs of yours" {
		t.Fatalf("empty line: %q", next)
	}
	row := lineWith(t, d, "!479")
	if !strings.Contains(row, "author") {
		t.Fatalf("role: %q", row)
	}
	if !strings.Contains(lineWith(t, d, "!482"), "reviewer") {
		t.Fatal("reviewer role")
	}
	d.Key("backspace")
	if f := lastCall(t, client, "Workspace.MergeRequests")[1].(gitlab.MRFilter); f.Mine {
		t.Fatal("backspace resets the filters")
	}
	d.Key("s")
	d.RequireView("state:merged")
}

func TestApproveInRowProject(t *testing.T) {
	client := newFake()
	d, _, _ := openExpanded(t, client, testScope(), []string{"platform/route-service"}, nil)
	d.Key("2", "j", "j") // header, !482, !479
	d.Key("a")
	args := lastCall(t, client, "MergeRequests.Approve")
	if args[0] != "platform/route-service" || args[1] != int64(479) {
		t.Fatalf("approve = %v", args)
	}
	d.RequireView("approved !479")
	reload := lastCall(t, client, "Workspace.MergeRequests")[0].([]string)
	if len(reload) != 1 || reload[0] != "platform/route-service" {
		t.Fatalf("the group is asked again: %v", reload)
	}
	d.Key("k") // !482
	d.Key("K") // header: actions on a record are not available
	before := client.CallCount("MergeRequests.Approve")
	d.Key("a")
	if client.CallCount("MergeRequests.Approve") != before {
		t.Fatal("a on a group header does nothing")
	}
}

func TestPipelinesFastTickOnlyRunning(t *testing.T) {
	client := newFake()
	d, _, scr := openExpanded(t, client, testScope(), nil, allPaths)
	d.Key("3")
	d.RequireView("▾ frontend/route-planner-ui", "◐ running", "#7001")
	if n := len(lastCall(t, client, "Workspace.Pipelines")[0].([]string)); n != 5 {
		t.Fatalf("first load asks every project: %d", n)
	}
	pipes := scr.tabs[TabPipelines].(*pipelinesTab)
	d.Send(pipes.FastMsg())
	paths := lastCall(t, client, "Workspace.Pipelines")[0].([]string)
	if len(paths) != 1 || paths[0] != "frontend/route-planner-ui" {
		t.Fatalf("fast tick asks %v", paths)
	}
}

func TestFilterByProjectAndBack(t *testing.T) {
	client := newFake()
	d, root, scr := openExpanded(t, client, testScope(), allPaths, nil)
	d.Key("2")
	// Collapse routing-engine, put the cursor on !311 of route-planner-ui.
	d.Key("J")      // routing-engine header
	d.Key("space")  // collapse
	d.Key("J", "j") // route-planner-ui → !311
	mrs := scr.tabs[TabMRs].(*mrsTab)
	if !strings.HasPrefix(lineWith(t, d, "!311"), "›") {
		t.Fatalf("cursor on !311:\n%s", d.View())
	}
	_ = grouped.RowID
	d.Key("f")
	if root.Depth() != 2 {
		t.Fatal("f opens the project")
	}
	p := root.Top().(*project.Project)
	if p.Path() != "frontend/route-planner-ui" || p.Current() != project.TabMRs {
		t.Fatalf("project %s tab %d", p.Path(), p.Current())
	}
	d.RequireView("~/src › frontend/route-planner-ui")
	d.Key("esc")
	if scr.Current() != TabMRs {
		t.Fatal("back on the MRs tab")
	}
	if !strings.HasPrefix(lineWith(t, d, "!311"), "›") {
		t.Fatalf("cursor on !311:\n%s", d.View())
	}
	if mrs.expanded["platform/routing-engine"] || strings.Contains(d.View(), "Contraction hierarchies") {
		t.Fatal("collapsed group stays collapsed")
	}

	// m and state:merged are carried into the project.
	d.Key("m", "s", "f")
	d.RequireView("state:merged", "me")
	d.Key("esc")

	// Pipelines with "mine": the project opens on Pipelines with the filter.
	d.Key("3", "m")
	d.Key("f")
	p = root.Top().(*project.Project)
	if p.Current() != project.TabPipelines {
		t.Fatal("Pipelines tab")
	}
	d.RequireView("user:egor")
}

func TestGoldenGroupTabs(t *testing.T) {
	d, _, _ := open(t, newFake(), testScope())
	d.Key("2")
	d.RequireGoldenSizes("mrs-collapsed")
	d.Key("3")
	d.RequireGoldenSizes("pipelines-collapsed")

	d, _, _ = openExpanded(t, newFake(), testScope(), []string{"platform/route-service", "infra/router-configs"},
		[]string{"platform/routing-engine", "frontend/route-planner-ui"})
	d.Key("2")
	d.RequireGoldenSizes("mrs-expanded")
	d.Key("3")
	d.RequireGoldenSizes("pipelines-expanded")
}

func openGroup(t *testing.T, client *fake.Client, limit int) (*uitest.Driver, *Screen) {
	t.Helper()
	env := newEnv(client)
	env.Settings.MaxGroupProjects = limit
	scr := NewGroup(env, gitlab.Group{ID: 9, FullPath: "platform"})
	return uitest.New(t, app.New(env, scr, app.Options{})), scr
}

func TestGroupListErrorRetries(t *testing.T) {
	client := newFake()
	client.AddGroup(gitlab.Group{ID: 9, FullPath: "platform"}, "platform/route-service", "platform/routing-engine")
	client.FailNext("Projects.ListGroupProjects", &gitlab.Error{Kind: gitlab.ErrServer, Message: "502 Bad Gateway"})
	d, scr := openGroup(t, client, 0)
	d.RequireView("group: platform · workspace · ✗ could not load, r — retry")
	d.RequireNoView("platform/route-service")
	if client.CallCount("Workspace.ProjectsOverview") != 0 {
		t.Fatal("no overview without the list")
	}
	d.Key("r")
	if n := client.CallCount("Projects.ListGroupProjects"); n != 2 {
		t.Fatalf("r must reload the list: %d calls", n)
	}
	d.RequireView("group: platform · workspace · 2 projects", "platform/route-service", "platform/routing-engine")
	d.RequireNoView("could not load")
	if !scr.sh.ready || client.CallCount("Workspace.ProjectsOverview") != 1 {
		t.Fatalf("after the retry the workspace works as usual: overview calls = %d", client.CallCount("Workspace.ProjectsOverview"))
	}
	d.Key("ctrl+l")
	d.RequireView("group: platform · workspace · 2 проекта")
}

func TestGroupHeaderShownOfMany(t *testing.T) {
	client := newFake()
	client.AddGroup(gitlab.Group{ID: 9, FullPath: "platform"}, "platform/route-service", "platform/routing-engine", "qa/e2e-routes")
	client.SetUnknownTotal(9)
	d, _ := openGroup(t, client, 2)
	d.RequireView("group: platform · workspace · 2 projects · showing 2 of many")
	d.Key("ctrl+l")
	d.RequireView("показаны 2 из многих")
}
