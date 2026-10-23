package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/glab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const path = "platform/route-service"

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func newEnv(client *fake.Client) *app.Env {
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	env.Browser = func(string) error { return nil }
	return env
}

func newFake(nMRs int) *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, WebURL: "https://gitlab.example.com/" + path})
	for i := 1; i <= nMRs; i++ {
		mr := gitlab.MergeRequest{
			IID: int64(i), Title: fmt.Sprintf("MR number %d", i), State: gitlab.MRStateOpened, Author: gitlab.User{Username: "anna.k"},
			SourceBranch: fmt.Sprintf("feat/%d", i), TargetBranch: "main", SHA: "sha",
			UpdatedAt: now.Add(-time.Duration(i) * time.Minute), WebURL: fmt.Sprintf("https://gitlab.example.com/%s/-/merge_requests/%d", path, i),
			MergeStatus: "mergeable", Description: "First line of the description\nsecond",
			Approvals:   gitlab.Approvals{Required: 2, ApprovedBy: []gitlab.User{{Username: "bob"}}},
			Discussions: gitlab.DiscussionStats{Total: 3, Unresolved: 1},
		}
		c.AddMR(path, mr)
	}
	return c
}

func open(t *testing.T, client *fake.Client, repo *gitremote.Repo) (*uitest.Driver, *app.Model, *Project) {
	t.Helper()
	env := newEnv(client)
	scr := New(env, Options{Path: path, Repo: repo})
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr
}

func TestProjectScreenTabsAndHeader(t *testing.T) {
	client := newFake(3)
	repo := &gitremote.Repo{Root: "/src/route-service", Branch: "fix/slot-race"}
	d, _, scr := open(t, client, repo)
	d.RequireView(path, "gitlab.example.com", "fix/slot-race", "1 MRs", "4 Tags", "@egor", "MR number 1", "state:opened")
	if scr.Current() != 0 {
		t.Fatal("MRs is the default tab")
	}
	d.Key("3")
	d.RequireView("Issues will appear here later")
	d.Key("tab")
	d.RequireView("No tags", "sort: semver ↓")
	d.Key("shift+tab", "shift+tab")
	if scr.Current() != 1 {
		t.Fatalf("current = %d", scr.Current())
	}
	d.Key("1")
	d.RequireView("MR number 1")
	if client.CallCount("MergeRequests.List") != 1 {
		t.Fatalf("returning to a loaded tab must not reload at once: %d", client.CallCount("MergeRequests.List"))
	}
}

func TestBranchRereadOnRefresh(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/.git", 0o755); err != nil {
		t.Fatal(err)
	}
	head := dir + "/.git/HEAD"
	_ = os.WriteFile(head, []byte("ref: refs/heads/main\n"), 0o644)
	repo, err := gitremote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := open(t, newFake(1), &repo)
	d.RequireView("main")
	_ = os.WriteFile(head, []byte("ref: refs/heads/feat/other\n"), 0o644)
	d.Key("r")
	d.RequireView("feat/other")
}

func TestProjectNotFound(t *testing.T) {
	env := newEnv(fake.New())
	d := uitest.New(t, app.New(env, New(env, Options{Path: "gone/project", Repo: nil}), app.Options{}))
	d.RequireView("Project not found or no access", "gone/project")
}

func TestMRsColumnsPreviewAndFilterLine(t *testing.T) {
	client := newFake(3)
	client.UpdateMR(path, 1, func(m *gitlab.MergeRequest) {
		m.Draft, m.HasConflicts = true, true
		m.Diff = &gitlab.DiffStats{Additions: 48, Deletions: 12, Files: 3}
		m.HeadPipeline = &gitlab.PipelineSummary{ID: 1, Status: gitlab.StatusRunning, Duration: 220 * time.Second}
	})
	client.UpdateMR(path, 2, func(m *gitlab.MergeRequest) {
		m.Approvals = gitlab.Approvals{ApprovedBy: []gitlab.User{{Username: "a"}, {Username: "b"}}}
	})
	d, _, _ := open(t, client, nil)
	d.RequireView("IID", "Title", "Author", "Pipeline", "Appr", "Thr", "Upd",
		"!1", "Draft: MR number 1", "⚠ conflicts", "anna.k", "feat/1 → main", "1/2", "1m",
		"state:opened", "scope:all", "reviewer:any", "draft:any", "3 of 3",
		// preview of the first row
		"!1  MR number 1", "● mergeable", "ready to merge", "1/2 (bob)", "pipeline #1", "3m40s", "+48", "−12", "3 files", "First line of the description")
	// CE: two approvals, none required -> "2", not "2/0".
	v := d.View()
	if !containsLine(v, "!2", " 2 ") || containsLine(v, "!2", "2/0") {
		t.Fatalf("CE approvals:\n%s", v)
	}
}

func containsLine(view string, subs ...string) bool {
	for _, l := range splitLines(view) {
		ok := true
		for _, s := range subs {
			if !contains(l, s) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func TestNarrowTerminalHidesPreviewAndColumns(t *testing.T) {
	d, _, _ := open(t, newFake(3), nil)
	d.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	d.RequireView("MR number 1", "IID")
	d.RequireNoView("ready to merge")
	for _, l := range splitLines(d.View()) {
		if len([]rune(l)) > 80 {
			t.Fatalf("line wider than the terminal: %q", l)
		}
	}
}

func TestMRsPaginationOnCursorNearEnd(t *testing.T) {
	client := newFake(45)
	d, _, scr := open(t, client, nil)
	mrs := scr.tabs[0].(*mrsTab)
	if len(mrs.Items()) != 20 {
		t.Fatalf("first page = %d", len(mrs.Items()))
	}
	d.Key("j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j") // 15th row
	if client.CallCount("MergeRequests.List") != 1 {
		t.Fatalf("no request before the cursor is within 5 rows of the end: %d", client.CallCount("MergeRequests.List"))
	}
	d.Key("j") // 16th row
	if len(mrs.Items()) != 40 || client.CallCount("MergeRequests.List") != 2 {
		t.Fatalf("items=%d calls=%d", len(mrs.Items()), client.CallCount("MergeRequests.List"))
	}
	d.RequireView("40 of 45")
}

func TestMRsNextPageErrorKeepsRowsAndRetries(t *testing.T) {
	client := newFake(45)
	d, _, scr := open(t, client, nil)
	client.FailNext("MergeRequests.List", &gitlab.Error{Kind: gitlab.ErrNetwork})
	d.Key("G")
	mrs := scr.tabs[0].(*mrsTab)
	if len(mrs.Items()) != 20 {
		t.Fatalf("loaded rows must stay: %d", len(mrs.Items()))
	}
	d.RequireView("✗ could not load, r — retry")
	d.Key("r")
	if len(mrs.Items()) != 40 {
		t.Fatalf("r must retry the page: %d", len(mrs.Items()))
	}
	d.RequireNoView("✗ could not load")
}

func TestMRsFilterInput(t *testing.T) {
	client := newFake(3)
	client.UpdateMR(path, 2, func(m *gitlab.MergeRequest) { m.Reviewers = []gitlab.User{client.User} })
	d, _, scr := open(t, client, nil)
	mrs := scr.tabs[0].(*mrsTab)

	d.Key("/")
	if !scr.CapturesInput() {
		t.Fatal("the filter input must capture keys")
	}
	d.Key("q") // typed, not quit
	if d.Quit {
		t.Fatal("q quit while typing")
	}
	d.Key("backspace")
	for _, r := range "reviewer:@me" {
		d.Key(string(r))
	}
	d.Key("enter")
	if scr.CapturesInput() {
		t.Fatal("enter closes the input")
	}
	var last fake.Call
	for _, c := range client.Calls() {
		if c.Method == "MergeRequests.List" {
			last = c
		}
	}
	f := last.Args[1].(gitlab.MRFilter)
	if f.Reviewer != "@me" {
		t.Fatalf("filter sent to the client: %+v", f)
	}
	d.RequireView("reviewer:@me", "MR number 2", "1 of 1")
	if len(mrs.Items()) != 1 {
		t.Fatalf("items = %d", len(mrs.Items()))
	}

	// Free text and state.
	d.Key("/")
	d.Key("ctrl+u") // clear is not bound in all builds; fall back to backspaces below
	for range 40 {
		d.Key("backspace")
	}
	for _, r := range "state:merged surge" {
		d.Key(string(r))
	}
	d.Key("enter")
	for _, c := range client.Calls() {
		if c.Method == "MergeRequests.List" {
			last = c
		}
	}
	f = last.Args[1].(gitlab.MRFilter)
	if f.State != gitlab.MRStateMerged || f.Search != "surge" {
		t.Fatalf("filter = %+v", f)
	}
	d.RequireView("No merged MRs")
}

func TestMRsFilterErrors(t *testing.T) {
	client := newFake(2)
	d, _, _ := open(t, client, nil)
	calls := client.CallCount("MergeRequests.List")
	d.Key("/")
	for _, r := range "foo:bar" {
		d.Key(string(r))
	}
	d.Key("enter")
	d.RequireView("unknown key \"foo\"")
	if client.CallCount("MergeRequests.List") != calls {
		t.Fatal("a rejected filter must not be applied")
	}
	d.Key("esc")
	d.RequireNoView("unknown key")
	d.RequireView("state:opened")
}

func TestMRsMeFilterToggle(t *testing.T) {
	client := newFake(3)
	client.UpdateMR(path, 1, func(m *gitlab.MergeRequest) { m.Author = client.User })
	client.UpdateMR(path, 1, func(m *gitlab.MergeRequest) { m.Reviewers = []gitlab.User{client.User} })
	d, _, scr := open(t, client, nil)
	d.Key("m")
	mrs := scr.tabs[0].(*mrsTab)
	if !mrs.filter.Mine || len(mrs.Items()) != 1 {
		t.Fatalf("mine=%v items=%d", mrs.filter.Mine, len(mrs.Items()))
	}
	d.Key("m")
	if mrs.filter.Mine || len(mrs.Items()) != 3 {
		t.Fatalf("mine=%v items=%d", mrs.filter.Mine, len(mrs.Items()))
	}
}

func TestMRsEmptyState(t *testing.T) {
	client := newFake(2)
	d, _, _ := open(t, client, nil)
	d.Key("/")
	for _, r := range "reviewer:@me" {
		d.Key(string(r))
	}
	d.Key("enter")
	d.RequireView("No open MRs where you are the reviewer", "/ changes filters")
	d.Key("ctrl+l")
	d.RequireView("Нет открытых MR, где ты ревьюер", "m — фильтр «я»", "Заголовок")
}

func TestMRsOpenAndCopy(t *testing.T) {
	client := newFake(2)
	env := newEnv(client)
	var opened string
	env.Browser = func(u string) error { opened = u; return nil }
	d := uitest.New(t, app.New(env, New(env, Options{Path: path, Repo: nil}), app.Options{}))
	d.Key("O")
	if opened != "https://gitlab.example.com/"+path+"/-/merge_requests/1" {
		t.Fatalf("opened %q", opened)
	}
	d.Key("o")
	if opened != "https://gitlab.example.com/"+path+"/-/merge_requests" {
		t.Fatalf("o opens the list: %q", opened)
	}
	d.Key("y")
	d.RequireView("copied: …/merge_requests/1")
}

func TestStaleResponseIsDropped(t *testing.T) {
	client := newFake(3)
	client.UpdateMR(path, 3, func(m *gitlab.MergeRequest) { m.State = gitlab.MRStateMerged })
	env := newEnv(client)
	scr := New(env, Options{Path: path, Repo: nil})
	mrs := scr.tabs[0].(*mrsTab)
	mrs.SetSize(120, 30)
	feed := func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		scr.Update(cmd())
	}

	// First request (opened) is slow: its command runs only after the second.
	first := mrs.loadFirst(true, false)
	mrs.filter.State = gitlab.MRStateMerged
	second := mrs.loadFirst(true, false)
	feed(second)
	feed(first) // late answer to the old filter
	if len(mrs.Items()) != 1 || mrs.Items()[0].IID != 3 {
		t.Fatalf("the second filter's result must win, got %+v", mrs.Items())
	}
}

func TestPollingAndAuthLost(t *testing.T) {
	client := newFake(2)
	d, root, scr := open(t, client, nil)
	mrs := scr.tabs[0].(*mrsTab)
	base := client.CallCount("MergeRequests.List")

	d.Send(common.PollTickMsg{Token: mrs.poll.Current()})
	if client.CallCount("MergeRequests.List") != base+1 {
		t.Fatal("a valid tick must refresh")
	}
	// 401 on the next poll: mark in the header, no more ticks.
	client.FailNext("MergeRequests.List", &gitlab.Error{Kind: gitlab.ErrUnauthorized, Status: 401})
	d.Send(common.PollTickMsg{Token: mrs.poll.Current()})
	if !root.Env().AuthLost {
		t.Fatal("401 must set AuthLost")
	}
	d.RequireView("✗ auth", "Token rejected")
	n := client.CallCount("MergeRequests.List")
	d.Send(common.PollTickMsg{Token: mrs.poll.Current()})
	if client.CallCount("MergeRequests.List") != n {
		t.Fatal("no requests while the token is rejected")
	}
	// r checks the token, then the list reloads.
	d.Key("r")
	if root.Env().AuthLost {
		t.Fatal("successful re-check clears the mark")
	}
	if client.CallCount("MergeRequests.List") <= n {
		t.Fatal("the list must reload after auth is restored")
	}
	d.RequireNoView("✗ auth")
}

func TestPollOnlyOnTopAndOnFocusReturn(t *testing.T) {
	client := newFake(2)
	d, root, scr := open(t, client, nil)
	mrs := scr.tabs[0].(*mrsTab)
	stale := mrs.poll.Current()

	d.Send(app.PushMsg{Screen: newDummy(root.Env())})
	base := client.CallCount("MergeRequests.List")
	d.Send(common.PollTickMsg{Token: stale})
	if client.CallCount("MergeRequests.List") != base {
		t.Fatal("a covered screen must not poll")
	}
	d.Key("esc")

	// Focus returns after a long absence: refresh at once.
	root.Env().Now = func() time.Time { return now.Add(10 * time.Minute) }
	before := client.CallCount("MergeRequests.List")
	d.Send(tea.BlurMsg{})
	d.Send(tea.FocusMsg{})
	if client.CallCount("MergeRequests.List") <= before {
		t.Fatal("focus return must refresh a stale list immediately")
	}
}

func TestEnterOpensDetailAndEscReturns(t *testing.T) {
	client := newFake(3)
	d, root, _ := open(t, client, nil)
	d.Key("j", "enter")
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.RequireView(path+" › MRs › !2", "MR number 2", "Overview")
	d.Key("esc")
	d.RequireView("› !2", "1 MRs")
}

func TestApproveFromList(t *testing.T) {
	client := newFake(2)
	d, _, scr := open(t, client, nil)
	client.ResetCalls()
	d.Key("a")
	calls := client.Calls()
	if calls[0].Method != "MergeRequests.Approve" || calls[0].Args[1] != int64(1) || calls[0].Args[2] != "sha" {
		t.Fatalf("calls = %+v", calls)
	}
	d.RequireView("approved !1")
	if !scr.tabs[0].(*mrsTab).Items()[0].Approvals.ApprovedByMe {
		t.Fatal("the list must reload after the action")
	}
	d.Key("a")
	found := false
	for _, c := range client.Calls() {
		found = found || c.Method == "MergeRequests.Unapprove"
	}
	if !found {
		t.Fatal("approving twice must unapprove")
	}
	d.RequireView("approval removed !1")
}

func TestMergeFromListRemovesRow(t *testing.T) {
	client := newFake(2)
	client.UpdateMR(path, 1, func(m *gitlab.MergeRequest) { m.MergeStatus = "mergeable" })
	d, _, scr := open(t, client, nil)
	d.Key("M")
	d.RequireView("Merge !1")
	d.Key("enter")
	d.RequireView("merged !1")
	items := scr.tabs[0].(*mrsTab).Items()
	if len(items) != 1 || items[0].IID != 2 {
		t.Fatalf("merged MR must leave the opened list: %+v", items)
	}
}

func tempRepo(t *testing.T) (*gitremote.Repo, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/.git", 0o755); err != nil {
		t.Fatal(err)
	}
	head := dir + "/.git/HEAD"
	_ = os.WriteFile(head, []byte("ref: refs/heads/main\n"), 0o644)
	repo, err := gitremote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return &repo, head
}

func fakeGlab(t *testing.T, loginOK bool) *glab.Glab {
	t.Helper()

	script := "#!/bin/sh\nexit 0\n"
	if !loginOK {
		script = "#!/bin/sh\nexit 1\n"
	}
	p := t.TempDir() + "/glab"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	g := glab.Detect(p, false)
	g.Timeout = 10 * time.Second
	// The first run of a fresh script is slow on macOS; warm it up so the
	// driver's short command timeout does not drop the login check.
	g.LoggedIn(context.Background(), "warmup")
	return g
}

func TestCheckoutHiddenWithoutGlab(t *testing.T) {
	client := newFake(1)
	repo, _ := tempRepo(t)
	env := newEnv(client)
	ran := false
	env.Exec = func(*exec.Cmd, func(error) tea.Msg) tea.Cmd { ran = true; return nil }
	root := app.New(env, New(env, Options{Path: path, Repo: repo}), app.Options{})
	d := uitest.New(t, root)
	d.RequireNoView("checkout")
	d.Key("C")
	if ran {
		t.Fatal("C without glab must do nothing")
	}
	d.Key("?")
	d.RequireNoView("checkout")
}

func TestCheckoutHiddenWithoutRepo(t *testing.T) {
	env := newEnv(newFake(1))
	env.Glab = fakeGlab(t, true)
	d := uitest.New(t, app.New(env, New(env, Options{Path: path, Repo: nil}), app.Options{}))
	d.RequireNoView("checkout")
}

func TestCheckoutRunsGlabInRepoAndRefreshesBranch(t *testing.T) {
	client := newFake(2)
	repo, head := tempRepo(t)
	env := newEnv(client)
	env.Glab = fakeGlab(t, true)
	var got *exec.Cmd
	env.Exec = func(c *exec.Cmd, done func(error) tea.Msg) tea.Cmd {
		got = c
		return func() tea.Msg {
			_ = os.WriteFile(head, []byte("ref: refs/heads/feat/2\n"), 0o644) // what glab does
			return done(nil)
		}
	}
	d := uitest.New(t, app.New(env, New(env, Options{Path: path, Repo: repo}), app.Options{}))
	d.RequireView("checkout", "main")
	d.Key("j", "C")
	if got == nil {
		t.Fatal("glab was not started")
	}
	args := strings.Join(got.Args[1:], " ")
	if args != "mr checkout 2" || got.Dir != repo.Root {
		t.Fatalf("args=%q dir=%q", args, got.Dir)
	}
	d.RequireView("feat/2")
	if env.GlabLoggedIn == nil || !*env.GlabLoggedIn {
		t.Fatal("login check must be cached")
	}
	n := 0
	env.Exec = func(c *exec.Cmd, done func(error) tea.Msg) tea.Cmd { n++; return nil }
	d.Key("C")
	if n != 1 {
		t.Fatal("second checkout must reuse the cached login state")
	}
}

func TestCheckoutNotLoggedIn(t *testing.T) {
	env := newEnv(newFake(1))
	env.Glab = fakeGlab(t, false)
	repo, _ := tempRepo(t)
	ran := false
	env.Exec = func(*exec.Cmd, func(error) tea.Msg) tea.Cmd { ran = true; return nil }
	d := uitest.New(t, app.New(env, New(env, Options{Path: path, Repo: repo}), app.Options{}))
	d.Key("C")
	if ran {
		t.Fatal("glab mr checkout must not start")
	}
	d.RequireView("glab is not logged in", "glab auth login --hostname gitlab.example.com")
}

func TestCheckoutNonZeroExit(t *testing.T) {
	env := newEnv(newFake(1))
	env.Glab = fakeGlab(t, true)
	repo, _ := tempRepo(t)
	env.Exec = func(c *exec.Cmd, done func(error) tea.Msg) tea.Cmd {
		return func() tea.Msg { return done(exec.Command("sh", "-c", "exit 3").Run()) }
	}
	d := uitest.New(t, app.New(env, New(env, Options{Path: path, Repo: repo}), app.Options{}))
	d.Key("C")
	d.RequireView("glab exited with an error (code 3)")
}
