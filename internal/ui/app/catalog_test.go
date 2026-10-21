package app

import (
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

// wsRoot is a workspace-like root screen for the catalog.
type wsRoot struct {
	*dummy
	entries []CatalogEntry
	repos   map[string]*gitremote.Repo
}

func (w *wsRoot) Update(msg tea.Msg) (Screen, tea.Cmd) {
	_, cmd := w.dummy.Update(msg)
	return w, cmd
}
func (w *wsRoot) CatalogEntries() []CatalogEntry        { return w.entries }
func (w *wsRoot) LocalRepo(path string) *gitremote.Repo { return w.repos[path] }

func TestCatalogSources(t *testing.T) {
	dir := t.TempDir()
	disk := &cache.Disk{Dir: dir}
	_ = disk.SaveStarred(testHost, []gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "platform/routing-engine"}, LastActivityAt: time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC)}})
	statePath := filepath.Join(dir, "state.json")
	var st cache.State
	st.Touch(testHost, "infra/router-configs")
	st.Touch(testHost, "qa/e2e-routes")
	_ = cache.SaveState(statePath, st)

	c := NewCatalog(disk, statePath, func() string { return testHost })
	repo := &gitremote.Repo{Root: "/src/route-service"}
	c.SetCurrent("platform/route-service", repo)
	c.Note(CatalogEntry{Path: "qa/e2e-routes", OpenMRs: 0, Activity: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)})

	if cur := c.Current(); cur == nil || !cur.Current || cur.Path != "platform/route-service" {
		t.Fatalf("current = %+v", cur)
	}
	if st := c.Starred(); len(st) != 1 || !st[0].Starred || st[0].Activity.IsZero() {
		t.Fatalf("starred = %+v", st)
	}
	rec := c.Recent()
	if len(rec) != 2 || rec[0].Path != "qa/e2e-routes" || rec[0].OpenMRs != 0 || rec[1].OpenMRs != -1 {
		t.Fatalf("recent = %+v", rec)
	}
	if c.LocalRepo("platform/route-service") != repo || c.LocalRepo("qa/e2e-routes") != nil {
		t.Fatal("local repo of the current project")
	}

	env := NewEnv(i18n.EN)
	ws := &wsRoot{dummy: newDummy(env, "ws"), entries: []CatalogEntry{{Path: "infra/router-configs", OpenMRs: 1}},
		repos: map[string]*gitremote.Repo{"infra/router-configs": {Root: "/src/router-configs"}}}
	c.SetWorkspace(ws)
	if w := c.Workspace(); len(w) != 1 || c.LocalRepo("infra/router-configs") == nil {
		t.Fatalf("workspace = %+v", w)
	}

	if cmd := c.Touch("infra/router-configs"); cmd != nil {
		cmd()
	}
	if got := cache.LoadState(statePath).Recent(testHost); got[0] != "infra/router-configs" || len(got) != 2 {
		t.Fatalf("state = %v", got)
	}
	var nilCat *Catalog
	if nilCat.Current() != nil || nilCat.Workspace() != nil || nilCat.Touch("x") != nil {
		t.Fatal("a nil catalog is empty")
	}
}

const testHost = "https://gitlab.example.com"

func TestCatalogPerHost(t *testing.T) {
	dir := t.TempDir()
	disk := &cache.Disk{Dir: dir}
	statePath := filepath.Join(dir, "state.json")
	host := testHost
	c := NewCatalog(disk, statePath, func() string { return host })
	_ = disk.SaveStarred(testHost, []gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "platform/routing-engine"}}})
	if cmd := c.Touch("platform/route-service"); cmd != nil {
		cmd()
	}
	if len(c.Starred()) != 1 || len(c.Recent()) != 1 {
		t.Fatalf("host A: starred %+v recent %+v", c.Starred(), c.Recent())
	}
	host = "https://gitlab.example.org"
	if len(c.Starred()) != 0 || len(c.Recent()) != 0 {
		t.Fatalf("host B sees host A: starred %+v recent %+v", c.Starred(), c.Recent())
	}
	if !c.StarredStale() {
		t.Fatal("host B has no starred snapshot")
	}
	if cmd := c.Touch("qa/e2e-routes"); cmd != nil {
		cmd()
	}
	st := cache.LoadState(statePath)
	if a, b := st.Recent(testHost), st.Recent(host); len(a) != 1 || a[0] != "platform/route-service" || len(b) != 1 || b[0] != "qa/e2e-routes" {
		t.Fatalf("state = %+v", st)
	}
}

func TestCatalogExpandedGroups(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	host := testHost
	c := NewCatalog(nil, statePath, func() string { return host })
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return at }
	if c.OpenWorkspace("~/src") != nil {
		t.Fatal("a workspace without a record writes nothing")
	}
	first := c.SetExpanded("~/src", "mrs", []string{"platform/route-service"})
	second := c.SetExpanded("~/src", "mrs", []string{"platform/route-service", "qa/e2e-routes"})
	// The saves run in the background in any order: the newer state wins.
	second()
	first()
	if got := cache.LoadState(statePath).Expanded(testHost, "~/src", "mrs"); len(got) != 2 {
		t.Fatalf("an older save overwrote a newer one: %v", got)
	}
	if got := c.Expanded("~/src", "mrs"); len(got) != 2 || len(c.Expanded("~/src", "pipelines")) != 0 {
		t.Fatalf("expanded = %v", got)
	}
	at = at.Add(time.Hour)
	if cmd := c.OpenWorkspace("~/src"); cmd == nil {
		t.Fatal("a known workspace is marked opened")
	} else {
		cmd()
	}
	if ws := cache.LoadState(statePath).Hosts[testHost].Workspaces["~/src"]; !ws.At.Equal(at) {
		t.Fatalf("at = %v", ws.At)
	}
	host = "https://gitlab.example.org"
	if len(c.Expanded("~/src", "mrs")) != 0 {
		t.Fatal("another host has its own groups")
	}
	var nilCat *Catalog
	if nilCat.Expanded("~/src", "mrs") != nil || nilCat.SetExpanded("~/src", "mrs", nil) != nil || nilCat.OpenWorkspace("~/src") != nil {
		t.Fatal("a nil catalog keeps nothing")
	}
}

func TestRootRegistersWorkspaceAndRefreshesStarred(t *testing.T) {
	client := fake.New()
	client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{FullPath: "qa/e2e-routes"}})
	client.SetStarred("qa/e2e-routes")
	disk := &cache.Disk{Dir: t.TempDir()}

	// Fresh snapshot: no request.
	_ = disk.SaveStarred(testHost, []gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "platform/routing-engine"}}})
	env := NewEnv(i18n.EN)
	env.Host = testHost
	env.Client, env.Catalog = client, NewCatalog(disk, "", func() string { return env.Host })
	ws := &wsRoot{dummy: newDummy(env, "ws"), entries: []CatalogEntry{{Path: "a/b"}}}
	uitest.New(t, New(env, ws, Options{}))
	if client.CallCount("Projects.Starred") != 0 {
		t.Fatal("a fresh starred snapshot must not be refreshed")
	}
	if len(env.Catalog.Workspace()) != 1 {
		t.Fatal("the workspace root is registered in the catalog")
	}

	// Stale snapshot: one background request, saved to disk.
	disk2 := &cache.Disk{Dir: t.TempDir()}
	env2 := NewEnv(i18n.EN)
	env2.Host = testHost
	env2.Client, env2.Catalog = client, NewCatalog(disk2, "", func() string { return env2.Host })
	uitest.New(t, New(env2, newDummy(env2, "p"), Options{}))
	if client.CallCount("Projects.Starred") != 1 {
		t.Fatalf("starred calls = %d", client.CallCount("Projects.Starred"))
	}
	if st := env2.Catalog.Starred(); len(st) != 1 || st[0].Path != "qa/e2e-routes" {
		t.Fatalf("starred = %+v", st)
	}
	if ps, _, ok := disk2.LoadStarred(testHost); !ok || len(ps) != 1 {
		t.Fatal("snapshot saved")
	}
	if env2.Catalog.Workspace() != nil {
		t.Fatal("no workspace root")
	}
}
