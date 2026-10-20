package mrdetail

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const commitsMethod = "MergeRequests.Commits"

const commitBase = "https://gitlab.example.com/" + path + "/-/commit/"

// commit is a fake MR commit with its web URL.
func commit(sha, title, author string, at time.Time) gitlab.CommitSummary {
	return gitlab.CommitSummary{SHA: sha, ShortSHA: sha[:8], Title: title, AuthorName: author, WebURL: commitBase + sha, CreatedAt: at}
}

// testCommits are the MR commits, newest (the head) first.
func testCommits() []gitlab.CommitSummary {
	return []gitlab.CommitSummary{
		commit("c3d4e5f6a7b8c9d0", "slot: guard the map with a mutex", "Anna K", now.Add(-20*time.Minute)),
		commit("b2c3d4e5f6a7b8c9", "slot: add race test", "Egor", now.Add(-2*time.Hour)),
	}
}

// commitsSetup opens the MR with two commits registered in the fake; the MR
// head SHA is the first of them.
func commitsSetup(t *testing.T, mr gitlab.MergeRequest) (*uitest.Driver, *Screen, *fake.Client) {
	t.Helper()
	mr.SHA = testCommits()[0].SHA
	d, _, scr, client := setup(t, mr)
	client.SetMRCommits(path, mr.IID, testCommits())
	return d, scr, client
}

func plainTabBar(s *Screen) string { return ansi.Strip(s.tabBar()) }

func TestCommitsTabLoadsLazilyOnce(t *testing.T) {
	d, scr, client := commitsSetup(t, baseMR())
	d.Key("tab", "tab")
	if n := client.CallCount(commitsMethod); n != 0 {
		t.Fatalf("commits requested before the tab was opened: %d", n)
	}
	d.Key("tab")
	if scr.tab != tabCommits {
		t.Fatalf("tab = %d", scr.tab)
	}
	if n := client.CallCount(commitsMethod); n != 1 {
		t.Fatalf("first visit: %d requests", n)
	}
	d.RequireView("SHA", "Title", "Author", "Ago",
		"c3d4e5f6", "slot: guard the map with a mutex", "Anna K", "20m",
		"b2c3d4e5", "slot: add race test", "Egor", "Commits 2")
	if v := d.View(); strings.Index(v, "c3d4e5f6") > strings.Index(v, "b2c3d4e5") {
		t.Fatal("API order (newest first) must be kept")
	}
	d.Key("tab", "tab", "tab", "tab")
	if n := client.CallCount(commitsMethod); n != 1 {
		t.Fatalf("second visit must not reload: %d requests", n)
	}
}

func TestShiftTabCyclesOverFourTabs(t *testing.T) {
	d, scr, client := commitsSetup(t, baseMR())
	d.Key("shift+tab")
	if scr.tab != tabCommits || client.CallCount(commitsMethod) != 1 {
		t.Fatalf("shift+tab from Overview: tab %d, requests %d", scr.tab, client.CallCount(commitsMethod))
	}
	d.Key("shift+tab")
	if scr.tab != tabPipelines {
		t.Fatalf("tab = %d", scr.tab)
	}
	d.Key("tab", "tab")
	if scr.tab != tabOverview {
		t.Fatalf("tab wraps to Overview, got %d", scr.tab)
	}
}

func TestCommitsTitleCount(t *testing.T) {
	numbered := regexp.MustCompile(`(Commits|Коммиты) \d`)
	mr := baseMR()
	_, scr, _ := commitsSetup(t, mr)
	if bar := plainTabBar(scr); !strings.Contains(bar, "Commits") || numbered.MatchString(bar) {
		t.Fatalf("unknown count: %q", bar)
	}

	mr.CommitCount = 5
	d, scr, _ := commitsSetup(t, mr)
	if bar := plainTabBar(scr); !strings.Contains(bar, "Commits 5") {
		t.Fatalf("count from the MR before loading: %q", bar)
	}
	d.Key("shift+tab")
	if bar := plainTabBar(scr); !strings.Contains(bar, "Commits 2") {
		t.Fatalf("loaded list wins: %q", bar)
	}
	d.Key("ctrl+l")
	if bar := plainTabBar(scr); !strings.Contains(bar, "Коммиты 2") {
		t.Fatalf("ru: %q", bar)
	}
}

func TestCommitsLoadingTail(t *testing.T) {
	d, _, client := commitsSetup(t, baseMR())
	client.SetDelay(commitsMethod, 10*uitest.CmdTimeout)
	d.Key("shift+tab")
	d.RequireView("SHA", "◐ loading…")
	d.RequireNoView("No commits")
}

func TestCommitsEmpty(t *testing.T) {
	d, _, _, client := setup(t, baseMR())
	d.Key("shift+tab")
	if client.CallCount(commitsMethod) != 1 {
		t.Fatal("not requested")
	}
	d.RequireView("No commits", "Commits 0")
	d.RequireNoView("loading")
}

func TestCommitsErrorToastAndRetry(t *testing.T) {
	d, _, client := commitsSetup(t, baseMR())
	client.FailNext(commitsMethod, &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	d.Key("shift+tab")
	d.RequireView("GitLab server error", "could not load")
	d.RequireNoView("c3d4e5f6")

	// Re-entering the tab retries.
	d.Key("tab", "shift+tab")
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("re-entering must retry: %d requests", n)
	}
	d.RequireView("c3d4e5f6", "Commits 2")

	// r retries as well.
	client.FailNext(commitsMethod, &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	d.Key("r")
	d.RequireView("GitLab server error", "c3d4e5f6") // the loaded list stays
	d.Key("r")
	if n := client.CallCount(commitsMethod); n != 4 {
		t.Fatalf("r must reload: %d requests", n)
	}
}

func TestCommitsReloadOnRefreshOnlyAfterOpening(t *testing.T) {
	d, _, client := commitsSetup(t, baseMR())
	d.Key("r")
	if n := client.CallCount(commitsMethod); n != 0 {
		t.Fatalf("r before the tab was opened: %d requests", n)
	}
	d.Key("shift+tab", "shift+tab", "r")
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("r after opening (on another tab too): %d requests", n)
	}
}

func TestCommitsReloadOnHeadSHAChangeNotOnPoll(t *testing.T) {
	d, scr, client := commitsSetup(t, baseMR())

	// Before the tab is opened a new SHA does not request commits.
	pushed := append([]gitlab.CommitSummary{commit("d4e5f6a7b8c9d0e1", "slot: fix typo", "Egor", now)}, testCommits()...)
	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.SHA = pushed[0].SHA })
	client.SetMRCommits(path, 479, pushed)
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 0 {
		t.Fatalf("SHA change before opening: %d requests", n)
	}

	d.Key("shift+tab")
	if n := client.CallCount(commitsMethod); n != 1 {
		t.Fatalf("open: %d", n)
	}
	gets := client.CallCount("MergeRequests.Get")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if client.CallCount("MergeRequests.Get") != gets+1 {
		t.Fatal("tick must reload the MR")
	}
	if n := client.CallCount(commitsMethod); n != 1 {
		t.Fatalf("tick with the same SHA must not reload commits: %d", n)
	}

	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.SHA = "e5f6a7b8c9d0e1f2" })
	client.SetMRCommits(path, 479, append([]gitlab.CommitSummary{commit("e5f6a7b8c9d0e1f2", "slot: address review", "Anna K", now)}, pushed...))
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("new head SHA must reload commits: %d", n)
	}
	d.RequireView("e5f6a7b8", "slot: address review", "Commits 4")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("no reload once the SHA matches: %d", n)
	}
}

func TestCommitsOpenAndCopy(t *testing.T) {
	d, scr, _ := commitsSetup(t, baseMR())
	var opened []string
	scr.env.Browser = func(u string) error { opened = append(opened, u); return nil }
	d.Key("shift+tab", "o", "j", "o")
	if len(opened) != 2 || opened[0] != commitBase+"c3d4e5f6a7b8c9d0" || opened[1] != commitBase+"b2c3d4e5f6a7b8c9" {
		t.Fatalf("opened = %q", opened)
	}
	d.Key("y")
	d.RequireView("copied: …/commit/b2c3d4e5f6a7b8c9")
	d.Key("enter")
	if scr.tab != tabCommits || len(opened) != 2 {
		t.Fatal("enter on a commit does nothing")
	}
}

func TestCommitsLinkFallsBackToMR(t *testing.T) {
	d, _, scr, _ := setup(t, baseMR())
	var opened []string
	scr.env.Browser = func(u string) error { opened = append(opened, u); return nil }
	d.Key("shift+tab", "o")
	if len(opened) != 1 || opened[0] != baseMR().WebURL {
		t.Fatalf("empty list opens the MR: %q", opened)
	}
	d.Key("y")
	d.RequireView("copied: …/merge_requests/479")
}

func TestCommitsLinkWithoutWebURLFallsBackToMR(t *testing.T) {
	d, scr, client := commitsSetup(t, baseMR())
	cs := testCommits()
	cs[1].WebURL = ""
	client.SetMRCommits(path, 479, cs)
	var opened []string
	scr.env.Browser = func(u string) error { opened = append(opened, u); return nil }
	d.Key("shift+tab", "j", "o")
	if len(opened) != 1 || opened[0] != baseMR().WebURL {
		t.Fatalf("a commit without web_url opens the MR: %q", opened)
	}
	d.Key("y")
	d.RequireView("copied: …/merge_requests/479")
}

func TestCommitsFooter(t *testing.T) {
	d, _, _ := commitsSetup(t, baseMR())
	d.Send(tea.WindowSizeMsg{Width: 200, Height: 40})
	d.RequireView("enter open", "o browser")
	d.Key("shift+tab")
	d.RequireView("j/k down", "o commit in browser", "y copy")
	d.RequireNoView("enter open")
	d.Key("ctrl+l")
	d.RequireView("o коммит в браузере")
}

func TestCommitsCachedOldHeadReloadedOnce(t *testing.T) {
	mr := baseMR()
	mr.SHA = "d4e5f6a7b8c9d0e1"
	d, _, scr, client := setup(t, mr)
	scr.env.Cache = cache.New()
	pushed := append([]gitlab.CommitSummary{commit(mr.SHA, "slot: fix typo", "Egor", now)}, testCommits()...)
	client.SetMRCommits(path, 479, pushed)
	scr.env.Cache.Set(scr.key("mrcommits"), testCommits()) // fresh, but of the old head

	d.Key("shift+tab")
	if n := client.CallCount(commitsMethod); n != 1 {
		t.Fatalf("a cached list of an old head is reloaded past the cache once: %d requests", n)
	}
	d.RequireView("d4e5f6a7", "slot: fix typo", "Commits 3")
}

func TestCommitsForcedOldHeadAcceptedAsIs(t *testing.T) {
	mr := baseMR()
	mr.SHA = "d4e5f6a7b8c9d0e1" // GitLab lags: the commits still end at the old head
	d, _, scr, client := setup(t, mr)
	client.SetMRCommits(path, 479, testCommits())

	d.Key("shift+tab")
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("plain load + one forced reload, got %d requests", n)
	}
	d.RequireView("c3d4e5f6", "Commits 2")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("the forced answer is final, ticks do not retry: %d", n)
	}
}

func TestCommitsRefreshRequestsOnce(t *testing.T) {
	d, scr, client := commitsSetup(t, baseMR())
	d.Key("shift+tab")
	if n := client.CallCount(commitsMethod); n != 1 {
		t.Fatalf("open: %d", n)
	}

	// r without a new head SHA: one request.
	d.Key("r")
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("r: %d requests", n)
	}

	// r after a push: one request, at the new head.
	pushed := append([]gitlab.CommitSummary{commit("d4e5f6a7b8c9d0e1", "slot: fix typo", "Egor", now)}, testCommits()...)
	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.SHA = pushed[0].SHA })
	client.SetMRCommits(path, 479, pushed)
	d.Key("r")
	if n := client.CallCount(commitsMethod); n != 3 {
		t.Fatalf("r with a new head SHA: %d requests", n)
	}
	if scr.commits.reqSHA != pushed[0].SHA {
		t.Fatalf("requested at %q", scr.commits.reqSHA)
	}
	d.RequireView("slot: fix typo", "Commits 3")

	// r while the MR details fail: commits are still reloaded once.
	client.FailNext("MergeRequests.Get", &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	d.Key("r")
	if n := client.CallCount(commitsMethod); n != 4 {
		t.Fatalf("r with failed MR details: %d requests", n)
	}
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 4 {
		t.Fatalf("the pending refresh is spent: %d requests", n)
	}
}

func TestCommitsAuthRestoredRequestsOnce(t *testing.T) {
	d, _, client := commitsSetup(t, baseMR())
	d.Key("shift+tab")
	d.Send(app.AuthRestoredMsg{})
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("auth restored: %d requests", n)
	}
}

func TestCommitsCountFollowsHeadSHA(t *testing.T) {
	five := []gitlab.CommitSummary{
		commit("e5e5e5e5e5e5e5e5", "five", "Egor", now),
		commit("d4d4d4d4d4d4d4d4", "four", "Egor", now),
		commit("c3c3c3c3c3c3c3c3", "three", "Egor", now),
		commit("b2b2b2b2b2b2b2b2", "two", "Egor", now),
		commit("a1a1a1a1a1a1a1a1", "one", "Egor", now),
	}
	mr := baseMR()
	mr.SHA, mr.CommitCount = five[0].SHA, 5
	d, _, scr, client := setup(t, mr)
	client.SetMRCommits(path, 479, five)
	d.Key("shift+tab")
	if bar := plainTabBar(scr); !strings.Contains(bar, "Commits 5") {
		t.Fatalf("loaded: %q", bar)
	}

	// A push of two commits; the reload fails: the MR's count wins.
	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.SHA, m.CommitCount = "f6f6f6f6f6f6f6f6", 7 })
	client.FailNext(commitsMethod, &gitlab.Error{Kind: gitlab.ErrServer, Status: 500})
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("new head: %d requests", n)
	}
	if bar := plainTabBar(scr); !strings.Contains(bar, "Commits 7") {
		t.Fatalf("list behind the head: %q", bar)
	}
	d.RequireView("five") // the old list stays

	// Ticks at the same SHA do not hammer the failed request.
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("tick after a failed reload: %d requests", n)
	}

	// A successful reload: the list's own length again.
	six := append([]gitlab.CommitSummary{commit("f6f6f6f6f6f6f6f6", "six", "Egor", now)}, five...)
	client.SetMRCommits(path, 479, six)
	d.Key("r")
	if bar := plainTabBar(scr); !strings.Contains(bar, "Commits 6") {
		t.Fatalf("reloaded: %q", bar)
	}
}

// rowEnd returns the trimmed end of the view line holding sub.
func rowEnd(t *testing.T, d *uitest.Driver, sub string) string {
	t.Helper()
	for _, l := range strings.Split(d.View(), "\n") {
		if strings.Contains(l, sub) {
			f := strings.Fields(l)
			return f[len(f)-1]
		}
	}
	t.Fatalf("no line with %q:\n%s", sub, d.View())
	return ""
}

func TestAgoRefreshedOnEveryMRAnswer(t *testing.T) {
	d, scr, _ := commitsSetup(t, baseMR())
	clock := now
	scr.env.Now = func() time.Time { return clock }

	d.Key("shift+tab", "j")
	if got := rowEnd(t, d, "c3d4e5f6"); got != "20m" {
		t.Fatalf("commit ago = %q", got)
	}
	clock = now.Add(10 * time.Hour)
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if got := rowEnd(t, d, "c3d4e5f6"); got != "10h" {
		t.Fatalf("commit ago after a poll = %q", got)
	}
	if c := scr.selectedCommit(); c == nil || c.SHA != "b2c3d4e5f6a7b8c9" {
		t.Fatalf("the selected row survives the rebuild: %+v", c)
	}

	d.Key("shift+tab")
	if got := rowEnd(t, d, "#91700"); got != "13h" {
		t.Fatalf("pipeline ago after a poll = %q", got)
	}
}
