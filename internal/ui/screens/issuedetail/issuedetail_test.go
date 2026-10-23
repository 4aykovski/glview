package issuedetail

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const path = "platform/route-service"

var (
	project = gitlab.ProjectRef{ID: 42, FullPath: path}
	now     = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	anna    = gitlab.User{Username: "anna.k"}
	egor    = gitlab.User{Username: "egor"}
	dorlov  = gitlab.User{Username: "d.orlov"}
)

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

// below is the screen under the issue screen, as the list is in the app.
type below struct{}

func (b *below) Init() tea.Cmd                        { return nil }
func (b *below) Update(tea.Msg) (app.Screen, tea.Cmd) { return b, nil }
func (b *below) View() string                         { return "LIST" }
func (b *below) Title() string                        { return "list" }
func (b *below) KeyMap() common.KeyMap                { return common.KeyMap{} }
func (b *below) SetSize(int, int)                     {}

func note(id int64, u gitlab.User, body string, ago time.Duration) gitlab.Note {
	return gitlab.Note{ID: id, Author: u, Body: body, CreatedAt: now.Add(-ago)}
}

func newFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: project, WebURL: "https://gitlab.example.com/" + path})
	c.AddIssue(path, gitlab.Issue{IID: 1318, Title: "Surge coefficient ignores zone overrides", Author: anna, Assignees: []gitlab.User{egor},
		Labels: []string{"bug", "pricing"}, Milestone: "2026-Q4", UserNotesCount: 2, MergeRequestsCount: 1, UpdatedAt: now,
		WebURL:      "https://gitlab.example.com/" + path + "/-/issues/1318",
		Description: "Zones with a manual override use the global coefficient.\n\n1. set override 1.5\n2. ask for an estimate"})
	c.AddIssue(path, gitlab.Issue{IID: 1300, Title: "Quiet issue", Author: anna, UpdatedAt: now})
	c.SetRelatedMRs(path, 1318, gitlab.RelatedMR{IID: 479, Title: "Fix slot race", State: gitlab.MRStateOpened})
	c.SetIssueDiscussions(path, 1318, []gitlab.Discussion{
		{ID: "t1", Notes: []gitlab.Note{note(1, dorlov, "Override is read before zone cache invalidation.", time.Hour), note(2, egor, "Confirmed, will fix.", 50*time.Minute)}},
		{ID: "t2", Notes: []gitlab.Note{note(3, egor, "Take it, related to !482.", 20*time.Minute)}},
		{ID: "t3", Notes: []gitlab.Note{{ID: 4, System: true, Author: egor, Body: "assigned to @egor", CreatedAt: now}}},
	})
	return c
}

type screen struct {
	d    *uitest.Driver
	root *app.Model
	scr  *Screen
	c    *fake.Client
	urls *[]string
}

func open(t *testing.T, c *fake.Client, iid int64, archived bool, lang i18n.Lang) *screen {
	t.Helper()
	env := app.NewEnv(lang)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	env.Cache = cache.New()
	var urls []string
	env.Browser = func(u string) error { urls = append(urls, u); return nil }
	is, err := c.Issues().Get(t.Context(), project, iid)
	if err != nil {
		t.Fatal(err)
	}
	root := app.New(env, &below{}, app.Options{})
	d := uitest.New(t, root)
	c.ResetCalls()
	scr := New(env, project, is, archived)
	d.Send(app.PushMsg{Screen: scr})
	return &screen{d: d, root: root, scr: scr, c: c, urls: &urls}
}

func TestScreenShowsIssueAndThreads(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	s.d.RequireView("platform/route-service › Issues › #1318", "Surge coefficient ignores zone overrides",
		"● open · #1318 · anna.k · assignee: egor · milestone: 2026-Q4", "Labels: bug · pricing",
		"→ !479 Fix slot race · opened", "Zones with a manual override use the global coefficient.",
		"Comments 2", "d.orlov", "Override is read before zone cache invalidation.", "Confirmed, will fix.", "Take it, related to !482.")
	s.d.RequireNoView("assigned to @egor") // system notes are hidden
	if n := s.c.CallCount("Issues.Get"); n != 1 {
		t.Fatalf("Get calls = %d", n)
	}
	if s.c.CallCount("Issues.Discussions") != 1 || s.c.CallCount("Issues.RelatedMRs") != 1 {
		t.Fatal("discussions and related MRs are asked once")
	}
}

func TestOpenJAndBack(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	s.d.Key("J")
	if c, _ := s.scr.threads.Current(); c.ID != "t2" {
		t.Fatalf("current = %q", c.ID)
	}
	s.d.Key("K")
	if c, _ := s.scr.threads.Current(); c.ID != "t1" {
		t.Fatalf("current = %q", c.ID)
	}
	s.d.Key("esc")
	s.d.RequireView("LIST")
}

func TestQuietIssueHasNoCommentsHint(t *testing.T) {
	s := open(t, newFake(), 1300, false, i18n.RU)
	s.d.RequireView("Комментариев нет · c — написать", "нет описания")
}

// merge_requests_count only counts the MRs that close the issue: a
// "Related to" MR is found by the always-sent related_merge_requests.
func TestRelatedMRsAreAskedEvenWithZeroCount(t *testing.T) {
	c := newFake()
	c.AddIssue(path, gitlab.Issue{IID: 1301, Title: "Mentioned only", Author: anna, UpdatedAt: now})
	c.SetRelatedMRs(path, 1301, gitlab.RelatedMR{IID: 479, Title: "Fix slot race", State: gitlab.MRStateOpened})
	s := open(t, c, 1301, false, i18n.EN)
	s.d.RequireView("→ !479 Fix slot race")
	if c.CallCount("Issues.RelatedMRs") != 1 {
		t.Fatal("related MRs are asked once, whatever the count says")
	}
}

func TestSpaceFoldsCurrentThreadAndKeepsItOnReload(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	s.d.Key("J", "space")
	s.d.RequireView("▸")
	s.d.Key("r")
	s.d.RequireView("▸")
	if c, _ := s.scr.threads.Current(); c.ID != "t2" {
		t.Fatalf("current = %q", c.ID)
	}
}

func TestScrollKeepsPositionOnPoll(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	s.d.Send(tea.WindowSizeMsg{Width: 120, Height: 14})
	s.d.Key("j", "j", "j")
	before := s.scr.scroll
	s.d.Send(common.PollTickMsg{Token: s.scr.poll.Current()})
	if before == 0 || s.scr.scroll != before || s.c.CallCount("Issues.Get") != 2 {
		t.Fatalf("scroll %d → %d, Get calls %d", before, s.scr.scroll, s.c.CallCount("Issues.Get"))
	}
	if s.c.CallCount("Issues.RelatedMRs") != 1 {
		t.Fatal("a tick leaves the related MRs to their cache")
	}
}

func TestStaleAnswerIsDropped(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	old := s.scr.discGen.Current()
	s.scr.discGen.Next()
	s.d.Send(discLoadedMsg{tok: old, discs: nil})
	s.d.RequireView("Take it, related to !482.")
}

func TestCommentFromTheScreen(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	s.d.Key("c")
	s.d.RequireView("Comment · #1318")
	for _, r := range "Last word" {
		s.d.Key(string(r))
	}
	s.d.Key("alt+enter")
	s.d.RequireView("Comment added", "Comments 3", "Last word")
	if c, _ := s.scr.threads.Current(); c.Link == "" || !strings.Contains(c.Link, "#note_") {
		t.Fatalf("current thread must be the new one: %+v", c)
	}
}

func TestCloseAndReopenFromTheScreen(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	s.d.Key("x")
	s.d.RequireView("Close issue #1318?")
	s.d.Key("y")
	s.d.RequireView("Issue #1318 closed", "○ closed · #1318")
	s.d.Key("x")
	s.d.RequireView("Issue #1318 reopened", "● open · #1318")
}

func TestArchivedProjectHidesCommentAndClose(t *testing.T) {
	s := open(t, newFake(), 1318, true, i18n.EN)
	s.d.Key("c", "x")
	s.d.RequireNoView("Comment · #", "Close issue")
	for _, b := range s.scr.KeyMap().Short {
		if h := b.Help(); (h.Key == "c" || h.Key == "x") && b.Visible() {
			t.Fatalf("%s must be hidden", h.Key)
		}
	}
}

func TestOpenAndCopyIssueLink(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	s.d.Key("o", "y")
	if got := *s.urls; len(got) != 1 || got[0] != "https://gitlab.example.com/"+path+"/-/issues/1318" {
		t.Fatalf("urls = %v", got)
	}
	s.d.RequireView("copied: …/issues/1318")
}

func TestGolden(t *testing.T) {
	s := open(t, newFake(), 1318, false, i18n.EN)
	s.d.Key("J")
	s.d.RequireGoldenSizes("issue")
}

func TestGoldenRU(t *testing.T) {
	s := open(t, newFake(), 1300, false, i18n.RU)
	s.d.RequireGolden("issue-empty-ru")
}
