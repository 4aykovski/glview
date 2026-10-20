package joblog

import (
	"fmt"
	"os"
	"strings"
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

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

const sample = "Running with gitlab-runner\n" +
	"section_start:1700000000:prepare\r\x1b[0KPreparing executor\n" +
	"Using docker image\n" +
	"section_end:1700000003:prepare\r\x1b[0K\n" +
	"section_start:1700000003:deps[collapsed=true]\r\x1b[0K\x1b[36mInstalling deps\x1b[0m\n" +
	"downloading\n" +
	"timeout while fetching module\n" +
	"section_end:1700000009:deps\r\x1b[0K\n" +
	"\x1b[31mFAIL: TestSurge\x1b[0m\n" +
	"10%\r50%\r100%\n"

func setup(t *testing.T, status gitlab.PipelineStatus, trace string) (*uitest.Driver, *app.Model, *Screen, *fake.Client) {
	t.Helper()
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	job := gitlab.Job{ID: 7, Name: "test:integration", Stage: "test", Status: status, Duration: 72 * time.Second, WebURL: "https://gitlab.example.com/job/7"}
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91840}, Jobs: []gitlab.Job{job}})
	c.SetTrace(7, trace)
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	scr := New(env, gitlab.ProjectRef{ID: 42, FullPath: path}, 91840, job)
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr, c
}

func TestRendersNumbersColorsSectionsAndProgress(t *testing.T) {
	d, _, scr, _ := setup(t, gitlab.StatusFailed, sample)
	d.RequireView(
		"#91840 › test:integration", "✗ failed", "1m12s", "○ paused",
		"1 Running with gitlab-runner",
		"▾ Preparing executor", "3s",
		"Using docker image",
		"▸ Installing deps", "6s",
		"FAIL: TestSurge",
		"100%",
	)
	d.RequireNoView("downloading", "10%", "50%", "section_", "\x1b")
	if scr.Following() {
		t.Fatal("a finished job does not follow")
	}
	if !strings.Contains(scr.View(), "\x1b[31m") {
		t.Fatal("SGR colors of the log must stay")
	}
}

func TestFoldKeys(t *testing.T) {
	d, _, _, _ := setup(t, gitlab.StatusFailed, sample)
	d.Key("g", "j", "j") // cursor on "Using docker image" inside prepare
	d.Key("space")
	d.RequireView("▸ Preparing executor")
	d.RequireNoView("Using docker image")
	d.Key("space")
	d.RequireView("▾ Preparing executor", "Using docker image")
	d.Key("z")
	d.RequireView("▸ Preparing executor", "▸ Installing deps")
	d.Key("z")
	d.RequireView("▾ Preparing executor", "▾ Installing deps", "timeout while fetching module")
}

func TestSearchRevealsFoldedSection(t *testing.T) {
	d, _, scr, _ := setup(t, gitlab.StatusFailed, sample)
	d.RequireNoView("timeout while fetching module")
	d.Key("/")
	if !scr.CapturesInput() {
		t.Fatal("search input captures keys")
	}
	for _, r := range "TIMEOUT" {
		d.Key(string(r))
	}
	d.Key("enter")
	d.RequireView("▾ Installing deps", "timeout while fetching module", "match 1 of 1")
	if l := scr.p.Lines()[scr.cursor]; !strings.Contains(l.Plain, "timeout") {
		t.Fatalf("cursor on %q", l.Plain)
	}
}

func TestSearchNextPrevAndNoMatches(t *testing.T) {
	d, _, _, _ := setup(t, gitlab.StatusFailed, "alpha one\nbeta\nalpha two\nalpha three\n")
	d.Key("/")
	for _, r := range "alpha" {
		d.Key(string(r))
	}
	d.Key("enter")
	d.RequireView("match 1 of 3")
	d.Key("n")
	d.RequireView("match 2 of 3")
	d.Key("n", "n")
	d.RequireView("match 1 of 3")
	d.Key("N")
	d.RequireView("match 3 of 3")
	d.Key("/")
	for range 10 {
		d.Key("backspace")
	}
	for _, r := range "zzz" {
		d.Key(string(r))
	}
	d.Key("enter")
	d.RequireView("no matches")
}

func TestEmptyLogShowsJobStatus(t *testing.T) {
	d, _, _, _ := setup(t, gitlab.StatusCreated, "")
	d.RequireView("log is empty: job created")
}

func TestTruncatedLog(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 80000; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	d, _, scr, _ := setup(t, gitlab.StatusFailed, b.String())
	d.RequireView("… beginning truncated, o — open the full log in the browser", "lines")
	if scr.p.Dropped != 30000 || len(scr.p.Lines()) != 50000 {
		t.Fatalf("dropped=%d lines=%d", scr.p.Dropped, len(scr.p.Lines()))
	}
}

func TestFollowRunningJobAndNewLines(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&b, "step %d\n", i)
	}
	d, _, scr, c := setup(t, gitlab.StatusRunning, b.String())
	if !scr.Following() {
		t.Fatal("running jobs follow by default")
	}
	d.RequireView("● follow", "step 100")

	c.AppendTrace(7, "step 101\nstep 102\n")
	n := c.CallCount("Jobs.Trace")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if c.CallCount("Jobs.Trace") != n+1 {
		t.Fatal("tick must refetch the log")
	}
	d.RequireView("step 102")

	d.Key("k")
	if scr.Following() {
		t.Fatal("scrolling up turns follow off")
	}
	d.RequireView("○ paused")
	c.AppendTrace(7, "step 103\n")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if strings.Contains(d.View(), "step 103") {
		t.Fatal("new lines must not move the view while follow is off")
	}
	d.Key("f")
	d.RequireView("● follow", "step 103")
	d.Key("k", "k", "G")
	d.RequireView("● follow", "step 103")
}

func TestIncrementalParsingAndFinalStop(t *testing.T) {
	d, _, scr, c := setup(t, gitlab.StatusRunning, "one\ntw")
	d.RequireView("one", "tw")
	c.AppendTrace(7, "o\nthree\n")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	d.RequireView("two", "three")
	if scr.consumed != len("one\ntwo\nthree\n") {
		t.Fatalf("consumed = %d", scr.consumed)
	}
	c.UpdatePipeline(path, 91840, func(p *gitlab.Pipeline) { p.Jobs[0].Status = gitlab.StatusSuccess })
	c.AppendTrace(7, "done\n")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	d.RequireView("done", "✓ success")
	if scr.polling() {
		t.Fatal("polling must stop after the final status")
	}
	n := c.CallCount("Jobs.Trace")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if c.CallCount("Jobs.Trace") != n {
		t.Fatal("no polling after the final status")
	}
}

func TestShorterTraceReparsesWholeLog(t *testing.T) {
	d, _, scr, c := setup(t, gitlab.StatusRunning, "old line 1\nold line 2\nold line 3\n")
	c.SetTrace(7, "new\n")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	d.RequireView("new")
	d.RequireNoView("old line")
}

func TestHorizontalScroll(t *testing.T) {
	long := strings.Repeat("abcdefghij", 20) + "END\n"
	d, _, _, _ := setup(t, gitlab.StatusFailed, long)
	d.RequireNoView("END")
	for range 25 {
		d.Key("right")
	}
	d.RequireView("END")
	for range 30 {
		d.Key("left")
	}
	d.RequireView("abcdefghij")
}

func TestRetryShowsNewJobLog(t *testing.T) {
	d, _, scr, c := setup(t, gitlab.StatusFailed, "old failure\n")
	d.Key("R")
	if c.CallCount("Jobs.Retry") != 1 || scr.Job().ID == 7 {
		t.Fatalf("retry calls=%d job=%d", c.CallCount("Jobs.Retry"), scr.Job().ID)
	}
	d.RequireView("retrying test:integration", "● follow")
	d.RequireNoView("old failure")
}

func TestCancelNeedsConfirmation(t *testing.T) {
	d, _, _, c := setup(t, gitlab.StatusRunning, "x\n")
	d.Key("c")
	d.RequireView("Cancel job test:integration?")
	d.Key("esc")
	if c.CallCount("Jobs.Cancel") != 0 {
		t.Fatal("cancel only after yes")
	}
	d.Key("c", "y")
	if c.CallCount("Jobs.Cancel") != 1 {
		t.Fatal("confirmed")
	}
	d.RequireView("test:integration canceled")
}

func TestOpenInBrowserAndNarrow(t *testing.T) {
	d, root, _, _ := setup(t, gitlab.StatusFailed, "x\n")
	var opened string
	root.Env().Browser = func(u string) error { opened = u; return nil }
	d.Key("o")
	if opened != "https://gitlab.example.com/job/7" {
		t.Fatalf("opened %q", opened)
	}
	d.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, l := range strings.Split(d.View(), "\n") {
		if len([]rune(l)) > 80 {
			t.Fatalf("wide line: %q", l)
		}
	}
}

// setupAttempt opens attempt 1 of 2 of a retried job: #5 failed, #7 is the latest.
func setupAttempt(t *testing.T, latest gitlab.PipelineStatus) (*uitest.Driver, *app.Model, *Screen, *fake.Client) {
	t.Helper()
	return setupAttempts(t, latest, 1)
}

// setupAttempts opens attempt sel (0 is the latest) of the retried job of
// setupAttempt.
func setupAttempts(t *testing.T, latest gitlab.PipelineStatus, sel int) (*uitest.Driver, *app.Model, *Screen, *fake.Client) {
	t.Helper()
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	old := gitlab.Job{ID: 5, Name: "test:integration", Stage: "test", Status: gitlab.StatusFailed, Duration: 30 * time.Second, WebURL: "https://gitlab.example.com/job/5"}
	job := gitlab.Job{ID: 7, Name: "test:integration", Stage: "test", Status: latest, WebURL: "https://gitlab.example.com/job/7", Attempts: []gitlab.Job{old}}
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91840}, Jobs: []gitlab.Job{job}})
	c.SetTrace(5, "first try\nFAIL: TestFlaky\n")
	c.SetTrace(7, "second try\n")
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	scr := NewAttempt(env, gitlab.ProjectRef{ID: 42, FullPath: path}, 91840, job, sel)
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr, c
}

func TestEarlierAttemptIsLoadedOnce(t *testing.T) {
	d, root, scr, c := setupAttempt(t, gitlab.StatusRunning)
	d.RequireView("#91840 › test:integration", "✗ failed", "30s · attempt 1/2", "○ paused", "FAIL: TestFlaky")
	d.RequireNoView("second try")
	if scr.Following() || scr.polling() {
		t.Fatal("an earlier attempt neither follows nor polls")
	}
	n := c.CallCount("Jobs.Trace")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if c.CallCount("Jobs.Trace") != n {
		t.Fatal("an earlier attempt is not polled")
	}
	var opened string
	root.Env().Browser = func(u string) error { opened = u; return nil }
	d.Key("o")
	if opened != "https://gitlab.example.com/job/5" {
		t.Fatalf("o opened %q, want the attempt on screen", opened)
	}
}

func TestEarlierAttemptActsOnLatest(t *testing.T) {
	d, _, scr, c := setupAttempt(t, gitlab.StatusRunning)
	d.Key("c")
	d.RequireView("Cancel job test:integration?")
	d.Key("y")
	if c.CallCount("Jobs.Cancel") != 1 {
		t.Fatal("cancel after yes")
	}
	for _, cl := range c.Calls() {
		if cl.Method == "Jobs.Cancel" && cl.Args[1] != int64(7) {
			t.Fatalf("cancel of %v, want the latest attempt", cl.Args[1])
		}
	}

	d.Key("R")
	for _, cl := range c.Calls() {
		if cl.Method == "Jobs.Retry" && cl.Args[1] != int64(7) {
			t.Fatalf("retry of %v, want the latest attempt", cl.Args[1])
		}
	}
	if c.CallCount("Jobs.Retry") != 1 || scr.Job().ID == 5 || scr.Job().ID == 7 {
		t.Fatalf("retry calls=%d job=%d", c.CallCount("Jobs.Retry"), scr.Job().ID)
	}
	d.RequireView("retrying test:integration", "● follow", "attempt 3/3")
	d.RequireNoView("attempt 1/2", "FAIL: TestFlaky")
	if !scr.polling() {
		t.Fatal("the new job is polled")
	}
	// The new job is the latest of three attempts; the list grew without a
	// request.
	if len(scr.jobs) != 3 || scr.jobs[0].ID != scr.Job().ID || scr.jobs[1].ID != 7 || scr.jobs[2].ID != 5 {
		t.Fatalf("attempts after retry: %+v", scr.jobs)
	}
	if n := c.CallCount("Pipelines.Get"); n != 0 {
		t.Fatalf("Pipelines.Get calls = %d", n)
	}
	d.Key("[", "[")
	d.RequireView("attempt 1/3", "FAIL: TestFlaky")
}

func TestAttemptLabelOnLatest(t *testing.T) {
	d, _, _, _ := setupAttempts(t, gitlab.StatusSuccess, 0)
	d.RequireView("✓ success", "· attempt 2/2", "second try")
	d.Key("?")
	d.RequireView("[/]", "prev/next attempt", "attempts")
	d.Key("esc")

	d, _, _, _ = setup(t, gitlab.StatusFailed, "x\n")
	d.RequireNoView("attempt")
	d.Key("?")
	d.RequireNoView("[/]", "attempt")
	d.Key("esc")
	d.Key("a", "[", "]") // nothing to switch to
	d.RequireNoView("Attempts of")
}

func TestSwitchAttemptsInPlace(t *testing.T) {
	d, _, scr, c := setupAttempts(t, gitlab.StatusRunning, 0)
	d.RequireView("· attempt 2/2", "● follow", "second try")
	if !scr.polling() {
		t.Fatal("the running latest attempt is polled")
	}
	d.Key("]") // already the newest
	if scr.Job().ID != 7 {
		t.Fatalf("job = %d", scr.Job().ID)
	}

	// A search is dropped with the attempt.
	d.Key("/", "t", "r", "y", "enter")
	d.RequireView("match 1 of 1")
	d.Key("[")
	d.RequireView("✗ failed", "30s · attempt 1/2", "○ paused", "FAIL: TestFlaky")
	d.RequireNoView("second try", "match 1 of", "● follow")
	if scr.Job().ID != 5 || scr.Following() || scr.polling() {
		t.Fatalf("job %d: follow=%v polling=%v", scr.Job().ID, scr.Following(), scr.polling())
	}
	n := c.CallCount("Jobs.Trace")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	d.Key("[") // already the oldest
	if c.CallCount("Jobs.Trace") != n || scr.Job().ID != 5 {
		t.Fatal("an earlier attempt is loaded once; [ on the oldest does nothing")
	}

	d.Key("]")
	d.RequireView("· attempt 2/2", "● follow", "second try")
	d.RequireNoView("FAIL: TestFlaky")
	if !scr.polling() {
		t.Fatal("back on the running latest attempt, polling resumes")
	}
	n = c.CallCount("Jobs.Trace")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if c.CallCount("Jobs.Trace") != n+1 {
		t.Fatal("the latest attempt is polled again")
	}
}

func TestAttemptsOverlaySwitchesInPlace(t *testing.T) {
	d, root, scr, _ := setupAttempts(t, gitlab.StatusSuccess, 0)
	d.Key("a")
	d.RequireView("Attempts of test:integration", "#7  ✓ success", "latest", "#5  ✗ failed   30s")
	d.Key("enter") // the attempt on screen: nothing changes
	d.RequireNoView("Attempts of")
	d.Key("a", "j", "enter")
	d.RequireNoView("Attempts of")
	d.RequireView("attempt 1/2", "FAIL: TestFlaky")
	if root.Depth() != 1 || root.Top() != app.Screen(scr) {
		t.Fatalf("depth = %d, top = %T", root.Depth(), root.Top())
	}
	d.Key("a") // the shown attempt is selected
	d.Key("k", "enter")
	d.RequireView("attempt 2/2", "second try")
}
