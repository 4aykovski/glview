package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const (
	routeSvc = "platform/route-service"
	engine   = "platform/routing-engine"
	planner  = "frontend/route-planner-ui"
)

// tagsFake adds to the test workspace 58 tags in route-service (v1.58.0 …
// v1.1.0, the newest with a release and a pipeline), 12 in routing-engine
// and the single tag of the planner.
func tagsFake() *fake.Client {
	c := newFake()
	var many []gitlab.Tag
	for i := 58; i >= 1; i-- {
		many = append(many, gitlab.Tag{Name: fmt.Sprintf("v1.%d.0", i), Message: fmt.Sprintf("release: v1.%d.0", i),
			Commit: gitlab.CommitSummary{ShortSHA: fmt.Sprintf("%07x", 0x9fa2c50+i), Title: "Release", CreatedAt: now.Add(-time.Duration(i) * 24 * time.Hour)}})
	}
	many[0].Release = &gitlab.ReleaseSummary{Description: "fix race in slot allocator (!479)"}
	many[0].Protected = true
	c.SetTags(routeSvc, many...)
	var few []gitlab.Tag
	for i := 12; i >= 1; i-- {
		few = append(few, gitlab.Tag{Name: fmt.Sprintf("v0.%d.0", i), Commit: gitlab.CommitSummary{ShortSHA: fmt.Sprintf("%07x", 0x3e81b00+i), Title: "Bump", CreatedAt: now.Add(-time.Duration(i) * 48 * time.Hour)}})
	}
	c.SetTags(engine, few...)
	c.SetTags(planner, gitlab.Tag{Name: "v2.3.1", Commit: gitlab.CommitSummary{ShortSHA: "c2d9a44", Title: "Heatmap", CreatedAt: now.Add(-3 * 24 * time.Hour)}})
	for path, id := range map[string]int64{routeSvc: 91761, engine: 40100} {
		ref := "v1.58.0"
		if path == engine {
			ref = "v0.12.0"
		}
		c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: id, IID: id % 1000, Status: gitlab.StatusSuccess, Ref: ref,
			CreatedAt: now.Add(-time.Hour), WebURL: "https://gitlab.example.com/" + path + "/-/pipelines/" + itoa(id)}})
	}
	head := gitlab.CommitSummary{ShortSHA: "0b7d3e9", Title: "Merge branch 'feat/pprof'"}
	c.SetCompare(routeSvc, "v1.58.0", "main", gitlab.Comparison{Head: &head, Commits: []gitlab.CommitSummary{head, {ShortSHA: "5c0e7d1"}}})
	c.SetCompare(routeSvc, "v1.57.0", "main", gitlab.Comparison{Head: &head, Commits: make([]gitlab.CommitSummary, 7)})
	c.SetBranches(routeSvc, "main")
	return c
}

// openTags opens the test workspace on the Tags tab with the given groups
// expanded.
func openTags(t *testing.T, client *fake.Client, expanded ...string) (*uitest.Driver, *app.Model, *Screen, string) {
	t.Helper()
	env := newEnv(client)
	statePath := filepath.Join(t.TempDir(), "state.json")
	var st cache.State
	st.SetExpanded(env.Host, source, "tags", expanded, now)
	if err := cache.SaveState(statePath, st); err != nil {
		t.Fatal(err)
	}
	env.Catalog = app.NewCatalog(nil, statePath, func() string { return env.Host })
	scr := New(env, source, testScope())
	root := app.New(env, scr, app.Options{})
	d := uitest.New(t, root)
	d.Key("4")
	return d, root, scr, statePath
}

func tagsTabOf(scr *Screen) *tagsTab { return scr.tabs[TabTags].(*tagsTab) }

func TestTagsTabCollapsedByDefault(t *testing.T) {
	client := tagsFake()
	d, _, _, _ := openTags(t, client)
	if n := client.CallCount("Workspace.Tags"); n != 0 {
		t.Fatalf("collapsed groups are not asked: %d calls", n)
	}
	d.RequireView("▸ platform/route-service  v1.58.0", "▸ platform/routing-engine  v0.12.0", "▸ frontend/route-planner-ui  v2.3.1",
		"▸ qa/e2e-routes  no tags", "▸ infra/router-configs  no tags")
	d.RequireNoView("4 Tags 5") // no count without a narrowing filter
	d.RequireView("/ search", "me: not applied to tags", "per project: 3 · sort: version ↓")
}

func TestTagsExpandAsksOnlyThatGroupAndIsRemembered(t *testing.T) {
	client := tagsFake()
	d, _, scr, statePath := openTags(t, client)
	d.Key("space") // route-service
	d.Send(scr.tabs[TabTags].(*tagsTab).ExpandMsg())
	calls := client.CallsOf("Workspace.Tags")
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	if paths := calls[0][0].([]string); len(paths) != 1 || paths[0] != routeSvc || calls[0][1] != "" || calls[0][2] != 3 {
		t.Fatalf("asked %v", calls[0])
	}
	d.RequireView("▾ platform/route-service  58 tags · showing 3", "v1.58.0", "v1.57.0", "v1.56.0", "↳ 55 more · f filter by project", "▸ platform/routing-engine")
	d.RequireNoView("v1.55.0")
	// The expanded set is kept per tab.
	if got := cache.LoadState(statePath).Expanded("https://gitlab.example.com", source, "tags"); strings.Join(got, ",") != routeSvc {
		t.Fatalf("state = %v", got)
	}
	if got := cache.LoadState(statePath).Expanded("https://gitlab.example.com", source, "mrs"); len(got) != 0 {
		t.Fatalf("MRs state = %v", got)
	}
}

func TestTagsRowsAndPreview(t *testing.T) {
	d, _, _, _ := openTags(t, tagsFake(), routeSvc)
	requireRowText(t, d, "› ▾ platform/route-service")
	d.Key("j")
	requireRowText(t, d, "v1.58.0 9fa2c8a release: v1.58.0 ◆ release ✓ #91761 1mo")
	requireRowText(t, d, "v1.57.0 9fa2c89 release: v1.57.0 — 1mo")
	d.RequireView("v1.58.0 @ 9fa2c8a · release: v1.58.0 · protected", "main is 2 commits ahead · n suggests v1.58.1",
		"Release notes: fix race in slot allocator (!479)")
	d.Key("j")
	d.RequireView("v1.57.0 @ 9fa2c89", "main is 7 commits ahead")
}

func requireRowText(t *testing.T, d *uitest.Driver, want string) {
	t.Helper()
	for _, l := range strings.Split(d.View(), "\n") {
		if strings.Contains(strings.Join(strings.Fields(l), " "), want) {
			return
		}
	}
	t.Fatalf("no row %q in:\n%s", want, d.View())
}

func TestTagsSearchAsksAllProjects(t *testing.T) {
	client := tagsFake()
	d, _, _, _ := openTags(t, client)
	d.Key("/")
	for _, r := range "v2" {
		d.Key(string(r))
	}
	d.Key("enter")
	calls := client.CallsOf("Workspace.Tags")
	last := calls[len(calls)-1]
	if paths := last[0].([]string); len(paths) != 5 || last[1] != "v2" {
		t.Fatalf("asked %v", last)
	}
	// The fake matches v2 as a substring: only the planner (v2.3.1) has such a tag.
	d.RequireView("/ search: v2", "found in 1 of 5 projects", "▾ frontend/route-planner-ui", "v2.3.1", "4 Tags 1")
	d.RequireNoView("platform/route-service", "n suggests")
	d.Key("backspace")
	d.RequireView("/ search", "▸ platform/route-service")
	d.RequireNoView("search:", "found in")
}

func TestTagsMAndSDoNothing(t *testing.T) {
	client := tagsFake()
	d, _, scr, _ := openTags(t, client, routeSvc)
	n := client.CallCount("Workspace.Tags")
	d.Key("m", "s")
	if client.CallCount("Workspace.Tags") != n || tagsTabOf(scr).search != "" {
		t.Fatal("m and s must not act")
	}
	d.RequireView("me: not applied to tags")
}

func TestTagsNewTagFromAnyLineOfTheGroup(t *testing.T) {
	d, _, _, _ := openTags(t, tagsFake(), routeSvc)
	for _, keys := range [][]string{{"g"}, {"g", "j", "j", "j", "j"}, {"g", "j", "j"}} { // header, "more" line, a tag
		d.Key(keys...)
		d.Key("n")
		d.RequireView("New tag · platform/route-service")
		d.Key("esc")
	}
	// On the header of a collapsed group too, in that group's project.
	d.Key("J")
	d.Key("n")
	d.RequireView("New tag · platform/routing-engine")
}

func TestTagsCreateMovesTheCursorToTheNewTag(t *testing.T) {
	client := tagsFake()
	client.SetCompare(routeSvc, "v1.58.0", "main", gitlab.Comparison{Head: &gitlab.CommitSummary{ShortSHA: "0b7d3e9"}, Commits: make([]gitlab.CommitSummary, 2)})
	d, _, scr, _ := openTags(t, client, routeSvc)
	d.Key("n")
	d.Key("tab", "tab", "tab", "enter")
	d.RequireView("Create tag v1.58.1 on main @ 0b7d3e9?")
	d.Key("y")
	if got := client.CallsOf("Tags.Create"); len(got) != 1 || got[0][1] != "v1.58.1" || got[0][2] != "main" {
		t.Fatalf("Create calls = %v", got)
	}
	d.RequireView("Tag v1.58.1 created", "▾ platform/route-service  59 tags")
	g, r, _ := tagsTabOf(scr).tbl.Cursor()
	if g == nil || g.Key != routeSvc || r == nil || r.ID != "v1.58.1" {
		t.Fatalf("cursor on %v %v", g, r)
	}
	// The collapsed header follows.
	d.Key("space")
	d.RequireView("▸ platform/route-service  v1.58.1")
}

func TestTagsDelete(t *testing.T) {
	client := tagsFake()
	d, _, _, _ := openTags(t, client, routeSvc)
	d.Key("D") // on the header: nothing
	d.RequireNoView("Delete tag")
	d.Key("j", "j", "D")
	d.RequireView("Delete tag v1.57.0?", "commit 9fa2c89 stays")
	d.Key("y")
	if got := client.CallsOf("Tags.Delete"); len(got) != 1 || got[0][1] != "v1.57.0" {
		t.Fatalf("Delete calls = %v", got)
	}
	d.RequireView("Tag v1.57.0 deleted", "57 tags · showing 3", "v1.56.0")
	d.RequireNoView("release: v1.57.0")
}

func TestTagsProtectedDeleteToast(t *testing.T) {
	d, _, _, _ := openTags(t, tagsFake(), routeSvc)
	d.Key("j", "D", "y")
	d.RequireView("Tag v1.58.0 is protected", "v1.58.0")
}

func TestTagsEnterOpensPipelineAndTogglesHeader(t *testing.T) {
	d, root, scr, _ := openTags(t, tagsFake(), routeSvc)
	d.Key("enter") // header: collapse
	d.RequireView("▸ platform/route-service")
	d.Key("enter")
	d.Send(tagsTabOf(scr).ExpandMsg())
	d.RequireView("▾ platform/route-service", "v1.58.0")
	d.Key("j", "enter")
	if _, ok := root.Top().(*Screen); ok {
		t.Fatal("the pipeline screen must open")
	}
	d.Key("esc", "j", "enter") // v1.57.0 has no pipeline
	d.RequireView("The tag has no pipeline")
	if _, ok := root.Top().(*Screen); !ok {
		t.Fatal("the workspace stays")
	}
}

func TestTagsFOpensTheProjectOnTags(t *testing.T) {
	client := tagsFake()
	d, root, _, _ := openTags(t, client, routeSvc)
	d.Key("/")
	for _, r := range "v1.1" {
		d.Key(string(r))
	}
	d.Key("enter", "f")
	p, ok := root.Top().(*project.Project)
	if !ok {
		t.Fatalf("top = %T", root.Top())
	}
	if p.Current() != project.TabTags || p.Path() != routeSvc {
		t.Fatalf("tab = %d path = %q", p.Current(), p.Path())
	}
	d.RequireView("~/src › platform/route-service", "/ search: v1.1")
	d.Key("esc")
	d.RequireView("4 Tags", "/ search: v1.1")
}

func TestTagsOpenInBrowser(t *testing.T) {
	client := tagsFake()
	env := newEnv(client)
	var opened string
	env.Browser = func(u string) error { opened = u; return nil }
	scr := New(env, source, testScope())
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	d.Key("4", "o") // the first group: route-service
	if opened != "https://gitlab.example.com/"+routeSvc+"/-/tags" {
		t.Fatalf("opened %q", opened)
	}
	d.Key("/", "v", "1", "enter", "o")
	if opened != "https://gitlab.example.com/"+routeSvc+"/-/tags?search=v1" {
		t.Fatalf("opened %q", opened)
	}
}

func TestTagsArchivedProjectCannotCreateOrDelete(t *testing.T) {
	client := tagsFake()
	client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: routeSvc}, DefaultBranch: "main", Archived: true,
		LastActivityAt: now.Add(-time.Hour), WebURL: "https://gitlab.example.com/" + routeSvc})
	d, _, _, _ := openTags(t, client, routeSvc)
	d.Key("n")
	d.RequireNoView("New tag")
	d.Key("j", "D")
	d.RequireNoView("Delete tag")
}

func TestTagsPollAsksOnlyExpanded(t *testing.T) {
	client := tagsFake()
	d, _, scr, _ := openTags(t, client, routeSvc)
	n := client.CallCount("Workspace.Tags")
	d.Send(common.PollTickMsg{Token: tagsTabOf(scr).poll.Current()})
	calls := client.CallsOf("Workspace.Tags")
	if len(calls) != n+1 {
		t.Fatalf("calls = %d, want %d", len(calls), n+1)
	}
	if paths := calls[len(calls)-1][0].([]string); len(paths) != 1 || paths[0] != routeSvc {
		t.Fatalf("polled %v", paths)
	}
}

func TestTagsRussian(t *testing.T) {
	d, _, _, _ := openTags(t, tagsFake(), routeSvc)
	d.Key("ctrl+l")
	d.RequireView("/ поиск", "я: не применяется к тегам", "на проект: 3 · сорт: version ↓", "58 тегов · показано 3", "↳ ещё 55")
}

func TestGoldenTagsTab(t *testing.T) {
	d, _, _, _ := openTags(t, tagsFake(), routeSvc, engine)
	d.Key("j")
	d.RequireGoldenSizes("tags-expanded")
}

// toasts counts the toasts with the text shown so far.
func toasts(d *uitest.Driver, text string) int {
	n := 0
	for _, m := range d.Msgs {
		if tm, ok := m.(app.ToastMsg); ok && tm.Text == text {
			n++
		}
	}
	return n
}

// A project opened with f from the group: the project tab asked, so it
// alone toasts and reloads; the group tab under it follows on return.
func TestTagsDeleteInTheProjectOpenedWithF(t *testing.T) {
	client := tagsFake()
	d, root, _, _ := openTags(t, client, routeSvc)
	d.Key("f")
	if _, ok := root.Top().(*project.Project); !ok {
		t.Fatalf("top = %T", root.Top())
	}
	d.RequireView("v1.57.0")
	lists, wsTags := client.CallCount("Tags.List"), client.CallCount("Workspace.Tags")
	d.Key("j", "D", "y")
	if got := client.CallCount("Tags.Delete"); got != 1 {
		t.Fatalf("Delete calls = %d", got)
	}
	if n := toasts(d, "Tag v1.57.0 deleted"); n != 1 {
		t.Fatalf("toasts = %d", n)
	}
	if got := client.CallCount("Tags.List") - lists; got != 1 {
		t.Fatalf("the project list is asked %d times", got)
	}
	if got := client.CallCount("Workspace.Tags"); got != wsTags {
		t.Fatalf("the covered group tab must wait: %d -> %d", wsTags, got)
	}
	d.Key("esc")
	d.RequireView("57 tags · showing 3", "v1.56.0")
	d.RequireNoView("release: v1.57.0")
	if got := client.CallCount("Workspace.Tags"); got != wsTags+1 {
		t.Fatalf("the group tab reloads once on return: %d -> %d", wsTags, got)
	}
	if n := toasts(d, "Tag v1.57.0 deleted"); n != 1 {
		t.Fatalf("toasts = %d", n)
	}
}

func TestTagsDeleteErrorInTheProjectOpenedWithFToastsOnce(t *testing.T) {
	client := tagsFake()
	d, _, _, _ := openTags(t, client, routeSvc)
	d.Key("f")
	d.RequireView("v1.57.0")
	client.FailNext("Tags.Delete", &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	wsTags := client.CallCount("Workspace.Tags")
	n := len(d.Msgs)
	d.Key("j", "D", "y")
	errs := 0
	for _, m := range d.Msgs[n:] {
		if tm, ok := m.(app.ToastMsg); ok && tm.Level == common.ToastError {
			errs++
		}
	}
	if errs != 1 {
		t.Fatalf("error toasts = %d", errs)
	}
	d.Key("esc")
	if got := client.CallCount("Workspace.Tags"); got != wsTags {
		t.Fatalf("nothing changed, the group tab stays: %d -> %d", wsTags, got)
	}
}

func TestTagsCreateInTheProjectOpenedWithF(t *testing.T) {
	client := tagsFake()
	client.SetCompare(routeSvc, "v1.58.0", "main", gitlab.Comparison{Head: &gitlab.CommitSummary{ShortSHA: "0b7d3e9"}, Commits: make([]gitlab.CommitSummary, 2)})
	d, _, _, _ := openTags(t, client, routeSvc)
	d.Key("f")
	d.RequireView("v1.57.0")
	wsTags := client.CallCount("Workspace.Tags")
	d.Key("n", "tab", "tab", "tab", "enter", "y")
	if got := client.CallCount("Tags.Create"); got != 1 {
		t.Fatalf("Create calls = %d", got)
	}
	if n := toasts(d, "Tag v1.58.1 created"); n != 1 {
		t.Fatalf("toasts = %d", n)
	}
	if got := client.CallCount("Workspace.Tags"); got != wsTags {
		t.Fatalf("the covered group tab must wait: %d -> %d", wsTags, got)
	}
	d.Key("esc")
	d.RequireView("59 tags", "v1.58.1")
	if got := client.CallCount("Workspace.Tags"); got != wsTags+1 {
		t.Fatalf("the group tab reloads once on return: %d -> %d", wsTags, got)
	}
}
