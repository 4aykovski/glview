package localgit

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitexec"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/localgit/gittest"
)

// drain runs the listen chain to the end, feeding the tracker.
func drain(t *testing.T, tr *Tracker, cmd tea.Cmd) {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		ok, done, next := tr.Handle(msg)
		if !ok {
			t.Fatalf("unexpected %T", msg)
		}
		if done {
			return
		}
		cmd = next
	}
}

func TestTrackerRunAndSummary(t *testing.T) {
	r := gittest.New()
	r.AB["/src/route-service"] = [2]int{0, 3}
	r.Results["/src/route-planner-ui"] = gitexec.Result{Status: gitexec.StatusFailed, Reason: gitexec.ReasonAuth, Output: []string{"Permission denied (publickey)."}}
	tr := New(r)
	repos := []*gitremote.Repo{{Root: "/src/route-service"}, {Root: "/src/route-planner-ui"}, {Root: "/src/dotfiles"}}
	cmd := tr.Start(gitexec.OpFetch, repos)
	if !tr.Busy() || !tr.Running("/src/dotfiles") || tr.Start(gitexec.OpFetch, repos) != nil {
		t.Fatal("busy while running, a second start is refused")
	}
	m := i18n.Get(i18n.EN)
	if h := tr.Header(m); h != "⟳ fetch 0/3" {
		t.Fatalf("header = %q", h)
	}
	drain(t, tr, cmd)
	if tr.Busy() || tr.Header(m) != "" {
		t.Fatal("idle after the end")
	}
	text, failed, hint := tr.Summary(m)
	if text != "fetch: 2 ✓, 1 ✗ (route-planner-ui: needs authentication)" || !failed || !strings.Contains(hint, "git fetch in /src/route-planner-ui") {
		t.Fatalf("summary = %q %v %q", text, failed, hint)
	}
	s := common.NewStyles(common.TerminalTheme(true))
	if c := tr.Cell(s, "/src/route-service"); c != "↓3" {
		t.Fatalf("cell = %q", c)
	}
	if !strings.Contains(tr.Cell(s, "/src/route-planner-ui"), "✗") {
		t.Fatal("failed cell")
	}
	if tr.Cell(s, "/src/dotfiles") != "" {
		t.Fatal("no upstream: empty")
	}
}

func TestTrackerUnavailable(t *testing.T) {
	tr := New(nil)
	if tr.Available() || tr.Start(gitexec.OpFetch, []*gitremote.Repo{{Root: "/x"}}) != nil || tr.CountAheadBehind([]*gitremote.Repo{{Root: "/x"}}) != nil {
		t.Fatal("no git: nothing runs")
	}
	var nilT *Tracker
	if nilT.Busy() || nilT.Available() || nilT.Header(i18n.Get(i18n.EN)) != "" {
		t.Fatal("nil tracker")
	}
}

func TestABText(t *testing.T) {
	if ABText(AB{Ahead: 1, Behind: 3, Known: true}) != "↓3 ↑1" || ABText(AB{Known: true}) != "" || ABText(AB{Behind: 2}) != "" {
		t.Fatal("ab text")
	}
}
