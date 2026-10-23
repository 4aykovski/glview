package project

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/tagactions"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

// tagsFake has six tags of project platform/route-service, main two commits
// ahead of v1.14.2.
func tagsFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, DefaultBranch: "main", WebURL: "https://gitlab.example.com/" + path})
	c.SetBranches(path, "main", "feat/surge")
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	day := 24 * time.Hour
	tag := func(name, sha, msg, title string, age time.Duration) gitlab.Tag {
		return gitlab.Tag{Project: gitlab.ProjectRef{FullPath: path}, Name: name, Message: msg,
			Commit: gitlab.CommitSummary{ShortSHA: sha, Title: title, CreatedAt: ago(age)}}
	}
	t1 := tag("v1.14.2", "9fa2c51", "release: v1.14.2", "Fix slot race", day)
	t1.Protected = true
	t1.Release = &gitlab.ReleaseSummary{Name: "v1.14.2", Description: "\nfix race in slot allocator (!479), pprof on admin port (!465)\n"}
	c.SetTags(path, t1,
		tag("v1.14.1", "3e81b07", "", "hotfix: nil map in route cache", 6*day),
		tag("v1.14.0", "c2d9a44", "release: v1.14.0", "Release 1.14", 14*day),
		tag("v1.14.0-rc2", "b87f1e0", "rc for zone-based pricing", "pricing", 15*day),
		tag("v1.13.4", "60aa2fd", "release: v1.13.4", "Release 1.13.4", 30*day),
		tag("v1.13.3", "17c4e5b", "release: v1.13.3", "Release 1.13.3", 31*day),
	)
	for _, p := range []struct {
		id     int64
		ref    string
		status gitlab.PipelineStatus
	}{
		{91761, "v1.14.2", gitlab.StatusSuccess}, {91530, "v1.14.1", gitlab.StatusSuccess}, {91402, "v1.14.0", gitlab.StatusSuccess},
		{91377, "v1.14.0-rc2", gitlab.StatusFailed}, {90988, "v1.13.4", gitlab.StatusSuccess},
	} {
		c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{
			ID: p.id, IID: p.id - 90000, Status: p.status, Ref: p.ref, CreatedAt: ago(time.Hour),
			WebURL: fmt.Sprintf("https://gitlab.example.com/%s/-/pipelines/%d", path, p.id)}})
	}
	head := gitlab.CommitSummary{ShortSHA: "0b7d3e9", Title: "Merge branch 'feat/pprof'"}
	c.SetCompare(path, "v1.14.2", "main", gitlab.Comparison{Head: &head, Commits: []gitlab.CommitSummary{
		head, {ShortSHA: "5c0e7d1", Title: "slots: retry on serialization failure"}}})
	c.SetCompare(path, "v1.14.1", "main", gitlab.Comparison{Head: &head, Commits: make([]gitlab.CommitSummary, 5)})
	c.SetCompare(path, "v1.14.0", "main", gitlab.Comparison{Head: &head})
	return c
}

// requireRow fails unless a line of the view holds the text, whitespace
// collapsed.
func requireRow(t *testing.T, d *uitest.Driver, want string) {
	t.Helper()
	for _, l := range splitLines(d.View()) {
		if strings.Contains(strings.Join(strings.Fields(l), " "), want) {
			return
		}
	}
	t.Fatalf("no row %q in:\n%s", want, d.View())
}

func openTags(t *testing.T, client *fake.Client, setup func(*app.Env)) (*uitest.Driver, *app.Model, *tagsTab) {
	t.Helper()
	env := newEnv(client)
	if setup != nil {
		setup(env)
	}
	scr := New(env, Options{Path: path, Tab: TabTags})
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr.tabs[TabTags].(*tagsTab)
}

func TestTagsListAndPreview(t *testing.T) {
	client := tagsFake()
	d, _, tab := openTags(t, client, nil)
	if len(tab.Items()) != 6 || tab.Items()[0].Name != "v1.14.2" {
		t.Fatalf("items = %+v", tab.Items())
	}
	d.RequireView("4 Tags 6", "/ search", "sort: semver ↓", "Tag", "Commit", "Message", "Pipeline", "Ago")
	requireRow(t, d, "› v1.14.2 9fa2c51 release: v1.14.2 ◆ release ✓ #91761 1d")
	requireRow(t, d, "v1.14.0-rc2 b87f1e0 rc for zone-based pricing ✗ #91377 2w")
	requireRow(t, d, "v1.14.1 3e81b07 hotfix: nil map in route cache ✓ #91530 6d")
	d.RequireView("v1.14.2 @ 9fa2c51 · release: v1.14.2 · protected",
		"main is 2 commits ahead · n suggests v1.14.3",
		"Release notes: fix race in slot allocator (!479), pprof on admin port (!465)",
		"1 / 6")
	// The tag has no pipeline in the window: a dash.
	requireRow(t, d, "v1.13.3 17c4e5b release: v1.13.3 — 1mo")
	// Requests: tags sorted by the server, pipelines of tags in a window of 100.
	if got := client.CallsOf("Tags.Pipelines"); len(got) != 1 || got[0][1] != 100 {
		t.Fatalf("pipelines calls = %v", got)
	}
}

func TestTagsPreviewStatesFollowTheCursor(t *testing.T) {
	d, _, _ := openTags(t, tagsFake(), nil)
	d.Key("j")
	d.RequireView("v1.14.1 @ 3e81b07 · hotfix: nil map in route cache", "main is 5 commits ahead · n suggests v1.14.3")
	d.RequireNoView("protected", "Release notes")
	d.Key("j")
	d.RequireView("main matches the tag")
}

func TestTagsCompareErrorIsQuiet(t *testing.T) {
	client := tagsFake()
	client.FailNext("Tags.Compare", &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	d, _, _ := openTags(t, client, nil)
	d.RequireView("comparison unavailable · n suggests v1.14.3")
	d.RequireNoView("server error") // no toast
}

func TestTagsCompareForAnotherTagIsDropped(t *testing.T) {
	client := tagsFake()
	client.SetDelay("Tags.Compare", time.Second) // the driver drops the slow answers
	d, _, tab := openTags(t, client, nil)
	d.RequireView("◐ comparing… · n suggests v1.14.3")
	old := tab.cmp.Token()
	d.Key("j")
	d.Send(tagactions.CompareMsg{Tok: old, Ref: tab.sh.ref, Tag: "v1.14.2", Branch: "main",
		Cmp: gitlab.Comparison{Commits: make([]gitlab.CommitSummary, 9)}})
	if e, ok := tab.cmp.Entry(tab.sh.ref, "v1.14.2"); ok {
		t.Fatalf("the answer for v1.14.2 must be dropped: %+v", e)
	}
	d.RequireView("v1.14.1 @ 3e81b07", "◐ comparing…")
	d.RequireNoView("9 commits")
	// The answer for the selected tag lands.
	d.Send(tagactions.CompareMsg{Tok: tab.cmp.Token(), Ref: tab.sh.ref, Tag: "v1.14.1", Branch: "main",
		Cmp: gitlab.Comparison{Commits: make([]gitlab.CommitSummary, 3)}})
	d.RequireView("main is 3 commits ahead")
}

func TestTagsSearch(t *testing.T) {
	client := tagsFake()
	d, _, tab := openTags(t, client, nil)
	d.Key("/")
	if !tab.CapturesInput() {
		t.Fatal("the search input must capture the keys")
	}
	for _, r := range "v1.13" {
		d.Key(string(r))
	}
	d.Key("enter")
	calls := client.CallsOf("Tags.List")
	if last := calls[len(calls)-1]; last[1] != "v1.13" || last[2] != "" {
		t.Fatalf("List args = %v", last)
	}
	d.RequireView("/ search: v1.13")
	if len(tab.Items()) != 2 {
		t.Fatalf("items = %+v", tab.Items())
	}
	// A search hides the suggestion: it comes from the whole list.
	d.RequireNoView("n suggests")
	d.Key("backspace")
	d.RequireView("/ search", "n suggests v1.14.3")
	d.RequireNoView("search:")
	if len(tab.Items()) != 6 {
		t.Fatalf("items = %+v", tab.Items())
	}
}

func TestTagsSearchEscCancels(t *testing.T) {
	client := tagsFake()
	d, _, tab := openTags(t, client, nil)
	n := client.CallCount("Tags.List")
	d.Key("/", "x", "esc")
	if tab.CapturesInput() || tab.search != "" || client.CallCount("Tags.List") != n {
		t.Fatal("esc must cancel the search without a request")
	}
}

func TestTagsEmptyStates(t *testing.T) {
	c := tagsFake()
	c.SetTags(path)
	d, _, _ := openTags(t, c, nil)
	d.RequireView("No tags")
	d.Key("/", "v", "9", "enter")
	d.RequireView("No tags matching “v9” · backspace — reset the search")
}

func TestTagsMAndSDoNothing(t *testing.T) {
	client := tagsFake()
	d, _, tab := openTags(t, client, nil)
	n := client.CallCount("Tags.List")
	d.Key("m", "s")
	if client.CallCount("Tags.List") != n || tab.search != "" {
		t.Fatal("m and s must not act")
	}
}

func TestTagsEnterOpensThePipeline(t *testing.T) {
	d, root, _ := openTags(t, tagsFake(), nil)
	d.Key("enter")
	if root.Top().Title() == path {
		t.Fatalf("the pipeline screen must open, top = %q", root.Top().Title())
	}
	d.Key("esc")
	d.RequireView("4 Tags")
}

func TestTagsEnterWithoutPipeline(t *testing.T) {
	d, root, _ := openTags(t, tagsFake(), nil)
	d.Key("G", "enter")
	d.RequireView("The tag has no pipeline")
	if root.Top().Title() != path {
		t.Fatalf("the screen must stay, top = %q", root.Top().Title())
	}
}

func TestTagsCopyAndOpen(t *testing.T) {
	var opened string
	d, _, _ := openTags(t, tagsFake(), func(env *app.Env) { env.Browser = func(u string) error { opened = u; return nil } })
	d.Key("y")
	d.RequireView("copied: tag name")
	d.Key("o")
	if opened != "https://gitlab.example.com/"+path+"/-/tags" {
		t.Fatalf("opened %q", opened)
	}
	d.Key("/", "v", "1", "enter", "o")
	if opened != "https://gitlab.example.com/"+path+"/-/tags?search=v1" {
		t.Fatalf("opened %q", opened)
	}
}

func TestTagsDelete(t *testing.T) {
	client := tagsFake()
	d, _, tab := openTags(t, client, nil)
	d.Key("j", "D")
	d.RequireView("Delete tag v1.14.1?", "commit 3e81b07 stays")
	d.RequireNoView("protected")
	d.Key("y")
	if got := client.CallsOf("Tags.Delete"); len(got) != 1 || got[0][1] != "v1.14.1" {
		t.Fatalf("Delete calls = %v", got)
	}
	d.RequireView("Tag v1.14.1 deleted")
	if len(tab.Items()) != 5 {
		t.Fatalf("the list must reload: %d", len(tab.Items()))
	}
	for _, it := range tab.Items() {
		if it.Name == "v1.14.1" {
			t.Fatal("v1.14.1 is still listed")
		}
	}
}

func TestTagsDeleteDefaultsToCancel(t *testing.T) {
	client := tagsFake()
	d, _, _ := openTags(t, client, nil)
	d.Key("D", "enter")
	if client.CallCount("Tags.Delete") != 0 {
		t.Fatal("enter in the confirm is Cancel")
	}
}

func TestTagsProtectedDelete(t *testing.T) {
	client := tagsFake()
	d, _, tab := openTags(t, client, nil)
	d.Key("D")
	d.RequireView("Delete tag v1.14.2?", "protected: GitLab will most likely refuse")
	d.Key("y")
	d.RequireView("Tag v1.14.2 is protected")
	if len(tab.Items()) != 6 || tab.Items()[0].Name != "v1.14.2" {
		t.Fatalf("the tag must stay: %+v", tab.Items())
	}
}

func TestTagsCreateFromTheTab(t *testing.T) {
	client := tagsFake()
	d, _, tab := openTags(t, client, nil)
	d.Key("j", "j", "n")
	d.RequireView("New tag · platform/route-service", "suggested: patch from v1.14.2")
	d.Key("tab", "tab", "tab", "enter")
	d.RequireView("Create tag v1.14.3 on main @ 0b7d3e9?")
	d.Key("y")
	if got := client.CallsOf("Tags.Create"); len(got) != 1 || got[0][1] != "v1.14.3" || got[0][2] != "main" || got[0][3] != "release: v1.14.3" {
		t.Fatalf("Create calls = %v", got)
	}
	d.RequireView("Tag v1.14.3 created")
	if tab.tbl.CursorID() != "v1.14.3" || tab.Items()[0].Name != "v1.14.3" {
		t.Fatalf("cursor = %q items = %+v", tab.tbl.CursorID(), tab.Items())
	}
}

func TestTagsArchivedProjectCannotCreateOrDelete(t *testing.T) {
	client := tagsFake()
	client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, DefaultBranch: "main", Archived: true})
	d, _, _ := openTags(t, client, nil)
	d.RequireNoView("new tag", "delete")
	d.Key("n", "D")
	d.RequireNoView("New tag", "Delete tag")
	d.RequireView("copy name")
}

func TestTagsPagesAndPoll(t *testing.T) {
	client := tagsFake()
	var many []gitlab.Tag
	for i := 45; i >= 1; i-- {
		many = append(many, gitlab.Tag{Name: fmt.Sprintf("v1.%d.0", i), Commit: gitlab.CommitSummary{ShortSHA: fmt.Sprintf("%07d", i)}})
	}
	client.SetTags(path, many...)
	d, _, tab := openTags(t, client, nil)
	if len(tab.Items()) != 20 || tab.Count() != 45 {
		t.Fatalf("first page = %d count = %d", len(tab.Items()), tab.Count())
	}
	d.RequireView("4 Tags 45", "1 / 45")
	for range 16 { // within 5 rows of the end
		d.Key("j")
	}
	if len(tab.Items()) != 40 {
		t.Fatalf("the next page must load near the end: %d", len(tab.Items()))
	}
	// A poll keeps the cursor on the same tag when a new one shows up first.
	name := tab.tbl.CursorID()
	client.SetTags(path, append([]gitlab.Tag{{Name: "v1.46.0", Commit: gitlab.CommitSummary{ShortSHA: "0000046"}}}, many...)...)
	d.Send(commonPoll(tab))
	if tab.tbl.CursorID() != name {
		t.Fatalf("cursor moved from %q to %q", name, tab.tbl.CursorID())
	}
	if tab.Items()[0].Name != "v1.46.0" {
		t.Fatalf("first = %q", tab.Items()[0].Name)
	}
}

func TestTagsOpenedFromWorkspaceOptions(t *testing.T) {
	client := tagsFake()
	env := newEnv(client)
	scr := New(env, Options{Path: path, Tab: TabTags, TagSearch: "v1.13"})
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	if scr.Current() != TabTags {
		t.Fatalf("tab = %d", scr.Current())
	}
	d.RequireView("/ search: v1.13")
	if len(scr.tabs[TabTags].(*tagsTab).Items()) != 2 {
		t.Fatal("the search must be applied")
	}
}

func TestTagsRussian(t *testing.T) {
	d, _, _ := openTags(t, tagsFake(), nil)
	d.Key("ctrl+l")
	d.RequireView("/ поиск", "сорт: semver ↓", "main опережает на 2 коммита · n предложит v1.14.3")
}

func TestGoldenTags(t *testing.T) {
	d, _, _ := openTags(t, tagsFake(), nil)
	d.RequireGoldenSizes("tags")
}

func commonPoll(tab *tagsTab) common.PollTickMsg {
	return common.PollTickMsg{Token: tab.poll.Current()}
}
