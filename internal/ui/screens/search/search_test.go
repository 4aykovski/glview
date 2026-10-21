package search

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/screens/joblog"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/screens/start"
	wsscreen "github.com/4aykovski/glview/internal/ui/screens/workspace"
	"github.com/4aykovski/glview/internal/ui/uitest"
	"github.com/4aykovski/glview/internal/workspace"
)

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 60 * time.Millisecond
	os.Exit(m.Run())
}

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func newFake() *fake.Client {
	c := fake.New()
	for i, p := range []string{"platform/route-service", "platform/routing-engine", "infra/router-configs", "qa/e2e-routes", "logistics/route-api", "frontend/route-planner-ui"} {
		c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: int64(i + 1), FullPath: p}, DefaultBranch: "main",
			LastActivityAt: now.Add(-time.Duration(i+1) * time.Hour)})
	}
	c.AddMR("platform/route-service", gitlab.MergeRequest{IID: 479, Title: "Fix race in courier slot allocator", State: gitlab.MRStateOpened, UpdatedAt: now})
	c.AddMR("infra/router-configs", gitlab.MergeRequest{IID: 12, Title: "Bump zone limits", State: gitlab.MRStateOpened, UpdatedAt: now})
	return c
}

// setup returns an env whose Search records the last overlay.
func setup(t *testing.T, client *fake.Client) (*app.Env, **Overlay) {
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	disk := &cache.Disk{Dir: t.TempDir()}
	_ = disk.SaveStarred(env.Host, []gitlab.Project{
		{ProjectRef: gitlab.ProjectRef{FullPath: "platform/routing-engine"}, LastActivityAt: now.Add(-time.Hour)},
		{ProjectRef: gitlab.ProjectRef{FullPath: "platform/route-service"}},
	})
	statePath := filepath.Join(t.TempDir(), "state.json")
	var st cache.State
	st.Touch(env.Host, "infra/router-configs")
	st.Touch(env.Host, "qa/e2e-routes")
	_ = cache.SaveState(statePath, st)
	env.Disk = disk
	env.Catalog = app.NewCatalog(disk, statePath, func() string { return env.Host })
	last := new(*Overlay)
	env.Search = func(global bool) app.Overlay {
		o := New(env, global)
		*last = o
		return o
	}
	return env, last
}

func repo(root string) *gitremote.Repo {
	return &gitremote.Repo{Root: root, Branch: "main", Remote: "origin"}
}

func wsScope() workspace.Scope {
	row := func(dir, path string) workspace.Project {
		r := repo("/src/" + dir)
		return workspace.Project{Ref: gitlab.ProjectRef{FullPath: path},
			Local: &gitremote.LocalProject{Dir: "/src/" + dir, Name: dir, Kind: gitremote.KindGitLab, Repo: r, ProjectPath: path}}
	}
	return workspace.NewScope("~/src", []workspace.Project{
		row("route-service", "platform/route-service"), row("router-configs", "infra/router-configs"),
	})
}

func TestPickerWithoutNetwork(t *testing.T) {
	client := newFake()
	env, _ := setup(t, client)
	env.Catalog.SetCurrent("platform/route-service", repo("/src/route-service"))
	root := app.New(env, project.New(env, project.Options{Path: "platform/route-service"}), app.Options{})
	d := uitest.New(t, root)
	client.ResetCalls()
	d.Key("p")
	d.RequireView("Switch project", "FROM GIT REMOTE", "platform/route-service", "current", "STARRED", "platform/routing-engine", "★",
		"RECENT", "qa/e2e-routes", "4 / 4")
	if strings.Count(d.View(), "platform/route-service") != 2 { // the header and one picker line
		t.Fatalf("a project in several sources is listed once:\n%s", d.View())
	}
	d.Key("r", "o", "u", "t")
	d.RequireView("platform/route-service", "platform/routing-engine", "infra/router-configs", "qa/e2e-routes")
	d.Key("backspace", "backspace", "backspace", "backspace", "s", "r", "v")
	d.RequireView("platform/route-service", "1 / 4")
	d.RequireNoView("qa/e2e-routes", "platform/routing-engine")
	if n := len(client.Calls()); n != 0 {
		t.Fatalf("the picker made %d requests: %+v", n, client.Calls())
	}
	d.Key("x", "x", "x")
	d.RequireView("nothing found — F search GitLab")
	d.Key("esc")
	d.RequireNoView("Switch project")
}

func TestPickerOtherHost(t *testing.T) {
	client := newFake()
	env, _ := setup(t, client)
	env.Host = "https://gitlab.example.org" // the same disk and state, another instance
	root := app.New(env, project.New(env, project.Options{Path: "platform/route-service"}), app.Options{})
	d := uitest.New(t, root)
	d.Key("p")
	d.RequireView("Switch project", "RECENT", "platform/route-service") // just opened on this host
	d.RequireNoView("STARRED", "platform/routing-engine", "qa/e2e-routes", "infra/router-configs")

	d.Key("esc")
	env.Host = "https://gitlab.example.com"
	d.Key("p")
	d.RequireView("STARRED", "platform/routing-engine", "RECENT", "qa/e2e-routes", "infra/router-configs")
}

func TestPickerFromProjectModeKeepsRoot(t *testing.T) {
	client := newFake()
	env, _ := setup(t, client)
	start := project.New(env, project.Options{Path: "platform/route-service"})
	root := app.New(env, start, app.Options{})
	d := uitest.New(t, root)
	d.Send(app.PushMsg{Screen: joblog.New(env, gitlab.ProjectRef{FullPath: "platform/route-service"}, 1, gitlab.Job{ID: 5, Name: "test", Status: gitlab.StatusSuccess})})
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.Key("F") // F works on the job log without an active search
	d.RequireView("Search GitLab", "type at least 3 characters")
	d.Key("esc", "p", "q", "a", "/", "e")
	d.Key("enter")
	if root.Depth() != 2 {
		t.Fatalf("the project goes above the start project: depth %d", root.Depth())
	}
	if p := root.Top().(*project.Project); p.Path() != "qa/e2e-routes" {
		t.Fatalf("top = %s", p.Path())
	}
	// A second p does not grow the stack.
	d.Key("p", "i", "n", "f", "r", "a", "enter")
	if p := root.Top().(*project.Project); root.Depth() != 2 || p.Path() != "infra/router-configs" {
		t.Fatalf("depth %d top %s", root.Depth(), p.Path())
	}
	d.Key("esc")
	if root.Depth() != 1 || root.Top() != start {
		t.Fatal("esc returns to the start project")
	}
	d.RequireView("platform/route-service")
	d.Key("esc")
	if root.Depth() != 1 {
		t.Fatal("esc on the root does nothing")
	}
}

// openGroup types q into F and opens the first group found.
func openGroup(d *uitest.Driver, last **Overlay, q string) {
	d.Key("F")
	for _, r := range q {
		d.Key(string(r))
	}
	d.Send((*last).DebounceMsg())
	for range 30 {
		d.Key("down")
	}
	d.Key("enter")
}

func TestGroupFromSearchKeepsStartWorkspace(t *testing.T) {
	client := newFake()
	client.AddGroup(gitlab.Group{ID: 9, FullPath: "platform", Name: "platform"}, "platform/route-service", "platform/routing-engine")
	client.AddGroup(gitlab.Group{ID: 10, FullPath: "frontend", Name: "frontend"}, "frontend/route-planner-ui")
	env, last := setup(t, client)
	ws := wsscreen.New(env, "~/src", wsScope())
	root := app.New(env, ws, app.Options{})
	d := uitest.New(t, root)
	// MRs tab, route-service expanded, the cursor on !479.
	d.Key("2", "space")
	d.Send(ws.ExpandMsg())
	d.Key("j")
	if !strings.HasPrefix(lineWith(d, "!479"), "›") {
		t.Fatalf("cursor on !479:\n%s", d.View())
	}

	openGroup(d, last, "pla")
	group, ok := root.Top().(*wsscreen.Screen)
	if !ok || group == ws || root.Depth() != 2 {
		t.Fatalf("the group goes above the start workspace: %T depth %d", root.Top(), root.Depth())
	}
	d.RequireView("group: platform · workspace · 2 projects")

	// p lists the projects of the group workspace and opens one above it.
	d.Key("p")
	d.RequireView("WORKSPACE", "platform/routing-engine")
	d.Key("r", "o", "u", "t", "i", "n", "g", "enter")
	if p, ok := root.Top().(*project.Project); !ok || p.Path() != "platform/routing-engine" || root.Depth() != 3 {
		t.Fatalf("stack: depth %d top %T", root.Depth(), root.Top())
	}
	d.Key("esc")
	if root.Top() != group {
		t.Fatal("esc returns to the group workspace")
	}
	// Another group replaces the first one.
	openGroup(d, last, "fron")
	if g, ok := root.Top().(*wsscreen.Screen); !ok || g == group || root.Depth() != 2 {
		t.Fatalf("one group workspace at most: depth %d", root.Depth())
	}
	d.RequireView("group: frontend · workspace")

	d.Key("esc")
	if root.Top() != ws || root.Depth() != 1 || ws.Current() != wsscreen.TabMRs {
		t.Fatal("esc returns to the start workspace on its tab")
	}
	if !strings.HasPrefix(lineWith(d, "!479"), "›") {
		t.Fatalf("the cursor and the expanded group are kept:\n%s", d.View())
	}
	// The picker lists the start workspace again.
	d.Key("p")
	d.RequireView("WORKSPACE", "infra/router-configs")
}

func TestPickerKeepsWorkspaceRoot(t *testing.T) {
	client := newFake()
	env, _ := setup(t, client)
	ws := wsscreen.New(env, "~/src", wsScope())
	root := app.New(env, ws, app.Options{})
	d := uitest.New(t, root)
	d.Key("2", "space") // expand platform/route-service
	d.Send(ws.ExpandMsg())
	d.Key("j", "enter") // MR detail of !479
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.Key("p")
	d.RequireView("WORKSPACE", "infra/router-configs")
	d.Key("i", "n", "f", "r", "a", "enter")
	if root.Depth() != 2 || root.Top().(*project.Project).Path() != "infra/router-configs" {
		t.Fatalf("stack: depth %d top %T", root.Depth(), root.Top())
	}
	if r := env.Catalog.LocalRepo("infra/router-configs"); r == nil || r.Root != "/src/router-configs" {
		t.Fatal("the workspace folder is known")
	}
	d.Key("esc")
	if root.Depth() != 1 || root.Top() != ws {
		t.Fatal("esc returns to the workspace")
	}
	// p in the filter input is typed, not opened.
	d.Key("1", "/", "p")
	d.RequireNoView("Switch project")
	d.Key("esc")
}

func TestGlobalSearchDebounce(t *testing.T) {
	client := newFake()
	env, last := setup(t, client)
	d := uitest.New(t, app.New(env, start.NewNotOnInstance(env, start.NotOnInstanceInfo{Reason: "not here"}, nil), app.Options{}))
	client.ResetCalls()
	d.Key("F")
	o := *last
	d.Key("r", "o", "u")
	stale := o.DebounceMsg()
	d.Key("t", "e", "r")
	d.RequireView("infra/router-configs", "local")
	d.Send(stale)
	if client.CallCount("Search.Search") != 0 {
		t.Fatal("a stale debounce tick is ignored")
	}
	d.Send(o.DebounceMsg())
	calls := client.CallsOf("Search.Search")
	if len(calls) != 1 || calls[0][0] != "router" {
		t.Fatalf("calls = %v", calls)
	}
}

func TestGlobalMergesLocalAndServer(t *testing.T) {
	client := newFake()
	client.AddGroup(gitlab.Group{ID: 9, FullPath: "platform", Name: "platform"}, "platform/route-service", "platform/routing-engine")
	env, last := setup(t, client)
	env.Catalog.SetCurrent("platform/route-service", nil)
	root := app.New(env, start.NewNotOnInstance(env, start.NotOnInstanceInfo{Reason: "not here"}, nil), app.Options{})
	d := uitest.New(t, root)
	d.Key("F", "r", "o", "u", "t", "e")
	d.Send((*last).DebounceMsg())
	v := d.View()
	if strings.Count(v, "platform/route-service") != 1 {
		t.Fatalf("listed once:\n%s", v)
	}
	d.RequireView("PROJECTS", "logistics/route-api")
	if !strings.Contains(lineWith(d, "platform/route-service"), "local") {
		t.Fatal("local mark")
	}
	d.RequireGoldenSizes("global")

	// A group opens as a Workspace above the start screen.
	d.Key("backspace", "backspace", "backspace", "backspace", "backspace", "p", "l", "a")
	d.Send((*last).DebounceMsg())
	d.RequireView("GROUPS", "platform")
	for range 10 {
		d.Key("down")
	}
	d.Key("enter")
	ws, ok := root.Top().(*wsscreen.Screen)
	if !ok || root.Depth() != 2 {
		t.Fatalf("top = %T, depth %d", root.Top(), root.Depth())
	}
	d.RequireView("group: platform · workspace · 2 projects", "platform/routing-engine")
	if ws.Scope().Local() {
		t.Fatal("a group workspace has no folders")
	}
}

func TestGroupWorkspaceLimit(t *testing.T) {
	client := newFake()
	var many []string
	for i := range 342 {
		many = append(many, "platform/p"+string(rune('a'+i%26))+strings.Repeat("x", i/26))
	}
	client.AddGroup(gitlab.Group{ID: 9, FullPath: "platform"}, many...)
	env, _ := setup(t, client)
	d := uitest.New(t, app.New(env, wsscreen.NewGroup(env, gitlab.Group{ID: 9, FullPath: "platform"}), app.Options{}))
	d.RequireView("group: platform · workspace · 100 projects · showing 100 of 342")
	if args := client.CallsOf("Projects.ListGroupProjects"); len(args) != 1 || args[0][1] != 100 {
		t.Fatalf("calls = %v", args)
	}
}

func TestGoldenPicker(t *testing.T) {
	client := newFake()
	env, _ := setup(t, client)
	env.Catalog.SetCurrent("platform/route-service", repo("/src/route-service"))
	d := uitest.New(t, app.New(env, wsscreen.New(env, "~/src", wsScope()), app.Options{}))
	d.Key("p", "r", "o", "u")
	d.RequireGoldenSizes("picker")
}

func lineWith(d *uitest.Driver, sub string) string {
	for _, l := range strings.Split(d.View(), "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

// bigScope is a workspace of n projects named group/proj-NN.
func bigScope(n int) workspace.Scope {
	var rows []workspace.Project
	for i := range n {
		path := fmt.Sprintf("group/proj-%02d", i)
		r := repo("/src/" + path)
		rows = append(rows, workspace.Project{Ref: gitlab.ProjectRef{FullPath: path},
			Local: &gitremote.LocalProject{Dir: "/src/" + path, Name: path, Kind: gitremote.KindGitLab, Repo: r, ProjectPath: path}})
	}
	return workspace.NewScope("~/big", rows)
}

func TestHalfPageInOverlays(t *testing.T) {
	for _, global := range []bool{false, true} {
		client := newFake()
		for i := range 35 {
			client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: int64(100 + i), FullPath: fmt.Sprintf("group/proj-%02d", i)}})
		}
		env, last := setup(t, client)
		d := uitest.New(t, app.New(env, wsscreen.New(env, "~/big", bigScope(35)), app.Options{}))
		if global {
			d.Key("F", "p", "r", "o", "j")
			d.Send((*last).DebounceMsg())
			if client.CallCount("Search.Search") != 1 {
				t.Fatal("the server search ran")
			}
		} else {
			d.Key("p", "p", "r", "o", "j")
		}
		o := *last
		_ = d.View() // the list height comes from the last render
		if n := len(o.Lines()); n < 30 {
			t.Fatalf("global=%v: %d lines", global, n)
		}
		first := o.Selected()
		d.Key("ctrl+d")
		half := o.Selected()
		if half == first || o.Input() != "proj" {
			t.Fatalf("global=%v: ctrl+d %q → %q, input %q", global, first, half, o.Input())
		}
		// Half a page of the 18-line list: 9 lines down.
		if idx := index(o.Lines(), half) - index(o.Lines(), first); idx != 9 {
			t.Fatalf("global=%v: moved %d lines", global, idx)
		}
		d.Key("ctrl+u")
		if o.Selected() != first || o.Input() != "proj" {
			t.Fatalf("global=%v: ctrl+u → %q, input %q", global, o.Selected(), o.Input())
		}
		d.Key("pgdown")
		if o.Selected() != half {
			t.Fatalf("global=%v: pgdown → %q", global, o.Selected())
		}
		d.Key("pgup")
		if o.Selected() != first {
			t.Fatalf("global=%v: pgup → %q", global, o.Selected())
		}
		for range 10 {
			d.Key("ctrl+d")
		}
		if lines := o.Lines(); o.Selected() == "" || index(lines, o.Selected()) < len(lines)-2 {
			t.Fatalf("global=%v: ctrl+d stops at the end: %q", global, o.Selected())
		}
	}
}

func index(lines []string, s string) int {
	for i, l := range lines {
		if l == s {
			return i
		}
	}
	return -1
}

func TestHelpListsOverlayKeys(t *testing.T) {
	client := newFake()
	env, _ := setup(t, client)
	d := uitest.New(t, app.New(env, wsscreen.New(env, "~/src", wsScope()), app.Options{}))
	d.Key("?")
	d.RequireView("P AND F OVERLAYS", "ctrl+d/ctrl+u pgdown/pgup", "half a page down / up")
}
