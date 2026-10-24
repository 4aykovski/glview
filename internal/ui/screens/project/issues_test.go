package project

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
	"github.com/4aykovski/glview/internal/ui/screens/comment"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

var (
	anna   = gitlab.User{Username: "anna.k", Name: "Anna K"}
	egor   = gitlab.User{Username: "egor", Name: "Egor"}
	dorlov = gitlab.User{Username: "d.orlov", Name: "D Orlov"}
)

// issuesFake has five open issues and one closed issue of project
// platform/route-service; #1312 has a related MR, #1318 two comments.
func issuesFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, DefaultBranch: "main", WebURL: "https://gitlab.example.com/" + path})
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	day := 24 * time.Hour
	url := func(iid int) string { return fmt.Sprintf("https://gitlab.example.com/%s/-/issues/%d", path, iid) }
	c.AddIssue(path, gitlab.Issue{IID: 1318, Title: "Surge coefficient ignores zone overrides", Author: anna, Assignees: []gitlab.User{egor},
		Labels: []string{"bug", "pricing"}, Milestone: "2026-Q4", UserNotesCount: 2, UpdatedAt: ago(20 * time.Minute), WebURL: url(1318),
		Description: "Zones with a manual override use the global coefficient.\n\n### Steps\n\n1. set override 1.5 for nsk-center\n2. ask for an estimate at peak hours\n3. get 1.35 instead of 1.5"})
	c.AddIssue(path, gitlab.Issue{IID: 1312, Title: "Duplicate slot assignment under load", Author: dorlov, Labels: []string{"bug", "slots"},
		MergeRequestsCount: 1, UpdatedAt: ago(time.Hour), WebURL: url(1312), Description: "Two couriers get one slot."})
	c.AddIssue(path, gitlab.Issue{IID: 1297, Title: "Expose route cache hit ratio metric", Author: egor, Labels: []string{"observ."}, UpdatedAt: ago(3 * day), WebURL: url(1297)})
	c.AddIssue(path, gitlab.Issue{IID: 1281, Title: "Migrate geo client to gRPC v2 API", Author: anna, Labels: []string{"tech-debt"}, UpdatedAt: ago(7 * day), WebURL: url(1281)})
	c.AddIssue(path, gitlab.Issue{IID: 1260, Title: "Courier ETA drifts after midnight UTC+7", Author: anna, Labels: []string{"bug"}, UpdatedAt: ago(14 * day), WebURL: url(1260)})
	c.AddIssue(path, gitlab.Issue{IID: 1204, Title: "Old report about stale zones", State: "closed", Author: anna, Labels: []string{"bug"}, UpdatedAt: ago(30 * day), WebURL: url(1204)})
	c.SetRelatedMRs(path, 1312, gitlab.RelatedMR{IID: 479, Title: "Fix slot race", State: gitlab.MRStateOpened})
	c.SetIssueNotes(path, 1318,
		gitlab.Note{ID: 1, Author: dorlov, Body: "Override is read before zone cache invalidation.", CreatedAt: ago(time.Hour)},
		gitlab.Note{ID: 2, Author: egor, Body: "I take it, related to !482 — will fix it there.", CreatedAt: ago(20 * time.Minute)})
	c.SetMembers(path, anna, egor, dorlov)
	c.SetLabels(path, "bug", "pricing", "slots")
	c.SetMilestones(path, "2026-Q4", "2026-Q3", "backlog")
	return c
}

type opened struct {
	d    *uitest.Driver
	root *app.Model
	scr  *Project
	tab  *issuesTab
	env  *app.Env
	urls *[]string
	fake *fake.Client
}

func openIssues(t *testing.T, client *fake.Client, setup func(*app.Env)) *opened {
	t.Helper()
	env := newEnv(client)
	var urls []string
	env.Browser = func(u string) error { urls = append(urls, u); return nil }
	env.Cache = cache.New()
	if setup != nil {
		setup(env)
	}
	scr := New(env, Options{Path: path, Tab: TabIssues})
	root := app.New(env, scr, app.Options{})
	return &opened{d: uitest.New(t, root), root: root, scr: scr, tab: scr.tabs[TabIssues].(*issuesTab), env: env, urls: &urls, fake: client}
}

func lastIssueFilter(c *fake.Client) gitlab.IssueFilter {
	calls := c.CallsOf("Issues.List")
	return calls[len(calls)-1][1].(gitlab.IssueFilter)
}

func TestIssuesListWithRelatedMR(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.RequireView("3 Issues 5", "state:opened", "assignee:any", "label:any", "milestone:any", "IID", "Title", "Labels", "Upd")
	requireRow(t, o.d, "› #1318 Surge coefficient ignores zone overrides bug pricing 20m")
	requireRow(t, o.d, "#1312 Duplicate slot assignment under load → !479 bug slots 1h")
	requireRow(t, o.d, "#1260 Courier ETA drifts after midnight UTC+7 bug 2w")
	o.d.RequireView("1 / 5")
	calls := o.fake.CallsOf("Issues.RelatedMRs")
	if len(calls) != 1 || calls[0][1] != int64(1312) {
		t.Fatalf("related MRs are asked for #1312 only: %v", calls)
	}
	if rel := o.tab.Related(1312); len(rel) != 1 || rel[0].IID != 479 {
		t.Fatalf("related = %+v", rel)
	}
}

func TestIssuesSeveralRelatedMRs(t *testing.T) {
	c := issuesFake()
	c.SetRelatedMRs(path, 1312, gitlab.RelatedMR{IID: 479, State: gitlab.MRStateOpened}, gitlab.RelatedMR{IID: 482, State: gitlab.MRStateMerged})
	o := openIssues(t, c, nil)
	requireRow(t, o.d, "#1312 Duplicate slot assignment under load → !479 +1")
}

func TestIssuesLazyUntilTheTabIsOpened(t *testing.T) {
	c := issuesFake()
	env := newEnv(c)
	scr := New(env, Options{Path: path})
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	if c.CallCount("Issues.List") != 0 {
		t.Fatal("the Issues tab must not load before it is opened")
	}
	d.Key("3")
	if c.CallCount("Issues.List") != 1 {
		t.Fatalf("calls = %d", c.CallCount("Issues.List"))
	}
	d.RequireView("#1318")
}

func TestIssuesRelatedAtMostFourAtOnce(t *testing.T) {
	c := issuesFake()
	for i := range 10 {
		iid := int64(2000 + i)
		c.AddIssue(path, gitlab.Issue{IID: iid, Title: fmt.Sprintf("Issue %d", iid), MergeRequestsCount: 1, UpdatedAt: now.Add(-time.Duration(i) * time.Minute)})
	}
	c.SetDelay("Issues.RelatedMRs", 2*time.Second) // slow answers: the driver drops them
	o := openIssues(t, c, nil)
	_ = o
	if n := c.CallCount("Issues.RelatedMRs"); n != 4 {
		t.Fatalf("requests in flight = %d, want 4", n)
	}
}

func TestIssuesRelatedNotRepeatedByPolling(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	n := o.fake.CallCount("Issues.RelatedMRs")
	o.d.Send(common.PollTickMsg{Token: o.tab.poll.Current()})
	if o.fake.CallCount("Issues.List") != 2 {
		t.Fatalf("the tick must reload the list: %d", o.fake.CallCount("Issues.List"))
	}
	if o.fake.CallCount("Issues.RelatedMRs") != n {
		t.Fatal("a tick must not ask for the related MRs again")
	}
	o.d.Key("r")
	if o.fake.CallCount("Issues.RelatedMRs") != n+1 {
		t.Fatalf("r asks again past the cache: %d", o.fake.CallCount("Issues.RelatedMRs"))
	}
}

func TestIssuesFilterMe(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("m")
	if f := lastIssueFilter(o.fake); f.Assignee != "@me" {
		t.Fatalf("filter = %+v", f)
	}
	o.d.RequireView("assignee:@me", "#1318", "1 / 1")
	o.d.RequireNoView("#1312")
	o.d.Key("m")
	o.d.RequireView("assignee:any", "#1312")
}

func TestIssuesStateAndReset(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("s")
	o.d.RequireView("state:closed", "#1204")
	o.d.RequireNoView("#1318")
	o.d.Key("s")
	o.d.RequireView("state:all", "#1204", "#1318")
	o.d.Key("s")
	o.d.RequireView("state:opened")
	o.d.Key("m", "s", "backspace")
	o.d.RequireView("state:opened", "assignee:any", "#1312", "#1318")
	o.d.RequireNoView("#1204")
}

func TestIssuesMilestoneFromSuggestions(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("/")
	typeAll(o.d, "milestone:2026")
	if o.fake.CallCount("Projects.Milestones") != 0 {
		t.Fatal("typing alone must not query the server")
	}
	o.d.Send(o.tab.fl.DebounceMsg())
	o.d.RequireView("2026-Q4", "2026-Q3")
	o.d.RequireNoView("backlog")
	o.d.Key("tab", "enter")
	if f := lastIssueFilter(o.fake); f.Milestone != "2026-Q4" {
		t.Fatalf("filter = %+v", f)
	}
	o.d.RequireView("milestone:2026-Q4", "#1318")
	o.d.RequireNoView("#1312")
	o.d.Key("o")
	if got := *o.urls; len(got) != 1 || got[0] != "https://gitlab.example.com/"+path+"/-/issues?milestone_title=2026-Q4" {
		t.Fatalf("urls = %v", got)
	}
}

func TestIssuesFilterAssigneeAndLabelsFromInput(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("/")
	typeAll(o.d, "assignee:anna.k label:bug,slots surge")
	o.d.Key("enter")
	f := lastIssueFilter(o.fake)
	if f.Assignee != "anna.k" || len(f.Labels) != 2 || f.Labels[1] != "slots" || f.Search != "surge" {
		t.Fatalf("filter = %+v", f)
	}
	o.d.Key("o")
	want := "https://gitlab.example.com/" + path + "/-/issues?assignee_username=anna.k&label_name%5B%5D=bug&label_name%5B%5D=slots&search=surge"
	if got := *o.urls; len(got) != 1 || got[0] != want {
		t.Fatalf("urls = %v", got)
	}
}

func TestIssuesEmptyStates(t *testing.T) {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, WebURL: "https://gitlab.example.com/" + path})
	o := openIssues(t, c, nil)
	o.d.RequireView("No open issues")
	if o.tab.Count() != 0 { // X-Total: 0 is a count, not "unknown"
		t.Fatalf("count = %d", o.tab.Count())
	}
	o.d.RequireNoView("backspace — reset")
	o.d.Key("m")
	o.d.RequireView("No open issues assigned to you · m — drop the “me” filter")
	o.d.Key("s")
	o.d.RequireView("No issues for the filters · backspace — reset")
	o.d.Key("backspace")
	o.d.RequireView("No open issues")
}

func TestIssuesPreviewWide(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	o.d.RequireView("Surge coefficient ignores zone overrides", "● open · #1318 · anna.k · assignee: egor · milestone: 2026-Q4",
		"Zones with a manual override use the global coefficient.", "COMMENTS 2",
		"d.orlov · 1h", "Override is read before zone cache invalidation.",
		"egor · 20m", "c — write a comment…", "│")
	o.d.Key("j")
	o.d.RequireView("● open · #1312 · d.orlov", "→ !479 Fix slot race · opened", "Two couriers get one slot.", "COMMENTS 0")
	o.d.RequireNoView("COMMENTS 2")
}

func TestIssuesPreviewBelowAndHidden(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.RequireView("● open · #1318", "COMMENTS 2", "c — write a comment…")
	if o.tab.previewRight() || !o.tab.previewBelow() {
		t.Fatal("120 columns: the preview is below the list")
	}
	o.d.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	o.d.RequireNoView("COMMENTS", "c — write a comment…")
	o.d.RequireView("#1318")
}

func TestIssuesPreviewClosedAndNoDescription(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("s", "s") // all
	o.d.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	for range 5 {
		o.d.Key("j")
	}
	o.d.RequireView("○ closed · #1204 · anna.k", "no description")
}

func TestIssuesNotesAreLazyAndForTheSelectedIssueOnly(t *testing.T) {
	c := issuesFake()
	c.SetDelay("Issues.Notes", time.Second) // the driver drops the slow answers
	o := openIssues(t, c, nil)
	if calls := c.CallsOf("Issues.Notes"); len(calls) != 1 || calls[0][1] != int64(1318) || calls[0][2] != 50 {
		t.Fatalf("notes calls = %v", calls)
	}
	o.d.RequireView("◐ loading…")
	old := o.tab.notesGen.Current()
	o.d.Key("j") // #1312 before the answer for #1318
	if calls := c.CallsOf("Issues.Notes"); len(calls) != 2 || calls[1][1] != int64(1312) {
		t.Fatalf("notes calls = %v", calls)
	}
	o.d.Send(issueNotesMsg{tok: old, iid: 1318, notes: []gitlab.Note{{ID: 9, Author: egor, Body: "stale"}}})
	if e := o.tab.notes[1318]; e.state != 0 || len(e.notes) != 0 {
		t.Fatalf("the answer for #1318 must be dropped: %+v", e)
	}
	o.d.RequireNoView("stale")
	// The answer for the selected issue lands.
	o.d.Send(issueNotesMsg{tok: o.tab.notesGen.Current(), iid: 1312, notes: []gitlab.Note{{ID: 10, Author: dorlov, Body: "fresh", CreatedAt: now.Add(-time.Hour)}}})
	o.d.RequireView("fresh")
	// Coming back to a loaded issue asks nothing.
	o.d.Key("k")
	n := c.CallCount("Issues.Notes")
	o.d.Key("j")
	o.d.RequireView("fresh")
	if c.CallCount("Issues.Notes") != n {
		t.Fatalf("notes calls = %d, want %d", c.CallCount("Issues.Notes"), n)
	}
}

func TestIssuesNotesErrorIsQuiet(t *testing.T) {
	c := issuesFake()
	c.FailNext("Issues.Notes", &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	o := openIssues(t, c, nil)
	o.d.RequireView("comments unavailable")
	o.d.RequireNoView("server error") // no toast
}

func TestIssuesCloseAndReopen(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("x")
	o.d.RequireView("Close issue #1318?", "Surge coefficient ignores zone overrides")
	o.d.Key("y")
	calls := o.fake.CallsOf("Issues.SetState")
	if len(calls) != 1 || calls[0][1] != int64(1318) || calls[0][2] != true {
		t.Fatalf("SetState calls = %v", calls)
	}
	o.d.RequireView("Issue #1318 closed", "#1312")
	if o.tab.Status() != "1 / 4" {
		t.Fatalf("status = %q", o.tab.Status())
	}
	for _, is := range o.tab.Items() {
		if is.IID == 1318 {
			t.Fatal("the closed issue must leave the list")
		}
	}
	if o.tab.selected().IID != 1312 {
		t.Fatalf("the cursor moves to the neighbour: %d", o.tab.selected().IID)
	}
	// Reopen from the closed list: no confirm.
	o.d.Key("s")
	o.d.RequireView("#1318")
	o.d.Key("x")
	o.d.RequireNoView("Close issue", "Reopen")
	o.d.RequireView("Issue #1318 reopened")
}

func manyIssues(n int) *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, DefaultBranch: "main", WebURL: "https://gitlab.example.com/" + path})
	for i := n; i >= 1; i-- {
		c.AddIssue(path, gitlab.Issue{IID: int64(i), Title: fmt.Sprintf("Issue number %d", i), Author: anna,
			UpdatedAt: now.Add(-time.Duration(n-i) * time.Minute)})
	}
	return c
}

// listedCursors are the page cursors of the Issues.List calls, "" for a
// first page.
func listedCursors(c *fake.Client) []string {
	var out []string
	for _, call := range c.CallsOf("Issues.List") {
		out = append(out, call[2].(string))
	}
	return out
}

// An issue closed from the second page leaves the list at once, and the
// refresh neither brings it back with the old tail nor asks the loaded
// pages again.
func TestIssuesCloseFromSecondPageLeavesNoStaleTail(t *testing.T) {
	c := manyIssues(45)
	o := openIssues(t, c, nil)
	for range 16 {
		o.d.Key("j")
	}
	if len(o.tab.Items()) != 40 {
		t.Fatalf("two pages must be loaded: %d", len(o.tab.Items()))
	}
	for range 6 {
		o.d.Key("j")
	}
	iid := o.tab.selected().IID
	if o.tab.tbl.Cursor() < 20 {
		t.Fatalf("the cursor must be on the second page: %d", o.tab.tbl.Cursor())
	}
	o.d.Key("x", "y")
	for _, is := range o.tab.Items() {
		if is.IID == iid {
			t.Fatalf("#%d is back in the list", iid)
		}
	}
	if len(o.tab.Items()) != 39 || o.tab.Count() != 44 {
		t.Fatalf("items = %d count = %d", len(o.tab.Items()), o.tab.Count())
	}
	if o.tab.selected() == nil || o.tab.selected().IID == iid {
		t.Fatalf("the cursor must go to a neighbour: %+v", o.tab.selected())
	}
	for range 14 { // within 5 rows of the end
		o.d.Key("j")
	}
	seen := map[string]int{}
	for _, cur := range listedCursors(c) {
		seen[cur]++
	}
	for cur, n := range seen {
		if cur != "" && n > 1 {
			t.Fatalf("page %q was asked %d times: %v", cur, n, listedCursors(c))
		}
	}
	// Offset paging: the third page starts where the server's second one
	// ended, so the row that shifted into the gap is the price of not
	// asking the loaded pages again.
	if len(o.tab.Items()) != 43 {
		t.Fatalf("the third page must be appended once: %d", len(o.tab.Items()))
	}
}

// With state:all the closed issue stays in place with its new status.
func TestIssuesCloseKeepsTheRowUnderStateAll(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("s", "s") // closed, all
	o.d.RequireView("state:all")
	o.d.Key("x", "y")
	o.d.RequireView("Issue #1318 closed")
	var got *gitlab.Issue
	for i := range o.tab.Items() {
		if o.tab.Items()[i].IID == 1318 {
			got = &o.tab.Items()[i]
		}
	}
	if got == nil || got.State != "closed" {
		t.Fatalf("#1318 must stay, closed: %+v", got)
	}
}

func TestIssuesCommentFromTheList(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("c")
	o.d.RequireView("Comment · #1318 Surge coefficient ignores zone overrides")
	typeAll(o.d, "Беру")
	o.d.Key("tab", "enter")
	calls := o.fake.CallsOf("Issues.Comment")
	if len(calls) != 1 || calls[0][2] != "Беру" {
		t.Fatalf("Comment calls = %v", calls)
	}
	o.d.RequireView("Comment added", "COMMENTS 3", "Беру")
}

func TestIssuesEnterOpensTheIssueScreen(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("j", "enter")
	if o.root.Top() == app.Screen(o.scr) {
		t.Fatal("enter must push the issue screen")
	}
}

func TestIssuesArchivedProjectHidesCommentAndClose(t *testing.T) {
	c := issuesFake()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}, WebURL: "https://gitlab.example.com/" + path, Archived: true})
	o := openIssues(t, c, nil)
	o.d.Key("c")
	o.d.RequireNoView("Comment · #")
	o.d.Key("x")
	o.d.RequireNoView("Close issue")
	if c.CallCount("Issues.SetState") != 0 {
		t.Fatal("x must do nothing")
	}
	o.d.RequireNoView("c — write a comment…")
	hint := o.tab.KeyMap()
	for _, b := range hint.Short {
		if h := b.Help(); (h.Key == "c" || h.Key == "x") && b.Visible() {
			t.Fatalf("%s must be hidden", h.Key)
		}
	}
}

func TestIssuesURLsAndCopy(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("O")
	o.d.Key("o")
	want := []string{"https://gitlab.example.com/" + path + "/-/issues/1318", "https://gitlab.example.com/" + path + "/-/issues"}
	if got := *o.urls; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("urls = %v", got)
	}
	o.d.Key("y")
	o.d.RequireView("copied: …/issues/1318")
}

func TestIssuesURLWithMeFilter(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("m", "o")
	if got := *o.urls; len(got) != 1 || got[0] != "https://gitlab.example.com/"+path+"/-/issues?assignee_username=egor" {
		t.Fatalf("urls = %v", got)
	}
}

func TestIssuesCommentSentRefreshesTheList(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	n := o.fake.CallCount("Issues.List")
	o.d.Send(comment.SentMsg{Target: comment.Target{Project: o.tab.sh.ref, Kind: comment.KindMR, IID: 1318}})
	if o.fake.CallCount("Issues.List") != n {
		t.Fatal("a comment on an MR is not for the Issues tab")
	}
	o.d.Send(comment.SentMsg{Target: comment.Target{Project: o.tab.sh.ref, Kind: comment.KindIssue, IID: 1318}})
	if o.fake.CallCount("Issues.List") != n+1 {
		t.Fatalf("list calls = %d", o.fake.CallCount("Issues.List"))
	}
}

func TestIssuesRussian(t *testing.T) {
	env := newEnv(issuesFake())
	env.Msg, env.Lang = i18n.Get(i18n.RU), i18n.RU
	env.Cache = cache.New()
	scr := New(env, Options{Path: path, Tab: TabIssues})
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	d.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	d.RequireView("● открыт · #1318 · anna.k · исполнитель: egor", "КОММЕНТАРИИ 2", "c — написать комментарий…", "закрыть/открыть")
	d.Key("m", "m", "s", "s")
	d.RequireView("state:all")
}

// Closing from the issue screen: it asked, so it toasts once; the covered
// tab follows on return.
func TestIssuesCloseFromTheIssueScreenToastsOnce(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Key("enter")
	lists := o.fake.CallCount("Issues.List")
	o.d.Key("x", "y")
	n := 0
	for _, m := range o.d.Msgs {
		if tm, ok := m.(app.ToastMsg); ok && tm.Text == "Issue #1318 closed" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("toasts = %d", n)
	}
	if got := o.fake.CallCount("Issues.List"); got != lists {
		t.Fatalf("the covered tab must wait: %d -> %d", lists, got)
	}
	o.d.Key("esc")
	if got := o.fake.CallCount("Issues.List"); got != lists+1 {
		t.Fatalf("the tab reloads on return: %d -> %d", lists, got)
	}
	for _, is := range o.tab.Items() {
		if is.IID == 1318 {
			t.Fatal("the closed issue must leave the list")
		}
	}
}

// merge_requests_count > 0 is the only reason to ask the related MRs of the
// list; an issue without it gets no marker and no request.
func TestIssuesWithoutClosingMRHaveNoMarkerAndNoRequest(t *testing.T) {
	c := issuesFake()
	c.SetRelatedMRs(path, 1318, gitlab.RelatedMR{IID: 479, State: gitlab.MRStateOpened})
	o := openIssues(t, c, nil)
	requireRow(t, o.d, "#1318 Surge coefficient ignores zone overrides bug pricing 20m")
	o.d.RequireNoView("→ !479 bug pricing")
	for _, call := range c.CallsOf("Issues.RelatedMRs") {
		if call[1] == int64(1318) {
			t.Fatal("related MRs of #1318 must not be asked from the list")
		}
	}
}

// View repeats the same preview on every frame: glamour runs once.
func TestIssuesPreviewDoesNotRenderMarkdownOnEveryView(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	o.d.View()
	renders := o.tab.md.Renders
	if renders == 0 {
		t.Fatal("the preview renders markdown")
	}
	for range 3 {
		o.d.View()
	}
	if o.tab.md.Renders != renders {
		t.Fatalf("renders %d -> %d", renders, o.tab.md.Renders)
	}
	o.d.Key("j")
	o.d.View()
	if o.tab.md.Renders == renders {
		t.Fatal("another issue renders its own markdown")
	}
}
