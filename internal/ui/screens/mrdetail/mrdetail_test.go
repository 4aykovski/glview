package mrdetail

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const path = "platform/route-service"

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

func baseMR() gitlab.MergeRequest {
	return gitlab.MergeRequest{
		IID: 479, Title: "Fix race in courier slot allocator", State: gitlab.MRStateOpened,
		Author: gitlab.User{Username: "anna.k"}, SourceBranch: "fix/slot-race", TargetBranch: "main", SHA: "a91f2c0d",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now, MergeStatus: "not_approved", Labels: []string{"bug", "backend"},
		WebURL:      "https://gitlab.example.com/" + path + "/-/merge_requests/479",
		Description: "## Why\n\n- slot map raced\n- see `slotMap`\n\n```go\nfunc main() {}\n```\n",
		Approvals:   gitlab.Approvals{Required: 2, Left: 1, ApprovedBy: []gitlab.User{{Username: "bob"}}},
		Discussions: gitlab.DiscussionStats{Total: 2, Unresolved: 1},
		Diff:        &gitlab.DiffStats{Additions: 48, Deletions: 12, Files: 3},
		HeadPipeline: &gitlab.PipelineSummary{ID: 91840, Status: gitlab.StatusFailed, Stages: []gitlab.StageSummary{
			{Name: "build", Status: gitlab.StatusSuccess, Done: 2, Total: 2, Duration: 62 * time.Second}, {Name: "test", Status: gitlab.StatusFailed, Done: 3, Total: 3, Duration: 280 * time.Second}}},
	}
}

func setup(t *testing.T, mr gitlab.MergeRequest) (*uitest.Driver, *app.Model, *Screen, *fake.Client) {
	t.Helper()
	return setupIn(t, mr, false)
}

// setupIn opens the MR in a project that is archived or not.
func setupIn(t *testing.T, mr gitlab.MergeRequest, archived bool) (*uitest.Driver, *app.Model, *Screen, *fake.Client) {
	t.Helper()
	client := fake.New()
	client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	client.AddMR(path, mr)
	client.SetDiscussions(path, mr.IID, []gitlab.Discussion{
		{ID: "d1", Resolvable: true, Resolved: false, Position: &gitlab.NotePosition{Path: "internal/slot.go", Line: 42}, Notes: []gitlab.Note{
			{Author: gitlab.User{Username: "anna.k"}, Body: "Why not a mutex?", CreatedAt: now.Add(-30 * time.Minute)},
			{Author: gitlab.User{Username: "egor"}, Body: "Because latency", CreatedAt: now.Add(-20 * time.Minute)},
		}},
		{ID: "d2", Notes: []gitlab.Note{{System: true, Author: gitlab.User{Username: "anna.k"}, Body: "added 1 commit", CreatedAt: now}}},
		{ID: "d3", Resolvable: true, Resolved: true, Notes: []gitlab.Note{{Author: gitlab.User{Username: "bob"}, Body: "LGTM overall", CreatedAt: now.Add(-10 * time.Minute)}}},
	})
	client.SetMRPipelines(path, mr.IID, []gitlab.PipelineSummary{
		{ID: 91840, Status: gitlab.StatusFailed, Ref: "refs/merge-requests/479/head", Source: "merge_request_event", SHA: "a91f2c0d4e", CreatedAt: now.Add(-time.Hour),
			User: &gitlab.User{Username: "anna.k"}, CommitTitle: "surge: add pricing", Duration: 220 * time.Second},
		{ID: 91700, Status: gitlab.StatusSuccess, Ref: "refs/merge-requests/479/head", Source: "merge_request_event", SHA: "0b7d3e9aaa", CreatedAt: now.Add(-3 * time.Hour)},
	})
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	scr := New(env, gitlab.ProjectRef{ID: 42, FullPath: path}, nil, mr, archived)
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr, client
}

func TestOverview(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.RequireView(
		path+" › MRs › !479", "Fix race in courier slot allocator",
		"anna.k · fix/slot-race → main · opened 1h ago", "+48", "−12", "3 files",
		"not_approved", "needs approvals", "1/2 approved", "failed", "1 thread unresolved",
		"Overview", "Discussions", "Pipelines",
		// markdown rendered: heading text and bullets, no raw syntax
		"Why", "• slot map raced", "func main() {}",
		// side column
		"APPROVALS", "✓ bob", "1 more needed", "LABELS", "bug, backend", "PIPELINE", "build", "test", "LAST THREAD", "bob", "LGTM overall",
	)
	d.RequireNoView("## Why", "```", "`slotMap`")
}

func TestStatusShownAsIsWithRussianCaption(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Key("ctrl+l")
	d.RequireView("not_approved", "нужны апрувы", "Обзор", "Обсуждения", "АПРУВЫ")
}

func TestCEApprovalsWithoutDenominator(t *testing.T) {
	mr := baseMR()
	mr.Approvals = gitlab.Approvals{ApprovedBy: []gitlab.User{{Username: "a"}, {Username: "b"}}}
	d, _, _, _ := setup(t, mr)
	d.RequireView("2 approved")
	d.RequireNoView("2/0")
}

func TestNarrowWrapsSideColumnBelow(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Send(tea.WindowSizeMsg{Width: 90, Height: 60})
	v := d.View()
	if strings.Index(v, "APPROVALS") < strings.Index(v, "func main() {}") {
		t.Fatalf("under 100 columns the side column goes below the description:\n%s", v)
	}
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	for _, l := range strings.Split(d.View(), "\n") {
		if strings.Contains(l, "APPROVALS") && !strings.Contains(l, "Why") {
			t.Fatalf("wide layout must put the side column beside the text: %q", l)
		}
	}
}

func TestDiscussions(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Key("tab")
	d.RequireView("anna.k", "● unresolved", "internal/slot.go:42", "Why not a mutex?", "Because latency", "✓ resolved", "LGTM overall")
	d.RequireNoView("added 1 commit")
}

// richDiscussions is a thread set modeled on a real MR: short code-line
// threads, a resolved one and a long security-bot report with HTML tables.
func richDiscussions(t *testing.T) []gitlab.Discussion {
	t.Helper()
	report, err := os.ReadFile("testdata/dependency-report.html")
	if err != nil {
		t.Fatal(err)
	}
	return []gitlab.Discussion{
		{ID: "r1", Resolvable: true, Position: &gitlab.NotePosition{Path: "internal/ids/ref.go", Line: 118}, Notes: []gitlab.Note{
			{ID: 7781, Author: gitlab.User{Username: "a.petrov"}, Body: "лучше `slot.IsZero()`", CreatedAt: now.Add(-3 * time.Hour)},
			{ID: 7782, Author: gitlab.User{Username: "d.orlov"}, Body: "?", CreatedAt: now.Add(-2 * time.Hour)},
			{ID: 7783, Author: gitlab.User{Username: "a.petrov"}, Body: "тогда не придётся проверять nil", CreatedAt: now.Add(-time.Hour)},
		}},
		{ID: "r2", Resolvable: true, Resolved: true, Position: &gitlab.NotePosition{Path: "cmd/main.go", Line: 12}, Notes: []gitlab.Note{
			{ID: 7790, Author: gitlab.User{Username: "bob"}, Body: "LGTM overall", CreatedAt: now.Add(-10 * time.Minute)},
		}},
		{ID: "r3", Resolvable: true, Notes: []gitlab.Note{
			{ID: 7800, Author: gitlab.User{Username: "deps-bot"}, Body: string(report), CreatedAt: now.Add(-5 * time.Minute)},
		}},
	}
}

func TestThreadsAreCards(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Key("tab")
	v := d.View()
	if n := strings.Count(v, "┌") + strings.Count(v, "┏"); n != 2 {
		t.Fatalf("two threads must give two frames, got %d:\n%s", n, v)
	}
	// note, rule, indented reply inside the first card
	lines := strings.Split(v, "\n")
	var iNote, iRule, iReply int
	for i, l := range lines {
		switch {
		case strings.Contains(l, "Why not a mutex?"):
			iNote = i
		case strings.Contains(l, "Because latency"):
			iReply = i
		}
	}
	for i := iNote; i < iReply; i++ {
		if strings.HasPrefix(lines[i], "│ ───") || strings.HasPrefix(lines[i], "┃ ───") {
			iRule = i
		}
	}
	if iRule == 0 || iReply <= iRule {
		t.Fatalf("expected note, rule, reply in order:\n%s", v)
	}
	if lead := func(l string) int { return len(l) - len(strings.TrimLeft(strings.TrimLeft(l, "│┃"), " ")) }; lead(lines[iReply]) <= lead(lines[iNote]) {
		t.Fatalf("reply must be indented:\n%s", v)
	}
}

// corners returns the escape prefix before each frame's top-left corner.
func corners(view string) []string {
	var out []string
	for _, l := range strings.Split(view, "\n") {
		if i := strings.IndexAny(l, "┌┏"); i >= 0 {
			out = append(out, l[:i])
		}
	}
	return out
}

func richSetup(t *testing.T) (*uitest.Driver, *app.Model, *Screen, *fake.Client) {
	t.Helper()
	d, root, scr, client := setup(t, baseMR())
	client.SetDiscussions(path, 479, richDiscussions(t))
	d.Key("r", "tab")
	return d, root, scr, client
}

// lightTheme gives Warning and Accent different colors (in the dark theme
// both are yellow) so tests can tell the frames apart.
func lightTheme(scr *Screen) { scr.env.Styles = common.NewStyles(common.TerminalTheme(false)) }

func TestFrameColorFollowsStatus(t *testing.T) {
	_, _, scr, _ := richSetup(t)
	lightTheme(scr)
	c := corners(scr.View()) // r1 current, r2 resolved, r3 unresolved
	if len(c) != 3 || c[1] == c[2] || c[0] == c[1] {
		t.Fatalf("frame colors must differ by status and current: %q", c)
	}
}

func TestHTMLTableInNoteIsRenderedAsTable(t *testing.T) {
	d, _, _, _ := richSetup(t)
	d.Key("J", "J", "space")
	d.RequireView("Компонент", "│", "Версия", "0.2.0")
	d.RequireNoView("<table", "<td", "КомпонентВерсия")
}

func TestPipelinesTab(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Key("tab", "tab")
	d.RequireView("#91840", "#91700", "!479", "failed", "success", "merge_request_event", "a91f2c0 surge: add", "anna.k", "3m40s")
	d.Key("enter")
	d.RequireView(path + " › Pipelines › #91840")
	d.Key("esc")
	d.RequireView("#91700")
}

func TestPollingIntervalsAndTopOnly(t *testing.T) {
	d, root, scr, client := setup(t, baseMR())
	if got := scr.pollDelay(); got != 30*time.Second {
		t.Fatalf("finished pipeline: %v", got)
	}
	scr.mr.HeadPipeline.Status = gitlab.StatusRunning
	if got := scr.pollDelay(); got != 3*time.Second {
		t.Fatalf("running pipeline: %v", got)
	}
	scr.mr.HeadPipeline.Status = gitlab.StatusSuccess
	scr.mr.RebaseInProgress = true
	if got := scr.pollDelay(); got != 3*time.Second {
		t.Fatalf("rebase: %v", got)
	}
	scr.mr.RebaseInProgress = false

	n := client.CallCount("MergeRequests.Get")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if client.CallCount("MergeRequests.Get") != n+1 {
		t.Fatal("tick on the top screen must reload the MR")
	}

	// Covered by another screen: no requests, stale ticks are ignored.
	stale := scr.poll.Current()
	d.Send(app.PushMsg{Screen: &cover{}})
	n = client.CallCount("MergeRequests.Get")
	d.Send(common.PollTickMsg{Token: stale})
	if client.CallCount("MergeRequests.Get") != n {
		t.Fatal("a covered screen must not poll")
	}
	d.Key("esc")
	if root.Depth() != 1 {
		t.Fatal("esc must pop")
	}
}

func TestRefreshKey(t *testing.T) {
	d, _, _, client := setup(t, baseMR())
	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.Title = "Renamed" })
	d.Key("r")
	d.RequireView("Renamed")
}

type cover struct{}

func (cover) Init() tea.Cmd                          { return nil }
func (c cover) Update(tea.Msg) (app.Screen, tea.Cmd) { return c, nil }
func (cover) View() string                           { return "cover" }
func (cover) Title() string                          { return "cover" }
func (cover) KeyMap() common.KeyMap                  { return common.KeyMap{} }
func (cover) SetSize(int, int)                       {}

func TestApproveUnapproveFromDetail(t *testing.T) {
	d, _, scr, client := setup(t, baseMR())
	client.ResetCalls()
	d.Key("a")
	if c := client.Calls(); c[0].Method != "MergeRequests.Approve" || c[0].Args[2] != "a91f2c0d" {
		t.Fatalf("calls = %+v", c)
	}
	d.RequireView("approved !479")
	if !scr.MR().Approvals.ApprovedByMe {
		t.Fatal("MR must be reloaded after the action")
	}
	d.Key("a")
	if got := client.Calls(); !hasCall(got, "MergeRequests.Unapprove") {
		t.Fatalf("second a must unapprove: %+v", got)
	}
	d.RequireView("approval removed !479")
}

func hasCall(calls []fake.Call, method string) bool {
	for _, c := range calls {
		if c.Method == method {
			return true
		}
	}
	return false
}

func TestMergeDialogFromDetail(t *testing.T) {
	mr := baseMR()
	mr.MergeStatus = "mergeable"
	mr.Squash, mr.ForceRemoveSourceBranch = true, true
	mr.HeadPipeline.Status = gitlab.StatusSuccess
	d, _, _, client := setup(t, mr)
	d.Key("M")
	d.RequireView("Merge !479", "Merge now", "Merge when the pipeline succeeds", "[x] Squash commits", "[x] Remove source branch")
	d.Key("enter")
	var call fake.Call
	for _, c := range client.Calls() {
		if c.Method == "MergeRequests.Merge" {
			call = c
		}
	}
	o := call.Args[2].(gitlab.MergeOptions)
	if o.SHA != "a91f2c0d" || !o.Squash || !o.RemoveSourceBranch || o.AutoMerge {
		t.Fatalf("options = %+v", o)
	}
	d.RequireView("merged !479")
}

func TestMergeDialogDefaultsAutoMergeWhileRunning(t *testing.T) {
	mr := baseMR()
	mr.MergeStatus = "ci_still_running"
	mr.HeadPipeline.Status = gitlab.StatusRunning
	d, _, _, client := setup(t, mr)
	d.Key("M", "enter")
	for _, c := range client.Calls() {
		if c.Method == "MergeRequests.Merge" {
			if o := c.Args[2].(gitlab.MergeOptions); !o.AutoMerge {
				t.Fatalf("auto-merge must be preselected: %+v", o)
			}
			d.RequireView("will merge when the pipeline succeeds")
			return
		}
	}
	t.Fatal("no merge call")
}

func TestMergeDialogToggles(t *testing.T) {
	mr := baseMR()
	mr.MergeStatus = "mergeable"
	d, _, _, client := setup(t, mr)
	d.Key("M", "right", "j", "space", "j", "space", "enter")
	for _, c := range client.Calls() {
		if c.Method == "MergeRequests.Merge" {
			if o := c.Args[2].(gitlab.MergeOptions); !o.AutoMerge || !o.Squash || !o.RemoveSourceBranch {
				t.Fatalf("options = %+v", o)
			}
			return
		}
	}
	t.Fatal("no merge call")
}

func TestMergeErrors(t *testing.T) {
	mr := baseMR()
	mr.MergeStatus = "mergeable"
	d, _, _, client := setup(t, mr)
	client.FailNext("MergeRequests.Merge", &gitlab.Error{Kind: gitlab.ErrConflict, Status: 409})
	d.Key("M", "enter")
	d.RequireView("Branch was pushed to, refresh (r)")
	client.FailNext("MergeRequests.Merge", &gitlab.Error{Kind: gitlab.ErrNotPossible, Status: 405})
	d.Key("M", "enter")
	d.RequireView("Can't merge: mergeable")
}

func TestMergeBlockedByConflict(t *testing.T) {
	mr := baseMR()
	mr.MergeStatus = "conflict"
	d, _, _, client := setup(t, mr)
	d.Key("M")
	d.RequireView("Can't merge: conflict", "has conflicts")
	d.Key("enter")
	if hasCall(client.Calls(), "MergeRequests.Merge") {
		t.Fatal("a blocked dialog must not merge")
	}
	d.Key("esc")
	d.RequireNoView("Can't merge: conflict —")
}

func TestRebaseTracking(t *testing.T) {
	mr := baseMR()
	d, _, scr, client := setup(t, mr)
	// The rebase finishes at the 3rd Get after it was queued.
	var queued bool
	getsAfter := 0
	client.OnCall("MergeRequests.Rebase", func(int) { queued = true })
	client.OnCall("MergeRequests.Get", func(int) {
		if !queued {
			return
		}
		getsAfter++
		if getsAfter == 3 {
			client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.RebaseInProgress = false })
		}
	})
	d.Key("b")
	d.RequireView("Rebase !479?")
	d.Key("y")
	d.RequireView("◐ rebase…")
	if !scr.act.Tracking() {
		t.Fatal("tracker must run")
	}
	d.Send(scr.act.TickMsg())
	d.RequireView("◐ rebase…")
	d.Send(scr.act.TickMsg())
	d.RequireView("◐ rebase…")
	d.Send(scr.act.TickMsg())
	d.RequireView("rebase done")
	d.RequireNoView("◐ rebase…")
	if scr.act.Tracking() {
		t.Fatal("tracker must stop")
	}
}

func TestRebaseFailureShowsMergeError(t *testing.T) {
	d, _, scr, client := setup(t, baseMR())
	client.OnCall("MergeRequests.Get", func(n int) {
		client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) {
			m.RebaseInProgress = false
			m.MergeError = "Rebase failed: conflict in slot.go"
		})
	})
	d.Key("b", "y")
	d.Send(scr.act.TickMsg())
	d.RequireView("rebase failed: Rebase failed: conflict in slot.go")
}

func TestRebaseTimeout(t *testing.T) {
	d, root, scr, _ := setup(t, baseMR())
	d.Key("b", "y")
	root.Env().Now = func() time.Time { return now.Add(3 * time.Minute) }
	d.Send(scr.act.TickMsg())
	d.RequireView("rebase is still running")
}

func TestChipsInFramesAndStageDurations(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.RequireView("╭", "│ ✓ 1/2 approved │", "PIPELINE #91840", "✓ build", "1m02s", "✗ test", "4m40s", "APPROVALS")
	// On a short terminal the chips are plain one-liners.
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 24})
	d.RequireNoView("╭")
	d.RequireView("✓ 1/2 approved")
}

func TestNoAgoWithoutDate(t *testing.T) {
	mr := baseMR()
	mr.CreatedAt = time.Time{}
	d, _, _, _ := setup(t, mr)
	d.RequireNoView("opened", "—")
	d.RequireView("anna.k · fix/slot-race → main")
}

func TestMergedMRChip(t *testing.T) {
	mr := baseMR()
	mr.State, mr.MergeStatus = gitlab.MRStateMerged, "not_open"
	d, _, _, _ := setup(t, mr)
	d.RequireView("✓ merged")
	if strings.Contains(d.View(), "not_open") {
		t.Fatalf("not_open leaked into the view:\n%s", d.View())
	}
}

func mergedMR() gitlab.MergeRequest {
	mr := baseMR()
	mr.State = gitlab.MRStateMerged
	mr.MergeStatus = "not_open"
	mr.MergedAt = now.Add(-5 * time.Hour)
	mr.MergedBy = &gitlab.User{Username: "d.orlov"}
	return mr
}

func TestHeaderMerged(t *testing.T) {
	d, _, _, _ := setup(t, mergedMR())
	d.RequireView("anna.k · fix/slot-race → main · merged by d.orlov 5h ago")
	d.RequireNoView("opened")
	d.Key("ctrl+l")
	d.RequireView("смержил d.orlov 5ч назад")
	d.RequireNoView("открыт 1ч")
}

func TestHeaderMergedWithoutUser(t *testing.T) {
	mr := mergedMR()
	mr.MergedBy = nil
	d, _, _, _ := setup(t, mr)
	d.RequireView("· merged 5h ago")
	d.RequireNoView("merged by", "opened")
	d.Key("ctrl+l")
	d.RequireView("смержен 5ч назад")
}

func TestHeaderClosed(t *testing.T) {
	mr := baseMR()
	mr.State = gitlab.MRStateClosed
	mr.MergeStatus = "not_open"
	mr.ClosedAt = now.Add(-72 * time.Hour)
	d, _, _, _ := setup(t, mr)
	d.RequireView("closed 3d ago")
	d.RequireNoView("opened")
	d.Key("ctrl+l")
	d.RequireView("закрыт 3д назад")
}

func TestHeaderMergedWithoutTimeFallsBackToOpened(t *testing.T) {
	mr := mergedMR()
	mr.MergedAt = time.Time{}
	d, _, _, _ := setup(t, mr)
	d.RequireView("opened 1h ago")
}

// TestCurrentThreadIsThickInDarkTheme: in the dark theme Accent and Warning
// are both yellow, so the current thread is told apart by its frame shape.
func TestCurrentThreadIsThickInDarkTheme(t *testing.T) {
	d, _, _, _ := richSetup(t)
	thick := func() (n int, next string) {
		lines := strings.Split(ansi.Strip(d.View()), "\n")
		for i, l := range lines {
			if strings.Contains(l, "┏") {
				n++
				next = lines[i+1]
			}
		}
		return n, next
	}
	if n, _ := thick(); n != 1 {
		t.Fatalf("exactly one thick frame expected, got %d", n)
	}
	d.Key("J")
	if n, next := thick(); n != 1 || !strings.Contains(next, "LGTM overall") {
		t.Fatalf("thick frame must move to the second thread: %d %q", n, next)
	}
}

func TestJMovesCurrentThread(t *testing.T) {
	d, _, scr, _ := richSetup(t)
	lightTheme(scr)
	first := corners(scr.View())[0]
	d.Key("J")
	if curThread(scr) != "r2" {
		t.Fatalf("current = %q", curThread(scr))
	}
	c := corners(scr.View())
	if c[1] != first || c[0] == first {
		t.Fatalf("the second frame must be Accent now: %q", c)
	}
	d.RequireView("LGTM overall")
	d.Key("K", "K")
	if curThread(scr) != "r1" {
		t.Fatalf("K must stop at the first thread: %q", curThread(scr))
	}
}

func TestJKRepicksCurrentThread(t *testing.T) {
	d, _, scr, _ := richSetup(t)
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 14})
	d.Key(strings.Split(strings.Repeat("j,", 10), ",")[:10]...)
	if curThread(scr) != "r2" {
		t.Fatalf("after scrolling past the first card current = %q", curThread(scr))
	}
	d.Key("k", "k", "k")
	if curThread(scr) != "r1" {
		t.Fatalf("scrolling back: current = %q", curThread(scr))
	}
}

func TestLongThreadFoldedAndSpaceUnfolds(t *testing.T) {
	d, _, _, _ := richSetup(t)
	d.RequireView("▸ deps-bot", "Уязвимые зависимости (go)", "▸ bob 10m", "LGTM overall")
	d.RequireNoView("Компонент")
	// replies are counted on the folded line (the first thread is unfolded)
	d.Key("J", "J", "space")
	d.RequireView("Компонент")
	d.Key("space")
	d.RequireNoView("Компонент")
	d.RequireView("▸ deps-bot")
}

func TestFoldedLineCountsReplies(t *testing.T) {
	d, _, _, _ := richSetup(t)
	d.Key("space") // fold the first thread: two replies
	d.RequireView("▸ a.petrov 3h", "2 replies")
	d.Key("ctrl+l")
	d.RequireView("2 ответа")
}

func TestFoldSurvivesReload(t *testing.T) {
	d, _, _, _ := richSetup(t)
	d.Key("J", "J", "space")
	d.RequireView("Компонент")
	d.Key("r")
	d.RequireView("Компонент")
	d.Key("J") // stays on the last thread; user choice for others is untouched
	d.RequireView("Компонент")
}

func TestSpacePagesOutsideDiscussions(t *testing.T) {
	mr := baseMR()
	mr.Description = strings.Repeat("line\n\n", 60)
	d, _, scr, _ := setup(t, mr)
	d.Key("space")
	d.View()
	if scr.scroll == 0 {
		t.Fatal("space must page the Overview")
	}
	d.Key("tab")
	if scr.scroll != 0 {
		t.Fatal("tab resets the scroll")
	}
}

func TestPageKeysWorkOnDiscussions(t *testing.T) {
	d, _, scr, _ := richSetup(t)
	d.Key("J", "J", "space", "pgdown")
	d.View()
	if scr.scroll == 0 {
		t.Fatal("pgdown must scroll the Discussions tab")
	}
	d.Key("ctrl+u", "ctrl+u", "ctrl+u")
	d.View()
	if scr.scroll != 0 {
		t.Fatalf("ctrl+u back: %d", scr.scroll)
	}
}

func TestThreadKeysInFooterOnlyOnDiscussions(t *testing.T) {
	d, _, _, _ := richSetup(t)
	d.Send(tea.WindowSizeMsg{Width: 160, Height: 40})
	d.RequireView("J/K threads", "space fold")
	d.Key("shift+tab")
	d.RequireNoView("J/K threads", "space fold")
	d.Key("tab", "ctrl+l")
	d.RequireView("J/K треды", "space свернуть")
}

func TestOpenAndCopyThreadLink(t *testing.T) {
	d, root, _, _ := richSetup(t)
	var opened []string
	root.Env().Browser = func(url string) error { opened = append(opened, url); return nil }
	mrURL := "https://gitlab.example.com/" + path + "/-/merge_requests/479"
	d.Key("o")
	d.Key("J", "o")
	if len(opened) != 2 || opened[0] != mrURL+"#note_7781" || opened[1] != mrURL+"#note_7790" {
		t.Fatalf("opened = %q", opened)
	}
	d.Key("shift+tab", "o")
	if opened[2] != mrURL {
		t.Fatalf("outside Discussions o opens the MR: %q", opened[2])
	}
}

func uncheckedMR() gitlab.MergeRequest {
	mr := baseMR()
	mr.MergeStatus = "unchecked"
	return mr
}

func TestUncheckedIsRecheckedOnce(t *testing.T) {
	mr := uncheckedMR()
	d, _, scr, client := setup(t, mr)
	client.SetRecheckResult(path, 479, "not_approved")
	// setup already opened the screen and sent the recheck; the scripted
	// result appears on the next read.
	d.RequireView("checking")
	if n := client.CallCount("MergeRequests.RecheckMergeStatus"); n != 1 {
		t.Fatalf("recheck calls = %d, want 1", n)
	}
	if got := scr.pollDelay(); got != 3*time.Second {
		t.Fatalf("poll while checking = %v, want 3s", got)
	}

	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	d.RequireView("not_approved")
	d.RequireNoView("checking")

	// Further ticks and r never send a second recheck.
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	d.Key("r")
	d.RequireView("not_approved")
	if n := client.CallCount("MergeRequests.RecheckMergeStatus"); n != 1 {
		t.Fatalf("recheck calls = %d, want 1", n)
	}
}

func TestRecheckErrorIsSilentAndWindowExpires(t *testing.T) {
	client := fake.New()
	client.FailNext("MergeRequests.RecheckMergeStatus", &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403})
	env := app.NewEnv(i18n.EN)
	cur := now
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return cur }
	client.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	client.AddMR(path, uncheckedMR())
	scr := New(env, gitlab.ProjectRef{ID: 42, FullPath: path}, nil, uncheckedMR(), false)
	d := uitest.New(t, app.New(env, scr, app.Options{}))

	d.RequireView("unchecked")
	d.RequireNoView("forbidden", "Forbidden")
	if n := client.CallCount("MergeRequests.RecheckMergeStatus"); n != 1 {
		t.Fatalf("recheck calls = %d", n)
	}
	if got := scr.pollDelay(); got != 3*time.Second {
		t.Fatalf("inside the window: %v", got)
	}
	cur = now.Add(61 * time.Second)
	if got := scr.pollDelay(); got != 30*time.Second {
		t.Fatalf("after 60s: %v", got)
	}
	if !strings.Contains(scr.StatusHint(), "30s") {
		t.Fatalf("hint = %q", scr.StatusHint())
	}
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount("MergeRequests.RecheckMergeStatus"); n != 1 {
		t.Fatalf("no second recheck, got %d calls", n)
	}
}

func TestMergedAndOtherStatusesAreNotRechecked(t *testing.T) {
	merged := uncheckedMR()
	merged.State = gitlab.MRStateMerged
	for _, mr := range []gitlab.MergeRequest{merged, baseMR()} {
		_, _, scr, client := setup(t, mr)
		if n := client.CallCount("MergeRequests.RecheckMergeStatus"); n != 0 {
			t.Errorf("%s/%s: %d recheck calls", mr.State, mr.MergeStatus, n)
		}
		if got := scr.pollDelay(); got != 30*time.Second {
			t.Errorf("%s/%s: poll %v", mr.State, mr.MergeStatus, got)
		}
	}
}

// curThread is the ID of the current thread card.
func curThread(s *Screen) string {
	c, _ := s.threads.Current()
	return c.ID
}

func TestCommentFromMRDetail(t *testing.T) {
	d, _, _, client := setup(t, baseMR())
	d.RequireView("Discussions 2")
	n := client.CallCount("MergeRequests.Discussions")
	d.Key("c")
	d.RequireView("Comment · !479")
	for _, r := range "Looks fine" {
		d.Key(string(r))
	}
	d.Key("tab", "enter")
	calls := client.CallsOf("MergeRequests.Comment")
	if len(calls) != 1 || calls[0][1] != int64(479) || calls[0][2] != "Looks fine" {
		t.Fatalf("Comment calls = %v", calls)
	}
	d.RequireView("Comment added", "Discussions 3")
	if client.CallCount("MergeRequests.Discussions") != n+1 {
		t.Fatal("the discussions must be asked again")
	}
}

func TestCommentIsHiddenAndDisabledInArchivedProject(t *testing.T) {
	d, _, _, client := setupIn(t, baseMR(), true)
	d.RequireView("Discussions 2")
	d.RequireNoView("comment")
	d.Key("c")
	d.RequireNoView("Comment · !479")
	d.Key("?")
	d.RequireNoView("comment")
	d.Key("esc")
	if client.CallCount("MergeRequests.Comment") != 0 {
		t.Fatal("nothing is sent")
	}
	// A project that is not archived shows it.
	d, _, _, _ = setupIn(t, baseMR(), false)
	d.RequireView("comment")
}

func TestCommentFromOtherTab(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Key("tab", "tab") // Pipelines
	d.Key("c")
	d.RequireView("Comment · !479")
}
