package workspace

import (
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

// typeText sends every rune of s as a key press.
func typeText(d *uitest.Driver, s string) {
	for _, r := range s {
		d.Key(string(r))
	}
}

func TestProjectFilterText(t *testing.T) {
	client := newFake()
	d, _, _ := open(t, client, testScope())
	d.Key("2")
	before := len(client.Calls())
	d.Key("P")
	d.RequireView("Projects · MRs", "[ ] platform/route-service", "5 / 5")
	typeText(d, "planner")
	d.RequireView("› planner", "[ ] frontend/route-planner-ui", "1 / 5")
	d.Key("enter") // no marks: the text filters
	d.RequireView("project: planner ✕", "showing 1 of 5 projects", "▸ frontend/route-planner-ui  1 open", "2 MRs 1")
	d.RequireNoView("platform/route-service", "qa/e2e-routes")
	if n := len(client.Calls()); n != before {
		t.Fatalf("P asks nothing: %v", client.Calls()[before:])
	}
	// Each tab has its own P.
	d.Key("3")
	d.RequireView("▸ platform/route-service")
	d.RequireNoView("project: planner")
	d.Key("2")
	// P opens with the text; esc changes nothing.
	d.Key("P")
	d.RequireView("› planner")
	d.Key("backspace", "space", "esc")
	d.RequireView("project: planner ✕", "showing 1 of 5 projects")
	// An empty input and enter reset it.
	d.Key("P")
	for range "planner" {
		d.Key("backspace")
	}
	d.Key("enter")
	d.RequireView("▸ platform/route-service", "▸ qa/e2e-routes")
	d.RequireNoView("project:")
	// backspace outside the overlay resets it with the other filters.
	d.Key("P")
	typeText(d, "e2e")
	d.Key("enter", "backspace")
	d.RequireView("▸ platform/route-service", "▸ qa/e2e-routes")
	if n := client.CallCount("Workspace.MergeRequests"); n != 0 {
		t.Fatalf("nothing is expanded: %d calls", n)
	}
	d.Key("ctrl+l")
	d.Key("P")
	d.RequireView("Проекты · MRs")
	typeText(d, "planner")
	d.Key("enter")
	d.RequireView("проект: planner ✕", "показаны 1 из 5 проектов")
}

func TestProjectFilterMarks(t *testing.T) {
	client := newFake()
	env := newEnv(client)
	statePath := withState(t, env, nil, nil)
	d := uitest.New(t, app.New(env, New(env, source, testScope()), app.Options{}))
	d.Key("3")
	before := len(client.Calls())
	d.Key("P")
	d.RequireView("Projects · Pipelines")
	// By activity: route-service, routing-engine, route-planner-ui…
	d.Key("space", "down", "down", "space")
	d.RequireView("Projects · Pipelines  2 marked", "[x] platform/route-service", "[ ] platform/routing-engine", "[x] frontend/route-planner-ui")
	d.Key("enter")
	d.RequireView("projects: 2 ✕", "showing 2 of 5 projects", "▸ platform/route-service", "▸ frontend/route-planner-ui")
	d.RequireNoView("platform/routing-engine", "qa/e2e-routes", "infra/router-configs")
	if n := len(client.Calls()); n != before {
		t.Fatalf("the overlay asks nothing: %v", client.Calls()[before:])
	}
	st := cache.LoadState(statePath)
	if got := st.Selected(env.Host, source, "pipelines"); strings.Join(got, ",") != "platform/route-service,frontend/route-planner-ui" {
		t.Fatalf("saved = %v", got)
	}
	if got := st.Selected(env.Host, source, "mrs"); len(got) != 0 {
		t.Fatalf("MRs has its own = %v", got)
	}

	// P opens with the marks on top; esc changes nothing.
	d.Key("P")
	if a, b := lineIndex(d, "[x] frontend/route-planner-ui"), lineIndex(d, "[ ] platform/routing-engine"); a < 0 || b < 0 || a > b {
		t.Fatalf("marked first:\n%s", d.View())
	}
	d.Key("space", "esc")
	d.RequireView("projects: 2 ✕")

	// Marks win over the text; one mark shows its path.
	d.Key("P")
	typeText(d, "planner")
	d.Key("space", "enter")
	d.RequireView("project: platform/route-service ✕", "showing 1 of 5 projects")
	if got := cache.LoadState(statePath).Selected(env.Host, source, "pipelines"); strings.Join(got, ",") != "platform/route-service" {
		t.Fatalf("saved = %v", got)
	}

	// backspace drops P and erases the saved marks.
	d.Key("backspace")
	d.RequireNoView("project:")
	d.RequireView("▸ qa/e2e-routes")
	if got := cache.LoadState(statePath).Selected(env.Host, source, "pipelines"); len(got) != 0 {
		t.Fatalf("backspace erases the marks: %v", got)
	}
	d.Key("P")
	d.RequireNoView("[x]", "› planner")
}

func TestProjectFilterRestored(t *testing.T) {
	client := newFake()
	env := newEnv(client)
	statePath := withState(t, env, nil, nil)
	var st cache.State
	st.SetSelected(env.Host, source, "pipelines", []string{"platform/route-service", "frontend/route-planner-ui"}, now)
	st.SetSelected(env.Host, source, "mrs", []string{"gone/project", "qa/e2e-routes"}, now)
	if err := cache.SaveState(statePath, st); err != nil {
		t.Fatal(err)
	}
	env.Catalog = app.NewCatalog(nil, statePath, func() string { return env.Host })
	d := uitest.New(t, app.New(env, New(env, source, testScope()), app.Options{}))
	d.Key("3")
	d.RequireView("projects: 2 ✕", "showing 2 of 5 projects", "▸ platform/route-service", "▸ frontend/route-planner-ui")
	d.RequireNoView("platform/routing-engine")
	// A project that left the workspace is dropped.
	d.Key("2")
	d.RequireView("project: qa/e2e-routes ✕", "showing 1 of 5 projects")
	d.Key("P")
	d.RequireView("1 marked", "[x] qa/e2e-routes")
	d.RequireNoView("gone/project")
	d.Key("esc")

	// Only missing projects left: no filter.
	st.SetSelected(env.Host, source, "mrs", []string{"gone/project"}, now)
	if err := cache.SaveState(statePath, st); err != nil {
		t.Fatal(err)
	}
	env.Catalog = app.NewCatalog(nil, statePath, func() string { return env.Host })
	d = uitest.New(t, app.New(env, New(env, source, testScope()), app.Options{}))
	d.Key("2")
	d.RequireNoView("project:", "projects:")
	d.RequireView("▸ qa/e2e-routes", "▸ platform/route-service")
}

func TestProjectPickerKeys(t *testing.T) {
	d, _, _ := open(t, newFake(), testScope())
	d.Key("2", "P")
	typeText(d, "platform")
	d.RequireView("2 / 5")
	d.Key("ctrl+a") // every visible line
	d.RequireView("2 marked", "[x] platform/route-service", "[x] platform/routing-engine")
	d.Key("ctrl+a") // all marked: unmark them
	d.RequireNoView("[x]", "marked")
	d.Key("ctrl+u", "ctrl+d", "pgdown", "pgup") // never edit the input
	d.RequireView("› platform")
	d.Key("space")
	for range "platform" {
		d.Key("backspace")
	}
	d.RequireView("1 marked", "5 / 5")
	d.Key("ctrl+a")
	d.RequireView("5 marked")
	d.Key("enter")
	d.RequireView("projects: 5 ✕", "showing 5 of 5 projects")
}

func TestGoldenProjectPicker(t *testing.T) {
	d, _, _ := open(t, newFake(), testScope())
	d.Key("3", "P", "space", "down", "down", "space", "down")
	d.RequireGoldenSizes("pipelines-picker")
	d.Key("enter")
	d.RequireGoldenSizes("pipelines-pmarks")
}

func TestGoldenProjectFilter(t *testing.T) {
	d, _, _ := open(t, newFake(), testScope())
	d.Key("2", "P")
	typeText(d, "platform")
	d.Key("enter")
	d.RequireGoldenSizes("mrs-pfilter")
	d.Key("m")
	d.RequireGoldenSizes("mrs-filtermode")
}

func TestFilterModeAsksEveryProjectPassingP(t *testing.T) {
	client := newFake()
	env := newEnv(client)
	statePath := withState(t, env, []string{"platform/route-service"}, nil)
	scr := New(env, source, testScope())
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	d.Key("2")
	d.Key("P")
	typeText(d, "platform")
	d.Key("enter")
	d.Key("/")
	typeText(d, "Fix")
	d.Key("enter")
	args := lastCall(t, client, "Workspace.MergeRequests")
	if paths := args[0].([]string); strings.Join(paths, ",") != "platform/route-service,platform/routing-engine" {
		t.Fatalf("filter mode asks %v", paths)
	}
	if f := args[1].(gitlab.MRFilter); f.Search != "Fix" {
		t.Fatalf("filter = %+v", f)
	}
	d.RequireView("▾ platform/route-service  1 open", "Fix race in courier slot allocator", "found in 1 of 2 projects", "2 MRs 1")
	d.RequireNoView("platform/routing-engine", "frontend/route-planner-ui")

	// Collapsing in filter mode is not saved and asks nothing.
	calls := client.CallCount("Workspace.MergeRequests")
	d.Key("g", "space")
	d.RequireView("▸ platform/route-service  1 open")
	if client.CallCount("Workspace.MergeRequests") != calls {
		t.Fatal("collapsing asks nothing")
	}
	if got := cache.LoadState(statePath).Expanded(env.Host, source, "mrs"); strings.Join(got, ",") != "platform/route-service" {
		t.Fatalf("state = %v", got)
	}

	// A wider P asks only the projects not asked yet, each once.
	before := len(client.CallsOf("Workspace.MergeRequests"))
	d.Key("P")
	for range "platform" {
		d.Key("backspace")
	}
	d.Key("enter")
	asked := map[string]int{}
	for _, c := range client.CallsOf("Workspace.MergeRequests")[before:] {
		for _, p := range c[0].([]string) {
			asked[p]++
		}
	}
	if len(asked) != 3 || asked["platform/route-service"] != 0 || asked["qa/e2e-routes"] != 1 {
		t.Fatalf("wider P asks %v", asked)
	}
	// Marking two of them asks nothing.
	before = len(client.CallsOf("Workspace.MergeRequests"))
	d.Key("P", "space", "down", "space", "enter")
	if n := len(client.CallsOf("Workspace.MergeRequests")); n != before {
		t.Fatalf("a narrower P asks nothing: %d calls", n-before)
	}

	// backspace: back to the saved groups; only the expanded one is asked.
	d.Key("backspace")
	if paths := lastCall(t, client, "Workspace.MergeRequests")[0].([]string); strings.Join(paths, ",") != "platform/route-service" {
		t.Fatalf("after reset asks %v", paths)
	}
	d.RequireView("▾ platform/route-service  4 open · 2 waiting for you · showing 3", "▸ platform/routing-engine", "▸ qa/e2e-routes")
	d.RequireNoView("found in")
}

func TestFilterModePipelines(t *testing.T) {
	client := newFake()
	d, _, _ := open(t, client, testScope())
	d.Key("3", "m")
	args := lastCall(t, client, "Workspace.Pipelines")
	if paths := args[0].([]string); len(paths) != 5 {
		t.Fatalf("m asks every project: %v", paths)
	}
	if f := args[1].(gitlab.PipelineFilter); f.Username != "egor" {
		t.Fatalf("filter = %+v", f)
	}
	d.RequireView("me: mine", "nothing found", "found in 0 of 5 projects")
	d.Key("m", "s") // status: running
	d.RequireView("▾ frontend/route-planner-ui  showing 1", "#7001", "found in 1 of 5 projects", "3 Pipelines 1")
	d.RequireNoView("platform/route-service")
	d.Key("backspace")
	if n := len(client.CallsOf("Workspace.Pipelines")); n != 2 {
		t.Fatalf("nothing is expanded: no request after reset (%d calls)", n)
	}
	d.RequireView("▸ platform/route-service  ► main #91790")
}
