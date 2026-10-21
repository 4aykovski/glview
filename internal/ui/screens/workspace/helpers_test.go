package workspace

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/uitest"
	"github.com/4aykovski/glview/internal/workspace"
)

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 60 * time.Millisecond
	os.Exit(m.Run())
}

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

const source = "~/src"

func newEnv(client *fake.Client) *app.Env {
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	env.Browser = func(string) error { return nil }
	return env
}

// project is one GitLab project of the test workspace.
type testProject struct {
	path, folder, branch string
	ago                  time.Duration
	id                   int64
}

var projects = []testProject{
	{"qa/e2e-routes", "e2e-routes", "main", 14 * 24 * time.Hour, 5},
	{"platform/route-service", "route-service", "fix/slot-race", time.Hour, 42},
	{"infra/router-configs", "router-configs", "main", 7 * 24 * time.Hour, 4},
	{"platform/routing-engine", "routing-engine", "main", 3 * time.Hour, 2},
	{"frontend/route-planner-ui", "route-planner-ui", "feat/map-layers", 4 * time.Hour, 3},
}

// newFake registers the five projects with MRs, pipelines and tags.
func newFake() *fake.Client {
	c := fake.New()
	for _, p := range projects {
		c.AddProject(gitlab.Project{
			ProjectRef: gitlab.ProjectRef{ID: p.id, FullPath: p.path}, Name: p.folder, DefaultBranch: "main",
			LastActivityAt: now.Add(-p.ago), WebURL: "https://gitlab.example.com/" + p.path,
		})
	}
	me := gitlab.User{Username: "egor"}
	mr := func(path string, iid int64, title, author, branch string, reviewMe bool, st gitlab.PipelineStatus, ago time.Duration) {
		m := gitlab.MergeRequest{IID: iid, Title: title, State: gitlab.MRStateOpened, Author: gitlab.User{Username: author},
			SourceBranch: branch, TargetBranch: "main", UpdatedAt: now.Add(-ago), MergeStatus: "mergeable",
			Approvals: gitlab.Approvals{Required: 2}, WebURL: "https://gitlab.example.com/" + path + "/-/merge_requests/" + itoa(iid)}
		if reviewMe {
			m.Reviewers = []gitlab.User{me}
		}
		if st != "" {
			m.HeadPipeline = &gitlab.PipelineSummary{ID: 91000 + iid, Status: st}
		}
		c.AddMR(path, m)
	}
	mr("platform/route-service", 482, "Draft: add surge pricing to route estimator", "anna.k", "feat/surge", true, gitlab.StatusRunning, 4*time.Minute)
	mr("platform/route-service", 479, "Fix race in courier slot allocator", "egor", "fix/slot-race", false, gitlab.StatusSuccess, time.Hour)
	mr("platform/route-service", 477, "Kafka consumer: batch offset commits", "bob", "feat/kafka", true, gitlab.StatusFailed, 3*time.Hour)
	mr("platform/route-service", 474, "Extract geo client into pkg/geo", "bob", "feat/geo", false, "", 5*time.Hour)
	mr("frontend/route-planner-ui", 311, "Map layers: courier heatmap toggle", "k.lee", "feat/map-layers", false, gitlab.StatusRunning, 12*time.Minute)
	mr("platform/routing-engine", 88, "Contraction hierarchies: rebuild on schedule", "maria.s", "feat/ch", true, gitlab.StatusSuccess, 3*time.Hour)
	pipe := func(path string, id int64, st gitlab.PipelineStatus, ref string, ago time.Duration) {
		c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{
			ID: id, IID: id % 1000, Status: st, Ref: ref, SHA: "0b7d3e9aaa", Source: "push", CommitTitle: "Merge branch",
			User: &gitlab.User{Username: "anna.k"}, Duration: 351 * time.Second, CreatedAt: now.Add(-ago),
			WebURL: "https://gitlab.example.com/" + path + "/-/pipelines/" + itoa(id),
			Stages: []gitlab.StageSummary{{Name: "build", Status: gitlab.StatusSuccess, Done: 1, Total: 1}, {Name: "deploy", Status: st, Total: 1}},
		}})
	}
	pipe("platform/route-service", 91790, gitlab.StatusManual, "main", 5*time.Hour)
	pipe("platform/route-service", 91822, gitlab.StatusSuccess, "fix/slot-race", time.Hour)
	pipe("platform/routing-engine", 40098, gitlab.StatusFailed, "main", 9*time.Hour)
	pipe("frontend/route-planner-ui", 7001, gitlab.StatusRunning, "main", 3*time.Minute)
	pipe("infra/router-configs", 7741, gitlab.StatusSuccess, "main", 7*24*time.Hour)
	pipe("qa/e2e-routes", 5501, gitlab.StatusSuccess, "main", 14*24*time.Hour)
	c.SetTags("platform/route-service", gitlab.Tag{Name: "v1.14.2"})
	c.SetTags("platform/routing-engine", gitlab.Tag{Name: "v0.9.0"})
	c.SetTags("frontend/route-planner-ui", gitlab.Tag{Name: "v2.3.1"})
	return c
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// localRow is a folder of the workspace.
func localRow(dir, name, path, branch string) workspace.Project {
	repo := &gitremote.Repo{Root: dir, Remote: "origin", RemoteURL: "git@gitlab.example.com:" + path + ".git", Branch: branch}
	lp := &gitremote.LocalProject{Dir: dir, Name: name, Kind: gitremote.KindGitLab, Repo: repo, ProjectPath: path}
	return workspace.Project{Ref: gitlab.ProjectRef{FullPath: path}, Local: lp}
}

func foreignRow(name, host, path string) workspace.Project {
	repo := &gitremote.Repo{Root: "/src/" + name, Remote: "origin", RemoteURL: "git@" + host + ":" + path + ".git", Branch: "main"}
	lp := &gitremote.LocalProject{Dir: "/src/" + name, Name: name, Kind: gitremote.KindForeign, Repo: repo,
		Remote: &gitremote.Remote{Host: host, Path: path}}
	return workspace.Project{Local: lp, Foreign: true}
}

func noGit(name string) workspace.Project {
	return workspace.Project{Local: &gitremote.LocalProject{Dir: "/src/" + name, Name: name, Kind: gitremote.KindNoGit}}
}

// testScope is the five projects, dotfiles from GitHub and two folders
// without .git.
func testScope() workspace.Scope {
	var rows []workspace.Project
	for _, p := range projects {
		rows = append(rows, localRow("/src/"+p.folder, p.folder, p.path, p.branch))
	}
	rows = append(rows, foreignRow("dotfiles", "github.com", "egor/dotfiles"), noGit("notes"), noGit("tmp"))
	return workspace.NewScope(source, rows)
}

// allPaths are the paths of the five projects.
var allPaths = []string{"qa/e2e-routes", "platform/route-service", "infra/router-configs", "platform/routing-engine", "frontend/route-planner-ui"}

// withState gives env a catalog over a state file in which the groups of
// the test workspace are expanded; it returns the file path.
func withState(t *testing.T, env *app.Env, mrs, pipes []string) string {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.json")
	var st cache.State
	st.SetExpanded(env.Host, source, "mrs", mrs, now)
	st.SetExpanded(env.Host, source, "pipelines", pipes, now)
	if err := cache.SaveState(statePath, st); err != nil {
		t.Fatal(err)
	}
	env.Catalog = app.NewCatalog(nil, statePath, func() string { return env.Host })
	return statePath
}

// openExpanded opens the test workspace with the given groups expanded.
func openExpanded(t *testing.T, client *fake.Client, scope workspace.Scope, mrs, pipes []string) (*uitest.Driver, *app.Model, *Screen) {
	t.Helper()
	env := newEnv(client)
	withState(t, env, mrs, pipes)
	scr := New(env, source, scope)
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr
}

func open(t *testing.T, client *fake.Client, scope workspace.Scope) (*uitest.Driver, *app.Model, *Screen) {
	t.Helper()
	env := newEnv(client)
	scr := New(env, source, scope)
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr
}

// lineWith returns the first view line containing sub.
func lineWith(t *testing.T, d *uitest.Driver, sub string) string {
	t.Helper()
	for _, l := range strings.Split(d.View(), "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	t.Fatalf("no line with %q:\n%s", sub, d.View())
	return ""
}

// lineIndex returns the index of the first view line containing sub.
func lineIndex(d *uitest.Driver, sub string) int {
	for i, l := range strings.Split(d.View(), "\n") {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

// lineAfter returns the view line below the first one containing sub.
func lineAfter(t *testing.T, d *uitest.Driver, sub string) string {
	t.Helper()
	lines := strings.Split(d.View(), "\n")
	for i, l := range lines {
		if strings.Contains(l, sub) && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	t.Fatalf("no line with %q:\n%s", sub, d.View())
	return ""
}
