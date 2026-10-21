package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestWorkspaceSnapshot(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	d := Disk{Dir: t.TempDir(), now: clk.now}
	rows := []gitlab.ProjectOverview{{
		Project: gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: "platform/route-service"}, DefaultBranch: "main"},
		OpenMRs: 6, ReviewMRs: 2, LastTag: "v1.14.2", TagKnown: true, Phase: 2,
		DefaultPipeline: &gitlab.PipelineSummary{ID: 91790, Status: gitlab.StatusManual, User: &gitlab.User{Username: "anna.k"}, CommitTitle: "Merge branch"},
	}}
	if err := d.SaveWorkspace("https://gitlab.example.com", "~/src", rows); err != nil {
		t.Fatal(err)
	}
	got, at, ok := d.LoadWorkspace("https://gitlab.example.com", "~/src")
	if !ok || !at.Equal(clk.t) || len(got.Projects) != 1 {
		t.Fatalf("load = %+v %v %v", got, at, ok)
	}
	p := got.Projects[0]
	if p.DefaultBranch != "main" || p.OpenMRs != 6 || p.DefaultPipeline.Status != gitlab.StatusManual || p.DefaultPipeline.User != nil || p.DefaultPipeline.CommitTitle != "" {
		t.Fatalf("row = %+v / %+v", p, p.DefaultPipeline)
	}
	if _, _, ok := d.LoadWorkspace("https://gitlab.example.com", "group: platform"); ok {
		t.Fatal("another source has its own snapshot")
	}
	files, _ := filepath.Glob(filepath.Join(d.Dir, "workspace", "*.json"))
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	for _, bad := range []string{"anna.k", "token", "Merge branch"} {
		if strings.Contains(string(raw), bad) {
			t.Fatalf("snapshot contains %q", bad)
		}
	}
	if err := os.WriteFile(files[0], []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _, ok := d.LoadWorkspace("https://gitlab.example.com", "~/src"); ok || len(got.Projects) != 0 {
		t.Fatal("a corrupt snapshot is a miss")
	}
}

func TestStarredSnapshot(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	d := Disk{Dir: t.TempDir(), now: clk.now}
	if _, _, ok := d.LoadStarred(hostA); ok {
		t.Fatal("no snapshot yet")
	}
	_ = d.SaveStarred(hostA, []gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "qa/e2e-routes"}, Starred: true}})
	ps, at, ok := d.LoadStarred(hostA)
	if !ok || len(ps) != 1 || !d.Fresh(at, StarredMaxAge) {
		t.Fatalf("starred = %+v", ps)
	}
	clk.add(2 * time.Hour)
	if d.Fresh(at, StarredMaxAge) {
		t.Fatal("older than an hour")
	}
}

func TestStarredPerHost(t *testing.T) {
	d := Disk{Dir: t.TempDir()}
	_ = d.SaveStarred(hostA, []gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "platform/route-service"}}})
	_ = d.SaveStarred(hostB, []gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "qa/e2e-routes"}}, {ProjectRef: gitlab.ProjectRef{FullPath: "infra/router-configs"}}})
	if ps, _, ok := d.LoadStarred(hostA); !ok || len(ps) != 1 || ps[0].FullPath != "platform/route-service" {
		t.Fatalf("host A = %+v", ps)
	}
	if ps, _, ok := d.LoadStarred(hostB); !ok || len(ps) != 2 {
		t.Fatalf("host B = %+v", ps)
	}
	if _, _, ok := d.LoadStarred("https://gitlab.example.net"); ok {
		t.Fatal("another host sees no snapshot")
	}
	if !strings.HasPrefix(starredName(hostA), filepath.Join("starred", "")) || starredName(hostA) == starredName(hostB) {
		t.Fatalf("names = %s %s", starredName(hostA), starredName(hostB))
	}
}

func TestStarredOldFileIsIgnored(t *testing.T) {
	d := Disk{Dir: t.TempDir()}
	// The host-less file of the old format.
	if err := d.Save("starred.json", []gitlab.Project{{ProjectRef: gitlab.ProjectRef{FullPath: "platform/route-service"}}}); err != nil {
		t.Fatal(err)
	}
	if ps, _, ok := d.LoadStarred(hostA); ok || len(ps) != 0 {
		t.Fatalf("old snapshot = %+v", ps)
	}
}
