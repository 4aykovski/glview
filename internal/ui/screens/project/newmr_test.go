package project

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
)

func TestMRsNewMRFromTheTab(t *testing.T) {
	client := newFake(3)
	client.SetBranches(path, "main", "feat/1", "feat/2", "feat/3")
	repo := &gitremote.Repo{Root: "/src/route-service", Branch: "feat/2"}
	d, _, scr := open(t, client, repo)
	d.Key("?")
	d.RequireView("n", "new MR")
	d.Key("esc", "j")
	cursor := scr.tabs[TabMRs].(*mrsTab).tbl.CursorID()
	d.Key("n")
	d.RequireView(path+" › MRs › new MR", "feat/2")
	d.Key("esc")
	d.RequireNoView("From branch")
	d.RequireView("MR number 1")
	if got := scr.tabs[TabMRs].(*mrsTab).tbl.CursorID(); got != cursor {
		t.Fatalf("cursor = %q, want %q", got, cursor)
	}
}

func TestMRsNewMRArchivedProject(t *testing.T) {
	client := newFake(2)
	client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, WebURL: "https://gitlab.example.com/" + path, Archived: true})
	d, _, _ := open(t, client, nil)
	d.Key("?")
	d.RequireNoView("new MR")
	d.Key("esc", "n")
	d.RequireNoView("From branch")
}
