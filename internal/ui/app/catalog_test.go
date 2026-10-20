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
	_ = disk.SaveStarred([]gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "platform/routing-engine"}, LastActivityAt: time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC)}})
	statePath := filepath.Join(dir, "state.json")
	_ = cache.SaveState(statePath, cache.State{Recent: []string{"qa/e2e-routes", "infra/router-configs"}})

	c := NewCatalog(disk, statePath)
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
	if got := cache.LoadState(statePath); got.Recent[0] != "infra/router-configs" || len(got.Recent) != 2 {
		t.Fatalf("state = %v", got.Recent)
	}
	var nilCat *Catalog
	if nilCat.Current() != nil || nilCat.Workspace() != nil || nilCat.Touch("x") != nil {
		t.Fatal("a nil catalog is empty")
	}
}

func TestRootRegistersWorkspaceAndRefreshesStarred(t *testing.T) {
	client := fake.New()
	client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{FullPath: "qa/e2e-routes"}})
	client.SetStarred("qa/e2e-routes")
	disk := &cache.Disk{Dir: t.TempDir()}

	// Fresh snapshot: no request.
	_ = disk.SaveStarred([]gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "platform/routing-engine"}}})
	env := NewEnv(i18n.EN)
	env.Client, env.Catalog = client, NewCatalog(disk, "")
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
	env2.Client, env2.Catalog = client, NewCatalog(disk2, "")
	uitest.New(t, New(env2, newDummy(env2, "p"), Options{}))
	if client.CallCount("Projects.Starred") != 1 {
		t.Fatalf("starred calls = %d", client.CallCount("Projects.Starred"))
	}
	if st := env2.Catalog.Starred(); len(st) != 1 || st[0].Path != "qa/e2e-routes" {
		t.Fatalf("starred = %+v", st)
	}
	if ps, _, ok := disk2.LoadStarred(); !ok || len(ps) != 1 {
		t.Fatal("snapshot saved")
	}
	if env2.Catalog.Workspace() != nil {
		t.Fatal("no workspace root")
	}
}
