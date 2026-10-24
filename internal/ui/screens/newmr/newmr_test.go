package newmr

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const path = "platform/route-service"

var project = gitlab.ProjectRef{ID: 42, FullPath: path}

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

type host struct{}

func (host) Init() tea.Cmd                          { return nil }
func (h host) Update(tea.Msg) (app.Screen, tea.Cmd) { return h, nil }
func (host) View() string                           { return "host" }
func (host) Title() string                          { return "host" }
func (host) KeyMap() common.KeyMap                  { return common.KeyMap{} }
func (host) SetSize(int, int)                       {}

func commit(sha, title string) gitlab.CommitSummary {
	return gitlab.CommitSummary{SHA: sha + "ffff", ShortSHA: sha, Title: title}
}

// newFake is a project with main and four feature branches, feat/surge
// three commits in five files ahead of main, two templates, members, labels
// and milestones.
func newFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: project, DefaultBranch: "main", RemoveSourceBranchAfterMerge: true, SquashOption: "default_off"})
	c.SetBranches(path, "main", "feat/surge", "feat/slots", "fix/race", "feat/a", "feat/b")
	c.SetCompare(path, "main", "feat/surge", gitlab.Comparison{Files: 5, Commits: []gitlab.CommitSummary{
		commit("a91f2c0", "surge: add pricing"), commit("5c0e7d1", "slots: retry"), commit("e41ab08", "deps: bump grpc")}})
	c.SetCompare(path, "main", "fix/race", gitlab.Comparison{Files: 1, Commits: []gitlab.CommitSummary{commit("77d0c13", "Fix race in courier slot allocator")}})
	c.SetCompare(path, "main", "feat/a", gitlab.Comparison{Files: 2, Commits: []gitlab.CommitSummary{commit("1a1a1a1", "a one"), commit("1a1a1a2", "a two")}})
	c.SetCompare(path, "main", "feat/b", gitlab.Comparison{Files: 4, Commits: []gitlab.CommitSummary{commit("1b1b1b1", "b one"), commit("1b1b1b2", "b two")}})
	c.SetMembers(path, c.User, gitlab.User{ID: 3, Username: "anna.k", Name: "Anna K"}, gitlab.User{ID: 4, Username: "d.orlov", Name: "D Orlov"})
	c.SetLabels(path, "bug", "pricing", "ux")
	c.SetMilestones(path, "2026-Q4", "2026-Q3")
	c.SetMRTemplates(path, "Default", "## What\n\n## Why\n", "Bug", "## Steps\n")
	return c
}

func repoOn(branch string) *gitremote.Repo {
	return &gitremote.Repo{Root: "/work/route-service", Branch: branch}
}

type opened struct {
	t   *testing.T
	d   *uitest.Driver
	env *app.Env
	f   *Form
	c   *fake.Client
}

// open pushes the form over a host screen, the local folder sitting on
// branch ("" = no folder).
func open(t *testing.T, c *fake.Client, lang i18n.Lang, branch string) *opened {
	t.Helper()
	env := app.NewEnv(lang)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	root := app.New(env, host{}, app.Options{})
	d := uitest.New(t, root)
	var repo *gitremote.Repo
	if branch != "" {
		repo = repoOn(branch)
	}
	f := New(env, Options{Project: project, Repo: repo, DefaultBranch: "main"})
	d.Send(app.PushMsg{Screen: f})
	return &opened{t: t, d: d, env: env, f: f, c: c}
}

func (o *opened) typeText(s string) {
	for _, r := range s {
		o.d.Key(string(r))
	}
}

// tick delivers the pause after an edit.
func (o *opened) tick() { o.d.Send(tickMsg{tok: o.f.tickGen.Current()}) }

// focusOn moves the focus to id with tab.
func (o *opened) focusOn(id fieldID) {
	o.t.Helper()
	for i := 0; o.f.Focus() != id; i++ {
		if i > int(fieldCount) {
			o.t.Fatalf("no way to focus %d", id)
		}
		o.d.Key("tab")
	}
}

func (o *opened) creates() [][]any {
	var out [][]any
	for _, c := range o.c.Calls() {
		if c.Method == "MergeRequests.Create" {
			out = append(out, c.Args)
		}
	}
	return out
}

func TestOpenInProjectMode(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	if src, dst := o.f.Branches(); src != "feat/surge" || dst != "main" {
		t.Fatalf("branches = %q → %q", src, dst)
	}
	if o.f.Focus() != fSrc {
		t.Fatalf("focus = %d, want the source branch", o.f.Focus())
	}
	o.d.RequireView("platform/route-service › MRs › new MR", "From branch", "Into branch", "enter Create")
}

func TestOpenWithoutProposalOnDefaultBranchOrDetachedHEAD(t *testing.T) {
	for name, repo := range map[string]*gitremote.Repo{
		"default branch": repoOn("main"),
		"detached":       {Root: "/work/route-service", Branch: "0b7d3e9a", Detached: true},
		"no folder":      nil,
	} {
		o := open(t, newFake(), i18n.EN, "")
		f := New(o.env, Options{Project: project, Repo: repo, DefaultBranch: "main"})
		if src, dst := f.Branches(); src != "" || dst != "main" {
			t.Errorf("%s: branches = %q → %q", name, src, dst)
		}
	}
}

func TestDefaultBranchComesFromTheProjectWhenUnknown(t *testing.T) {
	c := newFake()
	o := open(t, c, i18n.EN, "")
	f := New(o.env, Options{Project: project, Repo: repoOn("main")})
	o.d.Send(app.PopMsg{})
	o.d.Send(app.PushMsg{Screen: f})
	if src, dst := f.Branches(); src != "" || dst != "main" {
		t.Fatalf("branches = %q → %q, want the folder on main dropped and main as the target", src, dst)
	}
}

func TestGlobalShortcutsAreOff(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.focusOn(fTitle)
	o.d.Key("ctrl+a", "ctrl+k")
	o.typeText(" T p q ? r F")
	if !strings.Contains(o.f.TitleText(), "T p q ? r F") {
		t.Fatalf("title = %q", o.f.TitleText())
	}
	if o.d.Model.(*app.Model).Overlay() != nil || o.d.Model.(*app.Model).Depth() != 2 {
		t.Fatal("a global shortcut acted")
	}
}

func TestFitsTerminal(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		o := open(t, newFake(), lang, "feat/surge")
		o.d.RequireFitsSizes()
		for _, id := range []fieldID{fTemplate, fAssignees, fMilestone, fSquash, fButton} {
			o.focusOn(id)
			o.d.RequireFitsSizes()
		}
		o.d.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
		o.focusOn(fMilestone)
		o.d.RequireView("Milestone")
		o.d.Key("tab", "tab", "tab")
		o.d.RequireFitsSizes()
	}
}
