package newtag

import (
	"fmt"
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
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

// newFake has the tags v1.14.2 and v1.14.1 and main two commits ahead of v1.14.2.
func newFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: project, DefaultBranch: "main"})
	c.SetBranches(path, "main", "feat/surge", "feat/slots", "fix/race")
	c.SetTags(path, gitlab.Tag{Name: "v1.14.2", Commit: commit("9fa2c51", "Fix slot race")}, gitlab.Tag{Name: "v1.14.1"})
	head := commit("0b7d3e9", "Merge branch 'feat/pprof'")
	c.SetCompare(path, "v1.14.2", "main", gitlab.Comparison{Head: &head, Commits: []gitlab.CommitSummary{head, commit("5c0e7d1", "slots: retry on serialization failure")}})
	c.SetCompare(path, "v1.14.2", "feat/surge", gitlab.Comparison{Head: &gitlab.CommitSummary{ShortSHA: "a91f2c0"}, Commits: []gitlab.CommitSummary{commit("a91f2c0", "surge: add pricing")}})
	return c
}

func open(t *testing.T, c *fake.Client, lang i18n.Lang) (*uitest.Driver, *app.Env, *Form) {
	t.Helper()
	env := app.NewEnv(lang)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	root := app.New(env, host{}, app.Options{})
	d := uitest.New(t, root)
	f := New(env, project, "main")
	d.Send(app.OpenOverlayMsg{Overlay: f})
	d.Run(f.Init())
	return d, env, f
}

func typeText(d *uitest.Driver, s string) {
	for _, r := range s {
		d.Key(string(r))
	}
}

func created(d *uitest.Driver) *CreatedMsg {
	for _, m := range d.Msgs {
		if c, ok := m.(CreatedMsg); ok {
			return &c
		}
	}
	return nil
}

func TestFormShowsSuggestionsRefAndCommits(t *testing.T) {
	d, _, f := open(t, newFake(), i18n.EN)
	d.RequireView("New tag · platform/route-service", "Name", "v1.14.3",
		"suggested: patch from v1.14.2",
		"patch v1.14.3 · minor v1.15.0 · major v2.0.0 · custom…",
		"From ref", "main", "main @ 0b7d3e9 · 2 commits after v1.14.2",
		"Message (annotated)", "release: v1.14.3",
		"The tag will include:", "0b7d3e9 Merge branch 'feat/pprof'", "5c0e7d1 slots: retry on serialization failure",
		"esc cancel", "enter Create")
	if f.Name() != "v1.14.3" || f.Ref() != "main" || f.Message() != "release: v1.14.3" {
		t.Fatalf("name = %q ref = %q msg = %q", f.Name(), f.Ref(), f.Message())
	}
}

func TestCreatePatchTag(t *testing.T) {
	c := newFake()
	d, _, _ := open(t, c, i18n.EN)
	d.Key("tab", "tab", "tab", "enter")
	d.RequireView("Create tag v1.14.3 on main @ 0b7d3e9?", "Annotated tag: “release: v1.14.3”")
	if c.CallCount("Tags.Create") != 0 {
		t.Fatal("nothing is created before the confirmation")
	}
	d.Key("y")
	calls := c.CallsOf("Tags.Create")
	if len(calls) != 1 || calls[0][1] != "v1.14.3" || calls[0][2] != "main" || calls[0][3] != "release: v1.14.3" {
		t.Fatalf("Create calls = %v", calls)
	}
	cm := created(d)
	if cm == nil || cm.Tag.Name != "v1.14.3" || cm.Ref != project {
		t.Fatalf("CreatedMsg = %+v", cm)
	}
	d.RequireView("Tag v1.14.3 created")
	d.RequireNoView("New tag")
}

func TestEnterOutsideTheButtonDoesNotCreate(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, i18n.EN)
	d.Key("tab", "tab") // message
	d.Key("enter")
	if f.focus != focusButton {
		t.Fatalf("focus = %v, want the button", f.focus)
	}
	d.Key("tab") // wraps to the name
	d.Key("enter", "enter")
	if f.focus != focusMessage {
		t.Fatalf("name → ref → message by enter: %v", f.focus)
	}
	if c.CallCount("Tags.Create") != 0 {
		t.Fatal("enter outside the button must not create")
	}
}

func TestConfirmDefaultsToCancel(t *testing.T) {
	c := newFake()
	d, _, _ := open(t, c, i18n.EN)
	d.Key("tab", "tab", "tab", "enter", "enter")
	if c.CallCount("Tags.Create") != 0 {
		t.Fatal("enter in the confirm is Cancel")
	}
	d.RequireView("New tag", "enter Create")
}

func TestVariantsByArrows(t *testing.T) {
	d, _, f := open(t, newFake(), i18n.EN)
	d.Key("down")
	if f.Name() != "v1.15.0" || f.Message() != "release: v1.15.0" {
		t.Fatalf("name = %q msg = %q", f.Name(), f.Message())
	}
	d.Key("down", "up", "up")
	if f.Name() != "v1.14.3" {
		t.Fatalf("name = %q", f.Name())
	}
	d.Key("up")
	if f.Name() != "v1.14.3" {
		t.Fatalf("up at the top stays: %q", f.Name())
	}
}

func TestCustomNameStartsFromDefaultAndRejectsExisting(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, i18n.EN)
	d.Key("down", "down", "down")
	if !f.custom() || f.Name() != "v1.14.3" {
		t.Fatalf("custom = %v name = %q", f.custom(), f.Name())
	}
	d.RequireView("Name", "custom…")
	for range 7 {
		d.Key("backspace")
	}
	typeText(d, "v1.14.2")
	if f.Message() != "release: v1.14.2" {
		t.Fatalf("the message follows the typed name: %q", f.Message())
	}
	d.Key("tab", "tab", "tab", "enter")
	d.RequireView("tag v1.14.2 already exists")
	d.RequireNoView("Create tag v1.14.2")
	if f.focus != focusName {
		t.Fatalf("focus = %v, want the name", f.focus)
	}
	// ↑ leaves the custom name for a variant.
	d.Key("up")
	if f.custom() || f.Name() != "v2.0.0" {
		t.Fatalf("custom = %v name = %q", f.custom(), f.Name())
	}
}

func TestEmptyNameAndRef(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, i18n.EN)
	d.Key("down", "down", "down")
	for range 7 {
		d.Key("backspace")
	}
	d.Key("tab", "tab", "tab", "enter")
	d.RequireView("enter a tag name")
	if f.focus != focusName {
		t.Fatalf("focus = %v", f.focus)
	}
	d.Key("up", "tab")
	for range 4 {
		d.Key("backspace")
	}
	d.Key("tab", "tab", "enter")
	d.RequireView("enter a ref")
	if f.focus != focusRef || c.CallCount("Tags.Create") != 0 {
		t.Fatalf("focus = %v", f.focus)
	}
}

func TestMessageFollowsNameUntilEdited(t *testing.T) {
	d, _, f := open(t, newFake(), i18n.EN)
	d.Key("tab", "tab")
	typeText(d, "!")
	if f.Message() != "release: v1.14.3!" {
		t.Fatalf("msg = %q", f.Message())
	}
	d.Key("shift+tab", "shift+tab", "down")
	if f.Name() != "v1.15.0" || f.Message() != "release: v1.14.3!" {
		t.Fatalf("an edited message stays: name = %q msg = %q", f.Name(), f.Message())
	}
}

func TestLightweightTagWhenMessageEmpty(t *testing.T) {
	c := newFake()
	d, _, _ := open(t, c, i18n.EN)
	d.Key("tab", "tab")
	for range 16 {
		d.Key("backspace")
	}
	d.Key("tab", "enter")
	d.RequireView("Lightweight tag without a message")
	d.Key("y")
	if calls := c.CallsOf("Tags.Create"); len(calls) != 1 || calls[0][3] != "" {
		t.Fatalf("Create calls = %v", calls)
	}
}

func TestCreateErrorKeepsTheForm(t *testing.T) {
	c := newFake()
	c.FailNext("Tags.Create", &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403})
	d, _, f := open(t, c, i18n.EN)
	d.Key("down", "tab", "tab", "tab", "enter", "y")
	d.RequireView("Not enough permissions", "New tag", "v1.15.0")
	if f.creating || f.Name() != "v1.15.0" || f.Ref() != "main" {
		t.Fatalf("creating = %v name = %q ref = %q", f.creating, f.Name(), f.Ref())
	}
	if created(d) != nil {
		t.Fatal("no CreatedMsg after a failure")
	}
	// The unlocked form can be submitted again, a 400 shows the server text.
	c.FailNext("Tags.Create", &gitlab.Error{Kind: gitlab.ErrNotPossible, Status: 400, Message: "Tag v1.15.0 already exists"})
	d.Key("enter", "y")
	d.RequireView("Tag v1.15.0 already exists")
}

func TestFormIsLockedWhileCreating(t *testing.T) {
	c := newFake()
	c.SetDelay("Tags.Create", 300*time.Millisecond)
	d, _, f := open(t, c, i18n.EN)
	d.Key("tab", "tab", "tab", "enter", "y")
	if !f.creating {
		t.Fatal("the creation must be in flight")
	}
	d.RequireView("creating…")
	d.Key("esc", "tab", "x")
	d.RequireView("New tag")
	if f.focus != focusButton || f.Name() != "v1.14.3" {
		t.Fatalf("the form is locked: focus = %v name = %q", f.focus, f.Name())
	}
}

func TestAnswersOfAClosedFormAreDropped(t *testing.T) {
	c := newFake()
	env := app.NewEnv(i18n.EN)
	env.Client = c
	f := New(env, project, "main")
	tags := f.Init()()
	f.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if o, cmd := f.Update(tags); o != nil || cmd != nil {
		t.Fatalf("a closed form must ignore answers: %v %v", o, cmd)
	}
	if f.tagsLoaded {
		t.Fatal("tags applied to a closed form")
	}
	// Another instance does not take over the answers either.
	g := New(env, project, "main")
	g.Init()
	g.Update(tags)
	if g.tagsLoaded {
		t.Fatal("tags of another form applied")
	}
}

func TestRefSuggestionsAndCompare(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, i18n.EN)
	d.Key("tab")
	d.RequireNoView("feat/surge") // no list before an edit
	for range 4 {
		d.Key("backspace")
	}
	typeText(d, "feat")
	d.Send(f.DebounceMsg())
	d.RequireView("feat/surge", "feat/slots")
	d.RequireNoView("fix/race")
	d.Key("down", "up", "enter") // the first suggestion
	if f.Ref() != "feat/surge" {
		t.Fatalf("ref = %q", f.Ref())
	}
	d.RequireNoView("feat/slots") // the list is hidden until the next edit
	d.RequireView("feat/surge @ a91f2c0 · 1 commit after v1.14.2", "a91f2c0 surge: add pricing")
}

func TestRefComparisonErrorAndStaleAnswer(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, i18n.EN)
	d.Key("tab")
	typeText(d, "x")
	// Compare of an unregistered ref is empty, an injected error is shown.
	c.FailNext("Tags.Compare", &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404})
	d.Send(f.DebounceMsg())
	d.RequireView("ref not found or comparison unavailable")
	d.RequireNoView("The tag will include:")
	// A debounce tick of an earlier keystroke does nothing.
	n := c.CallCount("Tags.Compare")
	old := f.DebounceMsg()
	typeText(d, "y")
	d.Send(old)
	if c.CallCount("Tags.Compare") != n {
		t.Fatal("a superseded tick must not search")
	}
}

func TestCommitListIsCapped(t *testing.T) {
	c := newFake()
	var cs []gitlab.CommitSummary
	for i := range 12 {
		cs = append(cs, commit(fmt.Sprintf("c%06d", 12-i), fmt.Sprintf("change %d", 12-i)))
	}
	c.SetCompare(path, "v1.14.2", "main", gitlab.Comparison{Head: &cs[0], Commits: cs})
	d, _, _ := open(t, c, i18n.EN)
	d.RequireView("main @ c000012 · 12 commits after v1.14.2", "c000012 change 12", "c000003 change 3", "… 2 more")
	d.RequireNoView("change 2")
}

func TestNoNewCommits(t *testing.T) {
	c := newFake()
	c.SetCompare(path, "v1.14.2", "main", gitlab.Comparison{})
	d, _, _ := open(t, c, i18n.EN)
	d.RequireView("no new commits after v1.14.2")
}

func TestNoTagsSuggestsFirstVersion(t *testing.T) {
	c := newFake()
	c.SetTags(path)
	d, _, f := open(t, c, i18n.EN)
	d.RequireView("no semver tags · suggested v0.1.0", "patch v0.0.1 · minor v0.1.0 · major v1.0.0", "main · first tag of the project")
	if f.Name() != "v0.1.0" || f.Message() != "release: v0.1.0" {
		t.Fatalf("name = %q msg = %q", f.Name(), f.Message())
	}
	d.RequireNoView("The tag will include:")
	d.Key("tab", "tab", "tab", "enter")
	d.RequireView("Create tag v0.1.0 on main?") // no SHA known
}

func TestPrereleaseVariants(t *testing.T) {
	c := newFake()
	c.SetTags(path, gitlab.Tag{Name: "v1.14.0-rc2"}, gitlab.Tag{Name: "v1.13.4"})
	d, _, f := open(t, c, i18n.EN)
	d.RequireView("suggested: minor · release from v1.13.4", "patch v1.13.5 · minor v1.14.0 · release · major v2.0.0")
	if f.Name() != "v1.14.0" {
		t.Fatalf("name = %q", f.Name())
	}
	c.SetTags(path, gitlab.Tag{Name: "v1.16.0-rc1"}, gitlab.Tag{Name: "v1.14.2"})
	d, _, f = open(t, c, i18n.EN)
	d.RequireView("suggested: release from v1.14.2", "release v1.16.0")
	if f.Name() != "v1.16.0" {
		t.Fatalf("name = %q", f.Name())
	}
}

func TestTagsFailureLeavesCustomName(t *testing.T) {
	c := newFake()
	c.FailNext("Tags.List", &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	d, _, f := open(t, c, i18n.EN)
	d.RequireView("GitLab server error", "custom…")
	d.RequireNoView("suggested")
	typeText(d, "v9.0.0")
	if f.Name() != "v9.0.0" || f.Message() != "release: v9.0.0" {
		t.Fatalf("name = %q msg = %q", f.Name(), f.Message())
	}
	// Without the tags there is nothing to compare with.
	d.RequireNoView("first tag", "commits after")
}

func TestLoadingTagsShown(t *testing.T) {
	c := newFake()
	c.SetDelay("Tags.List", 300*time.Millisecond)
	d, _, f := open(t, c, i18n.EN)
	d.RequireView("◐ loading tags…")
	d.Key("down", "tab")
	if f.custom() || f.Name() != "" {
		t.Fatalf("no variants yet: custom = %v name = %q", f.custom(), f.Name())
	}
}

func TestRussian(t *testing.T) {
	d, _, _ := open(t, newFake(), i18n.RU)
	d.RequireView("Новый тег · platform/route-service", "предложено: patch от v1.14.2",
		"своё…", "main @ 0b7d3e9 · 2 коммита после v1.14.2", "В теге окажутся:", "enter Создать")
	d.Key("tab", "tab", "tab", "enter")
	d.RequireView("Создать тег v1.14.3 на main @ 0b7d3e9?", "Annotated-тег: «release: v1.14.3»")
}
