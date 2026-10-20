package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/gitremote"
)

func mkRepo(t *testing.T, dir, url string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[remote \"origin\"]\n\turl = " + url + "\n"
	_ = os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
}

func TestLocalDirSourceDeduplicates(t *testing.T) {
	dir := t.TempDir()
	mkRepo(t, filepath.Join(dir, "route-service"), "git@gitlab.example.com:platform/route-service.git")
	mkRepo(t, filepath.Join(dir, "route-service-2"), "https://gitlab.example.com/platform/route-service.git")
	mkRepo(t, filepath.Join(dir, "dotfiles"), "git@github.com:egor/dotfiles.git")
	_ = os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	inst, err := gitremote.NewInstance("https://gitlab.example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	src := LocalDirSource{Dir: dir, Instance: inst}
	ps, err := src.Projects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s := NewScope(src.Name(), ps)
	if len(s.Rows) != 3 || len(s.Refs()) != 1 || s.GitLabCount() != 1 {
		t.Fatalf("rows = %d refs = %v", len(s.Rows), s.Refs())
	}
	if s.Refs()[0].FullPath != "platform/route-service" {
		t.Fatalf("ref = %+v", s.Refs()[0])
	}
	if p := s.Primary("platform/route-service"); p == nil || p.Name != "route-service" {
		t.Fatalf("primary = %+v", p)
	}
	if len(s.Skipped) != 1 || s.Skipped[0] != "notes" {
		t.Fatalf("skipped = %v", s.Skipped)
	}
	if len(s.Repos()) != 3 || !s.Local() {
		t.Fatalf("repos = %d", len(s.Repos()))
	}
	for _, r := range s.Rows {
		if r.Local.Name == "dotfiles" && !r.Foreign {
			t.Fatal("foreign remote must be grey")
		}
	}
}

func TestGroupSourceLimit(t *testing.T) {
	c := fake.New()
	var paths []string
	for i := range 342 {
		paths = append(paths, fmt.Sprintf("platform/p%03d", i))
	}
	c.AddGroup(gitlab.Group{ID: 9, FullPath: "platform"}, paths...)
	src := GroupSource{Group: gitlab.Group{ID: 9, FullPath: "platform"}, Limit: 100, Client: c}
	ps, total, err := src.ProjectsTotal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 100 || total != 342 || src.Name() != "group: platform" {
		t.Fatalf("len = %d total = %d", len(ps), total)
	}
	s := NewScope(src.Name(), ps)
	s.Total = total
	if s.GitLabCount() != 100 || s.Local() || len(s.Repos()) != 0 {
		t.Fatal("group scope has no local folders")
	}
}
