package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitexec"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/localgit"
	"github.com/4aykovski/glview/internal/ui/localgit/gittest"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/uitest"
	"github.com/4aykovski/glview/internal/workspace"
)

// diskRepo writes a minimal .git for the prechecks: an origin remote, the
// branch and (with upstream) its branch.<name> section.
func diskRepo(t *testing.T, dir, url, branch string, upstream bool) *gitremote.Repo {
	t.Helper()
	cfg := "[remote \"origin\"]\n\turl = " + url + "\n"
	if upstream {
		cfg += "[branch \"" + branch + "\"]\n\tremote = origin\n\tmerge = refs/heads/" + branch + "\n"
	}
	head := "ref: refs/heads/" + branch + "\n"
	if branch == "" {
		head = "0b7d3e9a1c2b3d4e5f60718293a4b5c6d7e8f901\n"
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte(head), 0o644)
	r, err := gitremote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return &r
}

// gitScope is the five projects in real folders (route-planner-ui is
// detached, router-configs has no upstream) and a GitHub folder.
func gitScope(t *testing.T) (workspace.Scope, string) {
	root := t.TempDir()
	var rows []workspace.Project
	for _, p := range projects {
		dir := filepath.Join(root, p.folder)
		branch, up := p.branch, true
		switch p.folder {
		case "route-planner-ui":
			branch = ""
		case "router-configs":
			up = false
		}
		repo := diskRepo(t, dir, "git@gitlab.example.com:"+p.path+".git", branch, up)
		lp := &gitremote.LocalProject{Dir: dir, Name: p.folder, Kind: gitremote.KindGitLab, Repo: repo, ProjectPath: p.path}
		rows = append(rows, workspace.Project{Ref: gitlab.ProjectRef{FullPath: p.path}, Local: lp})
	}
	dir := filepath.Join(root, "dotfiles")
	repo := diskRepo(t, dir, "git@github.com:egor/dotfiles.git", "main", true)
	rows = append(rows, workspace.Project{Foreign: true, Local: &gitremote.LocalProject{Dir: dir, Name: "dotfiles", Kind: gitremote.KindForeign,
		Repo: repo, Remote: &gitremote.Remote{Host: "github.com", Path: "egor/dotfiles"}}})
	return workspace.NewScope(source, rows), root
}

func openGit(t *testing.T, r *gittest.Runner) (*uitest.Driver, *app.Model, *Screen, string) {
	t.Helper()
	scope, root := gitScope(t)
	env := newEnv(newFake())
	env.Git = localgit.New(r)
	scr := New(env, source, scope)
	m := app.New(env, scr, app.Options{})
	return uitest.New(t, m), m, scr, root
}

func TestFetchAllFolders(t *testing.T) {
	r := gittest.New()
	d, _, _, root := openGit(t, r)
	r.AB[filepath.Join(root, "route-service")] = [2]int{1, 3}
	r.Results[filepath.Join(root, "router-configs")] = gitexec.Result{Status: gitexec.StatusFailed, Reason: gitexec.ReasonAuth,
		Output: []string{"git@gitlab.example.com: Permission denied (publickey)."}}
	if r.AheadBehindCalls() != 6 {
		t.Fatalf("ahead/behind on open: %d", r.AheadBehindCalls())
	}
	d.RequireView("u fetch")
	d.Key("u")
	if n := len(r.Runs()); n != 6 {
		t.Fatalf("fetch runs in every folder, foreign too: %v", r.Runs())
	}
	d.RequireView("fetch: 5 ✓, 1 ✗ (router-configs: needs authentication)")
	if !strings.Contains(lineWith(t, d, "platform/route-service"), "↓3 ↑1") {
		t.Fatalf("↑↓ cell:\n%s", d.View())
	}
	if !strings.Contains(lineWith(t, d, "infra/router-configs"), "✗") {
		t.Fatal("failed cell")
	}
	d.Key("G", "k") // router-configs is above dotfiles, below the more active ones
	for !strings.HasPrefix(lineWith(t, d, "infra/router-configs"), "›") {
		d.Key("k")
	}
	d.RequireView("✗ fetch: needs authentication — run git fetch in", "Permission denied (publickey).")
}

func TestPullConfirmAndBusy(t *testing.T) {
	r := gittest.New()
	d, _, _, root := openGit(t, r)
	d.Key("U")
	d.RequireView("Pull (fast-forward only)?", "update 4 repositories, skip 2 (", "detached", "no upstream")
	d.Key("y")
	runs := r.Runs()
	if len(runs) != 6 || !strings.HasPrefix(runs[0], "pull:") {
		t.Fatalf("runs = %v", runs)
	}

	r.Block[filepath.Join(root, "dotfiles")] = true
	d.Key("u")
	d.RequireView("⟳ fetch")
	d.Key("u")
	d.RequireView("fetch is already running")
}

func TestProgressSurvivesNavigation(t *testing.T) {
	r := gittest.New()
	d, m, _, root := openGit(t, r)
	r.Block[filepath.Join(root, "dotfiles")] = true
	d.Key("u")
	d.RequireView("⟳ fetch 5/6")
	d.Key("enter")
	if m.Depth() != 2 {
		t.Fatal("project opened")
	}
	d.RequireView("⟳ fetch 5/6", "~/src › platform/route-service")
	if _, ok := m.Top().(*project.Project); !ok {
		t.Fatalf("top = %T", m.Top())
	}
}

func TestNoGitHidesKeys(t *testing.T) {
	scope, _ := gitScope(t)
	env := newEnv(newFake())
	env.Git = localgit.New(nil)
	d := uitest.New(t, app.New(env, New(env, source, scope), app.Options{}))
	d.RequireNoView("u fetch")
	d.Key("?")
	d.RequireNoView("pull ff-only")
}

func TestProjectModeFetchPull(t *testing.T) {
	r := gittest.New()
	dir := filepath.Join(t.TempDir(), "route-service")
	repo := diskRepo(t, dir, "git@gitlab.example.com:platform/route-service.git", "main", true)
	r.AB[dir] = [2]int{0, 3}
	r.OnRun = func(op gitexec.Op, d string) {
		if op == gitexec.OpPullFF { // a pull moved the branch elsewhere
			_ = os.WriteFile(filepath.Join(d, ".git", "HEAD"), []byte("ref: refs/heads/feat/after-pull\n"), 0o644)
		}
	}
	env := newEnv(newFake())
	env.Git = localgit.New(r)
	d := uitest.New(t, app.New(env, project.New(env, project.Options{Path: "platform/route-service", Repo: repo}), app.Options{}))
	d.RequireView("⎇ main ↓3")
	d.Key("?")
	d.RequireView("fetch", "pull ff-only")
	d.Key("esc", "U")
	d.RequireView("update 1 repository")
	d.Key("y")
	d.RequireView("⎇ feat/after-pull", "pull: 1 ✓")

	// Without a local folder u/U are hidden.
	env2 := newEnv(newFake())
	env2.Git = localgit.New(r)
	d2 := uitest.New(t, app.New(env2, project.New(env2, project.Options{Path: "platform/route-service"}), app.Options{}))
	d2.Key("?")
	d2.RequireNoView("pull ff-only")
}

func TestFetchOnStartAndGroupWithoutGit(t *testing.T) {
	r := gittest.New()
	scope, _ := gitScope(t)
	env := newEnv(newFake())
	env.Git = localgit.New(r)
	env.Settings.FetchOnStart = true
	uitest.New(t, app.New(env, New(env, source, scope), app.Options{}))
	if n := len(r.Runs()); n != 6 || !strings.HasPrefix(r.Runs()[0], "fetch:") {
		t.Fatalf("fetch on start: %v", r.Runs())
	}

	client := newFake()
	client.AddGroup(gitlab.Group{ID: 9, FullPath: "platform"}, "platform/route-service")
	env2 := newEnv(client)
	env2.Git = localgit.New(gittest.New())
	d := uitest.New(t, app.New(env2, NewGroup(env2, gitlab.Group{ID: 9, FullPath: "platform"}), app.Options{}))
	d.RequireView("group: platform · workspace · 1 project", "platform/route-service")
	d.Key("?")
	d.RequireNoView("pull ff-only")
}
