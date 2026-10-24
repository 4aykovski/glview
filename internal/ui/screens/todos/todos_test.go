package todos

import (
	"fmt"
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
	now    = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	anna   = gitlab.User{Username: "anna.k", Name: "Anna K"}
	dorlov = gitlab.User{Username: "d.orlov", Name: "D Orlov"}
	proj   = &gitlab.ProjectRef{ID: 42, FullPath: path}
)

func webURL(kind string, iid int) string {
	return fmt.Sprintf("https://gitlab.example.com/%s/-/%s/%d", path, kind, iid)
}

// todoFake has five pending to-dos of different targets and one done:
// review of !479, a mention in #1318, a failed pipeline of a commit, an
// epic of a group (no project), and an MR added by hand.
func todoFake() *fake.Client {
	c := fake.New()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	c.AddProject(gitlab.Project{ProjectRef: *proj, DefaultBranch: "main", WebURL: "https://gitlab.example.com/" + path})
	c.SetTodos(
		gitlab.Todo{ID: 101, Project: proj, Author: anna, Action: "review_requested", TargetType: "MergeRequest", TargetIID: 479,
			TargetTitle: "Slot allocator rewrite", TargetState: "opened", TargetURL: webURL("merge_requests", 479),
			Body: "Slot allocator rewrite", CreatedAt: ago(2 * time.Hour)},
		gitlab.Todo{ID: 102, Project: proj, Author: dorlov, Action: "mentioned", TargetType: "Issue", TargetIID: 1318,
			TargetTitle: "Surge coefficient ignores zone overrides", TargetState: "opened", TargetURL: webURL("issues", 1318) + "#note_7",
			Body: "@egor глянь, пожалуйста", CreatedAt: ago(3 * time.Hour)},
		gitlab.Todo{ID: 103, Project: proj, Author: anna, Action: "build_failed", TargetType: "Commit", TargetRef: "01234567",
			TargetTitle: "Fix slot overflow", TargetURL: "https://gitlab.example.com/" + path + "/-/commit/0123456789abcdef",
			Body: "Fix slot overflow", CreatedAt: ago(26 * time.Hour)},
		gitlab.Todo{ID: 104, Author: dorlov, Action: "okr_checkin_requested", TargetType: "Epic",
			TargetTitle: "Q4 planning", TargetState: "opened", TargetURL: "https://gitlab.example.com/groups/platform/-/epics/12",
			Body: "Q4 planning", CreatedAt: ago(30 * time.Hour)},
		gitlab.Todo{ID: 105, Project: proj, Author: anna, Action: "marked", TargetType: "MergeRequest", TargetIID: 482,
			TargetTitle: "Cache warmup on deploy", TargetState: "merged", TargetURL: webURL("merge_requests", 482),
			Body: "Cache warmup on deploy", CreatedAt: ago(48 * time.Hour)},
		gitlab.Todo{ID: 106, Project: proj, Author: dorlov, Action: "assigned", TargetType: "Issue", TargetIID: 1297, State: "done",
			TargetTitle: "Expose route cache hit ratio metric", TargetState: "opened", TargetURL: webURL("issues", 1297),
			Body: "Expose route cache hit ratio metric", CreatedAt: ago(72 * time.Hour)},
	)
	c.AddMR(path, gitlab.MergeRequest{IID: 479, Title: "Slot allocator rewrite", State: gitlab.MRStateOpened, Author: anna,
		SourceBranch: "feat/slots", TargetBranch: "main", WebURL: webURL("merge_requests", 479), UpdatedAt: ago(time.Hour)})
	c.AddIssue(path, gitlab.Issue{IID: 1318, Title: "Surge coefficient ignores zone overrides", Author: anna, UpdatedAt: ago(time.Hour), WebURL: webURL("issues", 1318)})
	return c
}

type opened struct {
	d    *uitest.Driver
	root *app.Model
	scr  *Screen
	env  *app.Env
	urls *[]string
	fake *fake.Client
}

func openLang(t *testing.T, client *fake.Client, lang i18n.Lang) *opened {
	t.Helper()
	env := app.NewEnv(lang)
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	var urls []string
	env.Browser = func(u string) error { urls = append(urls, u); return nil }
	env.Cache = cache.New()
	env.Todos = func() app.Screen { return New(env) }
	scr := New(env)
	root := app.New(env, scr, app.Options{})
	return &opened{d: uitest.New(t, root), root: root, scr: scr, env: env, urls: &urls, fake: client}
}

func open(t *testing.T, client *fake.Client) *opened { return openLang(t, client, i18n.EN) }

// requireRow fails unless a line of the view holds the text, whitespace
// collapsed.
func requireRow(t *testing.T, d *uitest.Driver, want string) {
	t.Helper()
	for _, l := range strings.Split(d.View(), "\n") {
		if strings.Contains(strings.Join(strings.Fields(l), " "), want) {
			return
		}
	}
	t.Fatalf("no row %q in:\n%s", want, d.View())
}

func lastFilter(c *fake.Client) gitlab.TodoFilter {
	calls := c.CallsOf("Todos.List")
	return calls[len(calls)-1][0].(gitlab.TodoFilter)
}

func TestListRu(t *testing.T) {
	o := openLang(t, todoFake(), i18n.RU)
	o.d.RequireView("To-Do · gitlab.example.com", "state:pending", "type:любой", "5 из 5", "Цель", "Заголовок", "Причина", "Назад", "Автор", "Проект")
	requireRow(t, o.d, "› !479 Slot allocator rewrite ревью 2ч anna.k platform/route-service")
	requireRow(t, o.d, "#1318 Surge coefficient ignores zone overrides упоминание 3ч d.orlov platform/route-service")
	requireRow(t, o.d, "01234567 Fix slot overflow пайплайн упал")
	if c := o.fake.CallCount("Todos.List"); c != 1 {
		t.Fatalf("list calls = %d", c)
	}
}

func TestListEn(t *testing.T) {
	o := open(t, todoFake())
	o.d.RequireView("To-Do · gitlab.example.com", "Target", "Title", "Action", "Ago", "Author", "Project", "5 of 5")
	requireRow(t, o.d, "› !479 Slot allocator rewrite review requested 2h anna.k platform/route-service")
	requireRow(t, o.d, "!482 Cache warmup on deploy added 2d anna.k platform/route-service")
}

func TestEpicWithoutProjectAndUnknownAction(t *testing.T) {
	o := open(t, todoFake())
	requireRow(t, o.d, "— Q4 planning okr_checkin_requested 1d d.orlov —")
}

func TestListFitsTerminal(t *testing.T) {
	o := open(t, todoFake())
	o.d.RequireFits()
	o.d.Key("j", "j")
	o.d.RequireFits()
	o.d.Key("s")
	o.d.RequireFits()
}

func TestEmptyStates(t *testing.T) {
	c := todoFake()
	c.SetTodos()
	o := open(t, c)
	o.d.RequireView("To-Do is empty", "0 of 0")
	o.d.Key("s")
	o.d.RequireView("No done to-dos")
	o.d.Key("t")
	o.d.RequireView("No to-dos for the filters · backspace — reset")
	o.d.Key("backspace")
	o.d.RequireView("To-Do is empty", "state:pending", "type:any")
}

func TestFilterTypeCycle(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("t")
	o.d.RequireView("type:MR")
	if f := lastFilter(o.fake); f.State != "pending" || f.Type != "MergeRequest" {
		t.Fatalf("filter = %+v", f)
	}
	requireRow(t, o.d, "!482 Cache warmup")
	o.d.RequireNoView("#1318")
	o.d.Key("t")
	o.d.RequireView("type:issue")
	if f := lastFilter(o.fake); f.Type != "Issue" {
		t.Fatalf("filter = %+v", f)
	}
	o.d.Key("t")
	o.d.RequireView("type:any")
	if f := o.scr.Filter(); f.Type != "" {
		t.Fatalf("filter = %+v", f)
	}
}

func TestFilterStateAndReset(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("s")
	o.d.RequireView("state:done", "1 of 1")
	requireRow(t, o.d, "#1297 Expose route cache hit ratio metric")
	o.d.RequireNoView("!479")
	if f := lastFilter(o.fake); f.State != "done" {
		t.Fatalf("filter = %+v", f)
	}
	o.d.Key("s")
	o.d.RequireView("state:pending", "!479")
	o.d.Key("s", "t")
	o.d.RequireView("state:done", "type:MR")
	o.d.Key("backspace")
	o.d.RequireView("state:pending", "type:any", "!479")
	if f := o.scr.Filter(); f.State != "pending" || f.Type != "" {
		t.Fatalf("filter = %+v", f)
	}
}

func TestMDoesNothing(t *testing.T) {
	o := open(t, todoFake())
	before := o.fake.CallCount("Todos.List")
	o.d.Key("m")
	if o.fake.CallCount("Todos.List") != before {
		t.Fatal("m must not reload the list")
	}
	o.d.RequireView("state:pending", "type:any")
}

func TestStaleAnswerOfThePreviousFilterIsDropped(t *testing.T) {
	o := open(t, todoFake())
	old := o.scr.gen.Current()
	o.d.Key("s")
	// The answer for the pending list arrives after the one for done.
	o.d.Send(loadedMsg{tok: old, page: gitlab.Page[gitlab.Todo]{Items: []gitlab.Todo{{ID: 999, TargetType: "Issue", TargetIID: 1, TargetTitle: "stale one"}}, Total: 1}})
	o.d.RequireView("state:done", "#1297")
	o.d.RequireNoView("stale one")
}

func TestPagingLoadsMoreNearTheEnd(t *testing.T) {
	c := fake.New()
	var ts []gitlab.Todo
	for i := 1; i <= 45; i++ {
		ts = append(ts, gitlab.Todo{ID: int64(i), Project: proj, Author: anna, Action: "mentioned", TargetType: "Issue", TargetIID: int64(1000 + i),
			TargetTitle: fmt.Sprintf("Issue number %d", i), CreatedAt: now.Add(-time.Duration(i) * time.Minute)})
	}
	c.SetTodos(ts...)
	o := open(t, c)
	o.d.RequireView("20 of 45")
	calls := c.CallsOf("Todos.List")
	if len(calls) != 1 || calls[0][2] != 20 {
		t.Fatalf("calls = %v", calls)
	}
	for range 16 {
		o.d.Key("j")
	}
	if got := c.CallCount("Todos.List"); got != 2 {
		t.Fatalf("list calls after scrolling = %d, want 2", got)
	}
	o.d.RequireView("40 of 45")
	// The cursor is kept by ID across the poll refresh.
	cur := o.scr.tbl.CursorID()
	o.d.Send(pollTick(o.scr))
	if o.scr.tbl.CursorID() != cur {
		t.Fatalf("cursor moved from %s to %s", cur, o.scr.tbl.CursorID())
	}
}

func pollTick(s *Screen) tea.Msg {
	return common.PollTickMsg{Token: s.poll.Current()}
}

func TestPollRefreshesOnTop(t *testing.T) {
	o := open(t, todoFake())
	before := o.fake.CallCount("Todos.List")
	o.d.Send(pollTick(o.scr))
	if o.fake.CallCount("Todos.List") != before+1 {
		t.Fatalf("a poll tick must reload the list")
	}
	// Covered by another screen the tick is dropped.
	o.d.Key("enter")
	o.d.Key("esc")
	tok := o.scr.poll.Current()
	o.d.Key("enter")
	n := o.fake.CallCount("Todos.List")
	o.d.Send(common.PollTickMsg{Token: tok})
	if o.fake.CallCount("Todos.List") != n {
		t.Fatal("a tick of a covered screen must be ignored")
	}
}

func TestPreviewMention(t *testing.T) {
	o := openLang(t, todoFake(), i18n.RU)
	o.d.Key("j")
	o.d.RequireView("Surge coefficient ignores zone overrides", "#1318 · platform/route-service · opened · 3ч", "упоминание · d.orlov", "@egor глянь, пожалуйста")
}

func TestPreviewReviewAndOpenedState(t *testing.T) {
	o := open(t, todoFake())
	o.d.RequireView("!479 · platform/route-service · opened · 2h", "review requested · anna.k")
}

func TestPreviewBodyHTMLAndTruncation(t *testing.T) {
	c := todoFake()
	long := "<p>one</p>" + strings.Repeat("<p>line</p>", 12)
	c.SetTodos(gitlab.Todo{ID: 7, Project: proj, Author: dorlov, Action: "mentioned", TargetType: "Issue", TargetIID: 5,
		TargetTitle: "Long one", TargetState: "opened", Body: long, CreatedAt: now.Add(-time.Hour)})
	o := open(t, c)
	o.d.RequireView("Long one", "mentioned · d.orlov", "one", "…")
	if strings.Contains(o.d.View(), "<p>") {
		t.Fatalf("HTML must be turned into markdown:\n%s", o.d.View())
	}
}

func TestPreviewRightFrom140Columns(t *testing.T) {
	o := open(t, todoFake())
	o.d.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	if !o.scr.previewRight() {
		t.Fatal("the preview must go right from 140 columns")
	}
	o.d.RequireView("│", "review requested · anna.k")
}

func TestEnterOpensMR(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("enter")
	if o.root.Depth() != 2 || o.root.Top().Title() != path+" !479" {
		t.Fatalf("depth %d, top %q", o.root.Depth(), o.root.Top().Title())
	}
	o.d.Key("esc")
	if o.root.Depth() != 1 {
		t.Fatal("esc must return to the To-Do screen")
	}
	requireRow(t, o.d, "› !479")
	if o.fake.CallCount("Todos.MarkDone") != 0 || o.fake.CallCount("Todos.MarkAllDone") != 0 {
		t.Fatal("opening a target must not mark the to-do done")
	}
}

func TestEnterOpensIssue(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("j", "enter")
	if o.root.Depth() != 2 || o.root.Top().Title() != path+" #1318" {
		t.Fatalf("depth %d, top %q", o.root.Depth(), o.root.Top().Title())
	}
	o.d.Key("esc")
	requireRow(t, o.d, "› #1318")
}

func TestEnterOnCommitAndEpicOpensBrowser(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("j", "j", "enter")
	o.d.Key("j", "enter")
	want := []string{"https://gitlab.example.com/" + path + "/-/commit/0123456789abcdef", "https://gitlab.example.com/groups/platform/-/epics/12"}
	if o.root.Depth() != 1 || fmt.Sprint(*o.urls) != fmt.Sprint(want) {
		t.Fatalf("depth %d, urls %v", o.root.Depth(), *o.urls)
	}
}

func TestBrowserKeys(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("O")
	if got := *o.urls; len(got) != 1 || got[0] != webURL("merge_requests", 479) {
		t.Fatalf("O: %v", got)
	}
	o.d.Key("o")
	if got := (*o.urls)[1]; got != "https://gitlab.example.com/dashboard/todos" {
		t.Fatalf("o with defaults: %q", got)
	}
	o.d.Key("s", "t", "t", "o")
	if got := (*o.urls)[2]; got != "https://gitlab.example.com/dashboard/todos?state=done&type=Issue" {
		t.Fatalf("o with filters: %q", got)
	}
	o.d.Key("backspace", "t", "o")
	if got := (*o.urls)[3]; got != "https://gitlab.example.com/dashboard/todos?type=MergeRequest" {
		t.Fatalf("o with a type: %q", got)
	}
}

func TestCopyLink(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("y")
	o.d.RequireView("…/merge_requests/479")
}

func TestMarkDone(t *testing.T) {
	o := open(t, todoFake())
	o.d.RequireView("To-Do 5")
	o.d.Key("x")
	calls := o.fake.CallsOf("Todos.MarkDone")
	if len(calls) != 1 || calls[0][0] != int64(101) {
		t.Fatalf("MarkDone calls = %v", calls)
	}
	o.d.RequireView("To-Do 4", "Done: !479", "4 of 4")
	o.d.RequireNoView("Slot allocator rewrite")
	// The cursor stays at the same position: now on the next to-do.
	requireRow(t, o.d, "› #1318")
	if o.fake.TodoState(101) != "done" {
		t.Fatal("the fake did not get the mark")
	}
}

func TestMarkDoneFailureRestoresTheRow(t *testing.T) {
	c := todoFake()
	c.FailNext("Todos.MarkDone", &gitlab.Error{Kind: gitlab.ErrNetwork})
	o := open(t, c)
	o.d.Key("j", "x")
	o.d.RequireView("To-Do 5", "Host unreachable")
	requireRow(t, o.d, "#1318 Surge coefficient ignores zone overrides mentioned")
	requireRow(t, o.d, "› #1318")
	o.d.RequireView("5 of 5")
	o.d.RequireNoView("Done:")
}

func TestRefusedRowDoesNotReturnIntoAnotherFilter(t *testing.T) {
	o := open(t, todoFake())
	o.scr.markDone() // the request is in flight: only its answer is sent by hand
	o.d.Key("s")     // meanwhile the list becomes the done one
	o.d.RequireView("state:done", "#1297")
	total := o.scr.total
	o.d.Send(doneMsg{owner: o.scr.gen.Current(), id: 101, err: &gitlab.Error{Kind: gitlab.ErrNetwork}})
	for _, it := range o.scr.Items() {
		if it.ID == 101 {
			t.Fatal("a pending to-do must not be put into the done list")
		}
	}
	if o.scr.total != total {
		t.Fatalf("total = %d, want %d", o.scr.total, total)
	}
	o.d.RequireNoView("Slot allocator rewrite")
	if !o.env.TodoKnown || o.env.TodoCount != 5 {
		t.Fatalf("counter = %d (known %v), the header is restored in any case", o.env.TodoCount, o.env.TodoKnown)
	}
}

func TestCounterSubtractsToDosBeingMarkedDone(t *testing.T) {
	o := open(t, todoFake())
	o.d.RequireView("To-Do 5")
	o.scr.markDone() // in flight; the server still counts it
	o.d.Key("r")
	o.d.RequireView("To-Do 4")
	if o.env.TodoCount != 4 {
		t.Fatalf("counter = %d, want 4", o.env.TodoCount)
	}
}

func TestMarkAllDoneConfirmWithUnknownTotal(t *testing.T) {
	c := todoFake()
	c.SetTodoTotalUnknown(true)
	o := open(t, c)
	o.d.Key("X")
	o.d.RequireView("Over 10 000 to-dos")
	o.d.RequireNoView("-1")
}

func TestFilterLineIsTranslated(t *testing.T) {
	o := openLang(t, todoFake(), i18n.RU)
	o.d.RequireView("type:любой")
	o.d.Key("t")
	o.d.RequireView("type:MR")
	o.d.Key("t")
	o.d.RequireView("type:issue")
	o = open(t, todoFake())
	o.d.RequireView("type:any")
}

func TestMarkDoneIsHiddenInDone(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("s")
	before := o.fake.CallCount("Todos.MarkDone")
	o.d.Key("x", "X")
	if o.fake.CallCount("Todos.MarkDone") != before || o.root.Overlay() != nil {
		t.Fatal("x and X must not act in state done")
	}
	for _, b := range o.scr.KeyMap().Short {
		if b.Visible() && (b.Help().Key == "x" || b.Help().Key == "X") {
			t.Fatalf("%q must be hidden in state done", b.Help().Key)
		}
	}
}

func TestMarkAllDone(t *testing.T) {
	o := open(t, todoFake())
	o.d.Key("X")
	if o.root.Overlay() == nil {
		t.Fatal("X must ask for a confirm")
	}
	o.d.RequireView("Mark all to-dos as done?", "5 to-dos in To-Do")
	// The default is Cancel.
	o.d.Key("enter")
	if o.fake.CallCount("Todos.MarkAllDone") != 0 || o.root.Overlay() != nil {
		t.Fatal("enter on the default button must cancel")
	}
	o.d.Key("X", "y")
	if o.fake.CallCount("Todos.MarkAllDone") != 1 {
		t.Fatal("MarkAllDone not called")
	}
	o.d.RequireView("To-Do is empty", "To-Do cleared")
	o.d.RequireNoView("To-Do 0", "To-Do 5")
	if !o.env.TodoKnown || o.env.TodoCount != 0 {
		t.Fatalf("counter = %d (known %v)", o.env.TodoCount, o.env.TodoKnown)
	}
}

func TestMarkAllDoneRu(t *testing.T) {
	o := openLang(t, todoFake(), i18n.RU)
	o.d.Key("X")
	o.d.RequireView("Отметить все задачи выполненными?", "5 задач в To-Do. Вернуть их в TUI нельзя.")
	o.d.Key("y")
	o.d.RequireView("В To-Do пусто", "To-Do очищен")
}

func TestCounterFromTheListAnswer(t *testing.T) {
	c := todoFake()
	o := open(t, c)
	c.AddTodo(gitlab.Todo{ID: 200, Project: proj, Author: anna, Action: "assigned", TargetType: "Issue", TargetIID: 9, TargetTitle: "New one"})
	o.d.Key("r")
	o.d.RequireView("To-Do 6")
	// A typed list does not change the counter.
	o.d.Key("t")
	o.d.RequireView("To-Do 6")
}

func TestUnknownTotalShowsNinesPlus(t *testing.T) {
	c := todoFake()
	c.SetTodoTotalUnknown(true)
	o := open(t, c)
	o.d.RequireView("To-Do 9999+")
}

func TestColdStartFailureShowsRetry(t *testing.T) {
	c := todoFake()
	c.FailNext("Todos.List", &gitlab.Error{Kind: gitlab.ErrServer})
	o := open(t, c)
	o.d.RequireView("could not load")
	o.d.Key("r")
	o.d.RequireView("!479")
	o.d.RequireNoView("could not load")
}

func TestSwitchLanguageRebuildsRows(t *testing.T) {
	o := open(t, todoFake())
	requireRow(t, o.d, "review requested")
	o.d.Key("ctrl+l")
	requireRow(t, o.d, "ревью")
	o.d.Key("ctrl+l")
}

func TestFooterKeys(t *testing.T) {
	o := open(t, todoFake())
	o.d.Send(tea.WindowSizeMsg{Width: 200, Height: 40})
	o.d.RequireView("done", "all done", "state", "type", "To-Do in browser", "target in browser", "copy")
}
