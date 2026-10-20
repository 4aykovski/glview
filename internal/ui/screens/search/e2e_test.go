package search

import (
	"testing"

	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	wsscreen "github.com/4aykovski/glview/internal/ui/screens/workspace"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

// TestWorkspaceEndToEnd: folder → Projects → 2 → f → Project MRs → esc →
// p → another project → esc → back in the Workspace.
func TestWorkspaceEndToEnd(t *testing.T) {
	client := newFake()
	env, _ := setup(t, client)
	ws := wsscreen.New(env, "~/src", wsScope())
	root := app.New(env, ws, app.Options{})
	d := uitest.New(t, root)
	d.RequireView("~/src · workspace · 2 GitLab projects", "platform/route-service", "infra/router-configs")
	if ws.Current() != wsscreen.TabProjects {
		t.Fatal("starts on Projects")
	}

	d.Key("2")
	d.RequireView("▾ platform/route-service", "!479")
	d.Key("f")
	p, ok := root.Top().(*project.Project)
	if !ok || p.Path() != "platform/route-service" || p.Current() != project.TabMRs {
		t.Fatalf("top = %T", root.Top())
	}
	d.RequireView("~/src › platform/route-service", "Fix race in courier slot allocator")

	d.Key("esc")
	if root.Top() != ws || ws.Current() != wsscreen.TabMRs {
		t.Fatal("back on the workspace MRs tab")
	}

	d.Key("p", "e", "2", "e", "enter")
	p, ok = root.Top().(*project.Project)
	if !ok || p.Path() != "qa/e2e-routes" || root.Depth() != 2 {
		t.Fatalf("picker opened %T, depth %d", root.Top(), root.Depth())
	}
	d.Key("esc")
	if root.Top() != ws || root.Depth() != 1 {
		t.Fatal("esc returns to the workspace")
	}
	d.RequireView("~/src · workspace")
}
