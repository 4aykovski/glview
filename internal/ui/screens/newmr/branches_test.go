package newmr

import (
	"context"
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitexec"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/localgit"
)

// setBranches puts the branches into the fields as typing does, without the
// pause, and asks the check.
func (o *opened) setBranches(src, dst string) {
	o.t.Helper()
	o.f.src.in.SetValue(src)
	o.f.dst.in.SetValue(dst)
	o.f.markPair()
}

func TestStatusLineComparison(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.d.RequireView("3 commits · 5 files")
	o = open(t, newFake(), i18n.RU, "feat/surge")
	o.d.RequireView("3 коммита · 5 файлов")
}

func TestStatusLineBranchNotPushed(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/new")
	if src, _ := o.f.Branches(); src != "feat/new" {
		t.Fatalf("src = %q, want the local branch even if the server lacks it", src)
	}
	o.d.RequireView("branch feat/new is not on the server — git push first")
	o = open(t, newFake(), i18n.RU, "feat/new")
	o.d.RequireView("ветки feat/new нет на сервере — сначала git push")
}

func TestStatusLineUnknownBranch(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.setBranches("feat/typo", "main")
	o.d.Run(o.f.startCheck())
	o.d.RequireView("branch not found or comparison unavailable")
	o.d.RequireNoView("git push")
}

func TestStatusLineNoChanges(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/slots") // known, but nothing registered ahead of main
	o.d.RequireView("no changes: feat/slots is not ahead of main")
}

func TestStatusLineDuplicate(t *testing.T) {
	c := newFake()
	c.AddMR(path, gitlab.MergeRequest{IID: 482, Title: "Fix slot race", State: gitlab.MRStateOpened, SourceBranch: "feat/surge", TargetBranch: "main"})
	o := open(t, c, i18n.EN, "feat/surge")
	o.d.RequireView("an MR from feat/surge is already open: !482")
	o = open(t, c, i18n.RU, "feat/surge")
	o.d.RequireView("из feat/surge уже открыт !482")
}

type abRunner struct{ ahead int }

func (abRunner) Run(context.Context, gitexec.Op, []gitremote.Repo) <-chan gitexec.Result { return nil }
func (r abRunner) AheadBehind(context.Context, gitremote.Repo) (int, int, error) {
	return r.ahead, 0, nil
}

func TestStatusLineUnpushedCommits(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.env.Git = localgit.New(abRunner{ahead: 2})
	msg := o.env.Git.CountAheadBehind([]*gitremote.Repo{repoOn("feat/surge")})()
	o.env.Git.Handle(msg)
	o.d.RequireView("2 commits not pushed — they will not be in the MR")
	o.env.Lang = i18n.RU
	o.env.Msg = i18n.Get(i18n.RU)
	o.d.RequireView("2 коммита не запушены — в MR их не будет")
}

func TestBranchChangedDuringCheck(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/a")
	o.setBranches("feat/a", "main")
	stale := o.f.startCheck()() // the answer for feat/a, held back
	o.setBranches("feat/b", "main")
	fresh := o.f.startCheck()()
	o.d.Send(stale)
	o.d.RequireNoView("2 commits · 2 files")
	o.d.Send(fresh)
	o.d.RequireView("2 commits · 4 files")
}

func TestAnswerOfTheClosedFormIsDropped(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.setBranches("feat/b", "main")
	old := o.f.startCheck()() // in flight when the form is closed
	o.d.Key("esc")
	o2 := open(t, newFake(), i18n.EN, "feat/surge")
	o2.d.Send(old)
	o2.d.RequireView("3 commits · 5 files")
	o2.d.RequireNoView("2 commits · 4 files")
}

func TestBranchSuggestions(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.d.Key("ctrl+u")
	o.typeText("feat/s")
	o.tick()
	o.d.RequireView("› feat/surge", "feat/slots")
	o.d.Key("down", "enter")
	if src, _ := o.f.Branches(); src != "feat/slots" {
		t.Fatalf("src = %q, want the second suggestion", src)
	}
	o.d.RequireNoView("› feat/surge")
	o.d.RequireView("no changes: feat/slots is not ahead of main")
}

func TestEnterWithoutSuggestionsGoesToTheNextField(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.d.Key("enter")
	if o.f.Focus() != fDst {
		t.Fatalf("focus = %d", o.f.Focus())
	}
}

func TestValidation(t *testing.T) {
	c := newFake()
	cases := []struct {
		name  string
		setup func(o *opened)
		want  string
		focus fieldID
	}{
		{"no source", func(o *opened) { o.f.src.in.SetValue("") }, "enter a branch", fSrc},
		{"no target", func(o *opened) { o.f.dst.in.SetValue("") }, "enter a branch", fDst},
		{"same branch", func(o *opened) { o.f.dst.in.SetValue("feat/surge") }, "branches are the same", fDst},
		{"no title", func(o *opened) { o.f.title.SetValue("  ") }, "enter a title", fTitle},
	}
	for _, tc := range cases {
		o := open(t, c, i18n.EN, "feat/surge")
		tc.setup(o)
		o.f.markPair()
		o.f.titleFollows = false
		if tc.focus == fTitle {
			o.f.title.SetValue("  ")
		}
		o.focusOn(fButton)
		o.d.Key("enter")
		o.d.RequireView(tc.want)
		if o.f.Focus() != tc.focus {
			t.Errorf("%s: focus = %d, want %d", tc.name, o.f.Focus(), tc.focus)
		}
		if n := len(o.creates()); n != 0 {
			t.Errorf("%s: Create called %d times", tc.name, n)
		}
	}
}

func TestTitleEditStopsFollowing(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.focusOn(fTitle)
	o.typeText("!")
	o.setBranches("fix/race", "main")
	if got := o.f.TitleText(); got != "Feat/surge!" {
		t.Fatalf("title = %q, an edited title does not follow", got)
	}
}

func TestSuggestionsTitleOfTheOnlyCommit(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "fix/race")
	if got := o.f.TitleText(); got != "Fix race in courier slot allocator" {
		t.Fatalf("title = %q", got)
	}
	o = open(t, newFake(), i18n.EN, "feat/surge")
	if got := o.f.TitleText(); got != "Feat/surge" {
		t.Fatalf("title = %q", got)
	}
	if !strings.Contains(humanize("feat/surge-pricing"), "Feat/surge pricing") {
		t.Fatal("humanize")
	}
}
