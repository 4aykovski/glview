package workspace

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

// n on the header of a collapsed group opens the New MR form of the group's
// project, with the branch of its local folder.
func TestMRsNewMRFromGroupHeader(t *testing.T) {
	client := newFake()
	client.SetBranches("platform/route-service", "main", "fix/slot-race")
	d, _, scr := open(t, client, testScope())
	d.Key("2")
	tab := scr.tabs[TabMRs].(*mrsTab)
	path := tab.cursorGroup()
	if path == "" {
		t.Fatal("no group under the cursor")
	}
	d.Key("?")
	d.RequireView("new MR")
	d.Key("esc", "n")
	d.RequireView(path+" › MRs › new MR", "From branch", "Into branch")
	var want string
	for _, p := range projects {
		if p.path == path {
			want = p.branch
		}
	}
	if want != "main" { // a folder on the default branch proposes nothing
		d.RequireView(want)
	}
	d.Key("esc")
	d.RequireNoView("From branch")
	d.RequireView("MRs")
}

func TestMRsNewMRFromAnyLineOfTheGroup(t *testing.T) {
	client := newFake()
	d, _, scr := openExpanded(t, client, testScope(), []string{"platform/route-service"}, nil)
	d.Key("2")
	tab := scr.tabs[TabMRs].(*mrsTab)
	d.Key("j") // the first MR line
	if tab.selected() == nil {
		t.Fatal("the cursor should stand on an MR")
	}
	path := tab.cursorGroup()
	d.Key("n")
	d.RequireView(path + " › MRs › new MR")
}

func TestMRsNewMRArchivedProject(t *testing.T) {
	client := newFake()
	for _, p := range projects {
		client.AddProject(gitlab.Project{
			ProjectRef: gitlab.ProjectRef{ID: p.id, FullPath: p.path}, Name: p.folder, DefaultBranch: "main",
			LastActivityAt: now.Add(-p.ago), WebURL: "https://gitlab.example.com/" + p.path, Archived: true,
		})
	}
	d, _, _ := open(t, client, testScope())
	d.Key("2", "?")
	d.RequireNoView("new MR")
	d.Key("esc", "n")
	d.RequireNoView("From branch")
}
