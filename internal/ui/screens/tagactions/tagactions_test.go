package tagactions

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

var ref = gitlab.ProjectRef{FullPath: "platform/route-service"}

func newEnv(lang i18n.Lang) (*app.Env, *fake.Client) {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: ref, DefaultBranch: "main"})
	env := app.NewEnv(lang)
	env.Client = c
	env.Cache = cache.New()
	return env, c
}

func overlayOf(t *testing.T, cmd tea.Cmd) app.Overlay {
	t.Helper()
	msg, ok := cmd().(app.OpenOverlayMsg)
	if !ok {
		t.Fatalf("want an overlay, got %T", cmd())
	}
	return msg.Overlay
}

func TestDeleteConfirmDescribesProtectedTag(t *testing.T) {
	env, _ := newEnv(i18n.EN)
	tag := gitlab.Tag{Name: "v1.14.2", Protected: true, Commit: gitlab.CommitSummary{ShortSHA: "9fa2c51"}}
	view := ansi.Strip(overlayOf(t, Delete(env, ref, tag)).View(100, 30))
	for _, want := range []string{"Delete tag v1.14.2?", "commit 9fa2c51 stays", "protected: GitLab will most likely refuse"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirm lacks %q:\n%s", want, view)
		}
	}
	plain := gitlab.Tag{Name: "v1.14.1", Commit: gitlab.CommitSummary{ShortSHA: "3e81b07"}}
	if view := ansi.Strip(overlayOf(t, Delete(env, ref, plain)).View(100, 30)); strings.Contains(view, "protected") {
		t.Fatalf("a plain tag must not carry the warning:\n%s", view)
	}
}

func TestDeleteDefaultsToCancel(t *testing.T) {
	env, c := newEnv(i18n.EN)
	c.SetTags(ref.FullPath, gitlab.Tag{Name: "v1.14.1"})
	ov := overlayOf(t, Delete(env, ref, gitlab.Tag{Name: "v1.14.1"}))
	if o, cmd := ov.Update(uitest.KeyPress("enter")); o != nil || cmd != nil {
		t.Fatalf("enter must cancel: %v %v", o, cmd)
	}
	if c.CallCount("Tags.Delete") != 0 {
		t.Fatal("no request without a confirmation")
	}
	_, cmd := ov.Update(uitest.KeyPress("y"))
	done, ok := cmd().(DeletedMsg)
	if !ok || done.Err != nil || done.Name != "v1.14.1" || c.CallCount("Tags.Delete") != 1 {
		t.Fatalf("msg = %#v", done)
	}
}

func toastText(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	m, ok := cmd().(app.ToastMsg)
	if !ok {
		t.Fatalf("want a toast, got %T", cmd())
	}
	return m.Text
}

func TestDoneToasts(t *testing.T) {
	env, _ := newEnv(i18n.EN)
	forbidden := &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403}
	if got := toastText(t, Done(env, DeletedMsg{Ref: ref, Name: "v1.14.2", Protected: true, Err: forbidden})); got != "Tag v1.14.2 is protected" {
		t.Fatalf("protected: %q", got)
	}
	// A 403 on an unprotected tag is a permission problem.
	if got := toastText(t, Done(env, DeletedMsg{Ref: ref, Name: "v1.14.1", Err: forbidden})); got == "Tag v1.14.1 is protected" {
		t.Fatalf("unprotected: %q", got)
	}
	if got := toastText(t, Done(env, DeletedMsg{Ref: ref, Name: "v1.14.1"})); got != "Tag v1.14.1 deleted" {
		t.Fatalf("success: %q", got)
	}
	ru, _ := newEnv(i18n.RU)
	if got := toastText(t, Done(ru, DeletedMsg{Ref: ref, Name: "v1.14.2", Protected: true, Err: forbidden})); got != "Тег v1.14.2 защищён" {
		t.Fatalf("ru: %q", got)
	}
}

func TestInvalidateDropsTagKeys(t *testing.T) {
	env, _ := newEnv(i18n.EN)
	keys := []string{ListKey(ref.FullPath, "", ""), ListKey(ref.FullPath, "v1", "2"), PipesKey(ref.FullPath),
		CompareKey(ref.FullPath, "v1.14.2", "main"), "lasttag:" + ref.FullPath}
	other := ListKey("platform/other", "", "")
	for _, k := range append(keys, other) {
		env.Cache.Set(k, 1)
	}
	Invalidate(env, ref)
	for _, k := range keys {
		if _, ok := cache.Lookup[int](env.Cache, k, 1e18); ok {
			t.Fatalf("%s survived", k)
		}
	}
	if _, ok := cache.Lookup[int](env.Cache, other, 1e18); !ok {
		t.Fatal("another project's cache must stay")
	}
}

func TestCompareUsesCache(t *testing.T) {
	env, c := newEnv(i18n.EN)
	c.SetCompare(ref.FullPath, "v1.14.2", "main", gitlab.Comparison{Commits: []gitlab.CommitSummary{{ShortSHA: "a"}, {ShortSHA: "b"}}})
	for range 2 {
		cmd, cancel := Compare(env, common.NewGen().Current(), ref, "v1.14.2", "main", false)
		msg := cmd().(CompareMsg)
		cancel()
		if msg.Err != nil || len(msg.Cmp.Commits) != 2 {
			t.Fatalf("msg = %+v", msg)
		}
	}
	if c.CallCount("Tags.Compare") != 1 {
		t.Fatalf("compare calls = %d, the second answer comes from the cache", c.CallCount("Tags.Compare"))
	}
}

func TestPreviewLines(t *testing.T) {
	env, _ := newEnv(i18n.EN)
	tag := gitlab.Tag{Name: "v1.14.2", Message: "release: v1.14.2\n\nbody", Protected: true,
		Commit:  gitlab.CommitSummary{ShortSHA: "9fa2c51", Title: "Fix slot race"},
		Release: &gitlab.ReleaseSummary{Description: "\nFixes the slot race\nmore"}}
	lines := func(d PreviewData) []string {
		out := Preview(env, d)
		for i := range out {
			out[i] = ansi.Strip(out[i])
		}
		return out
	}
	got := lines(PreviewData{Tag: tag, Branch: "main", State: CompareDone, Ahead: 2, Suggest: "v1.14.3"})
	want := []string{"v1.14.2 @ 9fa2c51 · release: v1.14.2 · protected", "main is 2 commits ahead · n suggests v1.14.3", "Release notes: Fixes the slot race"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
	// A lightweight tag shows the commit title; no release, no notes.
	light := gitlab.Tag{Name: "v1.14.1", Commit: gitlab.CommitSummary{ShortSHA: "3e81b07", Title: "Bump deps"}}
	if got := lines(PreviewData{Tag: light, Branch: "main", State: CompareDone}); got[0] != "v1.14.1 @ 3e81b07 · Bump deps" || got[1] != "main matches the tag" || got[2] != "" {
		t.Fatalf("lines = %q", got)
	}
	if got := lines(PreviewData{Tag: light, Branch: "main", State: CompareLoading, Suggest: "v1.14.2"}); got[1] != "◐ comparing… · n suggests v1.14.2" {
		t.Fatalf("loading = %q", got[1])
	}
	if got := lines(PreviewData{Tag: light, Branch: "main", State: CompareFailed}); got[1] != "comparison unavailable" {
		t.Fatalf("failed = %q", got[1])
	}
	// No default branch: only the suggestion.
	if got := lines(PreviewData{Tag: light, Suggest: "v1.14.2"}); got[1] != "n suggests v1.14.2" {
		t.Fatalf("no branch = %q", got[1])
	}
}

func TestSuggestedAndURL(t *testing.T) {
	tags := []gitlab.Tag{{Name: "v1.14.2"}, {Name: "v1.14.1"}}
	if got := Suggested(tags); got != "v1.14.3" {
		t.Fatalf("suggested = %q", got)
	}
	if got := TagsURL("https://gitlab.example.com/platform/route-service", ""); got != "https://gitlab.example.com/platform/route-service/-/tags" {
		t.Fatalf("url = %q", got)
	}
	if got := TagsURL("https://gitlab.example.com/p", "v1.1 x"); got != "https://gitlab.example.com/p/-/tags?search=v1.1+x" {
		t.Fatalf("url = %q", got)
	}
}
