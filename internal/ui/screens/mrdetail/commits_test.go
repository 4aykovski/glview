package mrdetail

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const commitsMethod = "MergeRequests.Commits"

func testCommits() []gitlab.CommitSummary {
	return []gitlab.CommitSummary{
		{SHA: "c3d4e5f6a7b8c9d0", ShortSHA: "c3d4e5f6", Title: "slot: guard the map with a mutex", AuthorName: "Anna K", CreatedAt: now.Add(-20 * time.Minute)},
		{SHA: "b2c3d4e5f6a7b8c9", ShortSHA: "b2c3d4e5", Title: "slot: add race test", AuthorName: "Egor", CreatedAt: now.Add(-2 * time.Hour)},
	}
}

// commitsSetup opens the MR with two commits registered in the fake.
func commitsSetup(t *testing.T, mr gitlab.MergeRequest) (*uitest.Driver, *Screen, *fake.Client) {
	t.Helper()
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
	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.SHA = "d4e5f6a7" })
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

	client.UpdateMR(path, 479, func(m *gitlab.MergeRequest) { m.SHA = "e5f6a7b8" })
	client.SetMRCommits(path, 479, append([]gitlab.CommitSummary{{SHA: "e5f6a7b8c9d0", ShortSHA: "e5f6a7b8", Title: "slot: address review", AuthorName: "Anna K", CreatedAt: now}}, testCommits()...))
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("new head SHA must reload commits: %d", n)
	}
	d.RequireView("e5f6a7b8", "slot: address review", "Commits 3")
	d.Send(common.PollTickMsg{Token: scr.poll.Current()})
	if n := client.CallCount(commitsMethod); n != 2 {
		t.Fatalf("no reload once the SHA matches: %d", n)
	}
}

func TestCommitsOpenAndCopy(t *testing.T) {
	d, scr, _ := commitsSetup(t, baseMR())
	var opened []string
	scr.env.Browser = func(u string) error { opened = append(opened, u); return nil }
	base := "https://gitlab.example.com/" + path + "/-/commit/"
	d.Key("shift+tab", "o", "j", "o")
	if len(opened) != 2 || opened[0] != base+"c3d4e5f6a7b8c9d0" || opened[1] != base+"b2c3d4e5f6a7b8c9" {
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

func TestCommitURLWithoutMRURL(t *testing.T) {
	_, scr, _ := commitsSetup(t, baseMR())
	if got := scr.commitURL("abc"); got != "https://gitlab.example.com/"+path+"/-/commit/abc" {
		t.Fatalf("from the MR URL: %q", got)
	}
	scr.mr.WebURL = ""
	if got := scr.commitURL("abc"); got != "https://gitlab.example.com/"+path+"/-/commit/abc" {
		t.Fatalf("fallback: %q", got)
	}
	scr.mr.WebURL = "https://gitlab.example.com/sub/" + path + "/-/merge_requests/479"
	if got := scr.commitURL("abc"); got != "https://gitlab.example.com/sub/"+path+"/-/commit/abc" {
		t.Fatalf("relative root kept: %q", got)
	}
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
