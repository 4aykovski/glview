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

func TestProjectFilterIsLocal(t *testing.T) {
	client := newFake()
	d, _, _ := open(t, client, testScope())
	d.Key("2")
	before := len(client.Calls())
	d.Key("P")
	typeText(d, "planner")
	d.RequireView("project: planner", "▸ frontend/route-planner-ui  1 open")
	d.RequireNoView("platform/route-service", "qa/e2e-routes")
	d.Key("esc") // leaves the input, keeps the filter
	d.RequireView("project: planner ✕", "showing 1 of 5 projects", "▸ frontend/route-planner-ui", "2 MRs 1")
	if n := len(client.Calls()); n != before {
		t.Fatalf("P asks nothing: %v", client.Calls()[before:])
	}
	// Each tab has its own P.
	d.Key("3")
	d.RequireView("▸ platform/route-service")
	d.RequireNoView("project: planner")
	d.Key("2")
	// An empty input and enter reset it.
	d.Key("P")
	for range "planner" {
		d.Key("backspace")
	}
	d.Key("enter")
	d.RequireView("▸ platform/route-service", "▸ qa/e2e-routes")
	d.RequireNoView("project:")
	// backspace outside the input resets it with the other filters.
	d.Key("P")
	typeText(d, "e2e")
	d.Key("enter", "backspace")
	d.RequireView("▸ platform/route-service", "▸ qa/e2e-routes")
	if n := client.CallCount("Workspace.MergeRequests"); n != 0 {
		t.Fatalf("nothing is expanded: %d calls", n)
	}
	d.Key("ctrl+l")
	d.Key("P")
	typeText(d, "planner")
	d.Key("enter")
	d.RequireView("проект: planner ✕", "показаны 1 из 5 проектов")
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
	asked := map[string]int{}
	for _, c := range client.CallsOf("Workspace.MergeRequests")[before:] {
		for _, p := range c[0].([]string) {
			asked[p]++
		}
	}
	if len(asked) != 3 || asked["platform/route-service"] != 0 || asked["qa/e2e-routes"] != 1 {
		t.Fatalf("wider P asks %v", asked)
	}
	d.Key("esc")

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
