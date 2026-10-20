package pipeline

import (
	"context"
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/joblog"
)

func TestAttemptsOverlayOpensEarlierAttempt(t *testing.T) {
	c := fixture()
	d, root, _ := open(t, c)
	d.Key("l") // test:unit, retried once
	d.Key("a")
	d.RequireView("Attempts of test:unit", "#3  ◐ running", "latest", "#9  ✗ failed   42s  1h", "enter log")
	d.Key("j", "enter")
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	log, ok := root.Top().(*joblog.Screen)
	if !ok || log.Job().ID != 9 {
		t.Fatalf("top = %T", root.Top())
	}
	d.RequireView("#91840 › test:unit", "✗ failed", "attempt 1/2", "○ paused", "FAIL: TestFlaky")
	if log.Following() {
		t.Fatal("an earlier attempt does not follow")
	}
	d.Key("]") // the overlay hands the log every attempt
	d.RequireView("◐ running", "attempt 2/2")
	d.Key("[")

	// R and c act on the latest attempt.
	d.Key("c")
	d.RequireView("Cancel job test:unit?")
	d.Key("y")
	if cl := lastCall(c.Calls(), "Jobs.Cancel"); cl == nil || cl.Args[1] != int64(3) {
		t.Fatalf("cancel call = %+v", cl)
	}
	d.Key("R")
	if cl := lastCall(c.Calls(), "Jobs.Retry"); cl == nil || cl.Args[1] != int64(3) {
		t.Fatalf("retry call = %+v", cl)
	}
	if log.Job().ID == 9 || log.Job().ID == 3 {
		t.Fatalf("after retry the log shows job %d", log.Job().ID)
	}
	d.RequireView("retrying test:unit", "attempt 3/3")
	d.RequireNoView("attempt 1/2", "FAIL: TestFlaky")
}

func TestAttemptsOverlayLatestAndClose(t *testing.T) {
	d, root, _ := open(t, fixture())
	d.Key("l", "a")
	d.RequireView("Attempts of test:unit")
	d.Key("esc")
	d.RequireNoView("Attempts of test:unit")
	d.Key("a", "enter") // the latest attempt is selected first
	log, ok := root.Top().(*joblog.Screen)
	if !ok || log.Job().ID != 3 {
		t.Fatalf("top = %T", root.Top())
	}
	d.RequireView("attempt 2/2")
}

func TestLogFromPipelineSwitchesAttemptsInPlace(t *testing.T) {
	d, root, _ := open(t, fixture())
	d.Key("l", "enter") // test:unit, the latest attempt
	log, ok := root.Top().(*joblog.Screen)
	if !ok || log.Job().ID != 3 || root.Depth() != 2 {
		t.Fatalf("top = %T, depth = %d", root.Top(), root.Depth())
	}
	d.RequireView("◐ running · attempt 2/2", "compiled 100%")
	d.Key("]") // already the newest
	d.Key("[")
	d.RequireView("✗ failed", "attempt 1/2", "FAIL: TestFlaky")
	d.RequireNoView("compiled 100%")
	d.Key("[") // already the oldest
	if root.Depth() != 2 || root.Top() != app.Screen(log) || log.Job().ID != 9 {
		t.Fatalf("depth = %d, job = %d", root.Depth(), log.Job().ID)
	}
	d.Key("]")
	d.RequireView("attempt 2/2", "compiled 100%")
	d.Key("a", "j", "enter")
	d.RequireView("attempt 1/2", "FAIL: TestFlaky")
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.Key("esc")
	if root.Depth() != 1 {
		t.Fatalf("esc must return to Pipeline, depth = %d", root.Depth())
	}
	d.RequireView("#91840", "test:unit")
}

func TestAttemptsKeyWithoutEarlierAttempts(t *testing.T) {
	d, root, _ := open(t, fixture())
	d.RequireNoView("attempts:") // build ran once
	d.Key("a")
	d.RequireNoView("Attempts of")
	d.Key("l", "l", "l", "a") // e2e bridge
	d.RequireNoView("Attempts of")
	if root.Depth() != 1 {
		t.Fatal("must not push")
	}
}

func TestRetryKeepsEarlierAttempts(t *testing.T) {
	c := fixture()
	d, root, scr := open(t, c)
	d.Key("l", "R")
	j, _ := scr.Selected()
	if j.ID == 3 || len(j.Attempts) != 2 || j.Attempts[0].ID != 3 || j.Attempts[1].ID != 9 {
		t.Fatalf("after retry: job %d, attempts %+v", j.ID, j.Attempts)
	}
	d.RequireView("· #3 ◐ · #9 ✗")
	d.Key("a", "G", "enter")
	log, ok := root.Top().(*joblog.Screen)
	if !ok || log.Job().ID != 9 {
		t.Fatalf("top = %T", root.Top())
	}
	d.RequireView("attempt 1/3", "FAIL: TestFlaky")
}

func TestRetryInLogRefreshesPipelineOnReturn(t *testing.T) {
	c := fixture()
	d, root, scr := open(t, c)
	d.Key("l", "enter") // the log of test:unit (#3)
	d.Key("R")
	retried := lastCall(c.Calls(), "Jobs.Retry")
	if retried == nil || retried.Args[1] != int64(3) {
		t.Fatalf("retry call = %+v", retried)
	}
	d.Key("esc")
	if root.Depth() != 1 {
		t.Fatalf("depth = %d", root.Depth())
	}
	// Back on Pipeline the new attempt is there without waiting for polling.
	j, _ := scr.Selected()
	if j.ID == 3 || len(j.Attempts) != 2 || j.Attempts[0].ID != 3 {
		t.Fatalf("after retry in the log: job %d, attempts %+v", j.ID, j.Attempts)
	}
	d.RequireView("· #3 ◐ · #9 ✗")
	// The latest attempt is the new job: R retries it, not #3 again.
	d.Key("a", "enter")
	d.Key("R")
	if cl := lastCall(c.Calls(), "Jobs.Retry"); cl == nil || cl.Args[1] != j.ID {
		t.Fatalf("second retry call = %+v, want job %d", cl, j.ID)
	}
}

func TestAttemptsOverlayUsesCurrentAttempts(t *testing.T) {
	c := fixture()
	d, root, scr := open(t, c)
	d.Key("l", "a") // test:unit: #3 is the latest, #9 the earlier attempt
	d.RequireView("Attempts of test:unit")
	// Polling brings a new attempt while the overlay is open.
	nj, err := c.Jobs().Retry(context.Background(), gitlab.ProjectRef{ID: 42, FullPath: path}, 3)
	if err != nil {
		t.Fatal(err)
	}
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	d.Key("enter") // #3, the latest when the overlay opened
	log, ok := root.Top().(*joblog.Screen)
	if !ok || log.Job().ID != 3 {
		t.Fatalf("top = %T", root.Top())
	}
	d.RequireView("attempt 2/3")
	d.Key("R")
	if cl := lastCall(c.Calls(), "Jobs.Retry"); cl == nil || cl.Args[1] != nj.ID {
		t.Fatalf("retry call = %+v, want the latest job %d", cl, nj.ID)
	}
}

func TestAttemptsKeyShownOnlyWithAttempts(t *testing.T) {
	d, _, scr := open(t, fixture())
	hasA := func() bool {
		return slices.ContainsFunc(scr.KeyMap().Short, func(b common.Binding) bool { return slices.Contains(b.Keys(), "a") })
	}
	if hasA() {
		t.Fatal("build ran once: no a")
	}
	d.Key("l") // test:unit, retried
	if !hasA() {
		t.Fatal("test:unit has an earlier attempt: a is shown")
	}
	d.Key("l", "l") // e2e bridge
	if hasA() {
		t.Fatal("no a on a bridge")
	}
}

func TestAttemptsLineOverflow(t *testing.T) {
	st := app.NewEnv(i18n.EN).Styles
	job := gitlab.Job{ID: 305, Status: gitlab.StatusSuccess, Attempts: []gitlab.Job{
		{ID: 303, Status: gitlab.StatusFailed}, {ID: 302, Status: gitlab.StatusFailed}, {ID: 301, Status: gitlab.StatusFailed},
	}}
	cases := []struct {
		w    int
		want string
	}{
		{100, "#305 ✓ · #303 ✗ · #302 ✗ · #301 ✗"},
		{33, "#305 ✓ · #303 ✗ · #302 ✗ · #301 ✗"},
		{32, "#305 ✓ · #303 ✗ · #302 ✗ · +1"},
		{24, "#305 ✓ · #303 ✗ · +2"},
		{5, "#305 ✓ · +3"}, // the latest attempt is always shown
	}
	for _, tc := range cases {
		if got := ansi.Strip(attemptsLine(st, job, tc.w)); got != tc.want {
			t.Errorf("w=%d: %q, want %q", tc.w, got, tc.want)
		}
	}
}

func lastCall(calls []fake.Call, method string) *fake.Call {
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == method {
			return &calls[i]
		}
	}
	return nil
}
