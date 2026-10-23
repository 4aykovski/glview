package project

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/mrlinks"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const (
	trackerURL = "https://tracker.example.com/browse/ABC-123"
	issueURL   = "https://gitlab.example.com/platform/route-service/-/issues/12"
)

func withRule(env *app.Env) {
	env.Settings.LinkRules = []mrlinks.Rule{{Re: regexp.MustCompile(`[A-Z]+-\d+`), URL: "https://tracker.example.com/browse/$0"}}
}

func linkedFake() *fake.Client {
	c := newFake(3)
	c.UpdateMR(path, 1, func(m *gitlab.MergeRequest) {
		m.Title = "ABC-123: fix slot race"
		m.Description = "Closes #12"
		m.DescriptionLinks = []gitlab.Link{{Text: "#12", URL: issueURL}}
	})
	return c
}

func openLinked(t *testing.T, client *fake.Client) (*uitest.Driver, *app.Model, *Project) {
	t.Helper()
	env := newEnv(client)
	withRule(env)
	scr := New(env, Options{Path: path, Repo: nil})
	root := app.New(env, scr, app.Options{})
	return uitest.New(t, root), root, scr
}

func TestTitleRuleIsOSC8InListAndPreview(t *testing.T) {
	d, _, _ := openLinked(t, linkedFake())
	raw := d.Model.View().Content
	want := ansi.SetHyperlink(trackerURL) + "ABC-123" + ansi.ResetHyperlink()
	if strings.Count(raw, want) != 2 { // the row and the preview
		t.Fatalf("want ABC-123 as a link in the row and the preview, got %d", strings.Count(raw, want))
	}
	d.RequireNoView(trackerURL)

	plain := newFake(3)
	plain.UpdateMR(path, 1, func(m *gitlab.MergeRequest) { m.Title = "ABC-123: fix slot race"; m.Description = "Closes #12" })
	d2, _, _ := open(t, plain, nil)
	if d.View() != d2.View() {
		t.Errorf("links changed the visible text:\n%s\n---\n%s", d.View(), d2.View())
	}
}

func TestLinksKeyRequestsDetails(t *testing.T) {
	client := linkedFake()
	d, root, _ := openLinked(t, client)
	var opened []string
	root.Env().Browser = func(u string) error { opened = append(opened, u); return nil }
	d.Key("L")
	if client.CallCount("MergeRequests.Get") != 1 {
		t.Fatalf("Get calls = %d", client.CallCount("MergeRequests.Get"))
	}
	// Details bring the HTML link, the rule and the row's own text match.
	d.RequireView("Links · !1", "ABC-123 — "+trackerURL, "#12 — "+issueURL)
	d.Key("j", "enter")
	if len(opened) != 1 || opened[0] != issueURL {
		t.Fatalf("opened = %v", opened)
	}
}

func TestLinksNone(t *testing.T) {
	client := newFake(2)
	d, _, _ := openLinked(t, client)
	d.Key("L")
	d.RequireView("No links in the MR")
	d.RequireNoView("Links · !1")
}

func TestLinksAnswerForAnotherMRIsDropped(t *testing.T) {
	client := linkedFake()
	env := newEnv(client)
	withRule(env)
	scr := New(env, Options{Path: path, Repo: nil})
	mrs := scr.tabs[0].(*mrsTab)
	mrs.SetSize(120, 30)
	scr.Update(mrs.loadFirst(true, false)())
	if len(mrs.Items()) != 3 {
		t.Fatalf("items = %d", len(mrs.Items()))
	}
	same := mrs.onKey(uitest.KeyPress("L"))
	if _, out := scr.Update(same()); out == nil {
		t.Fatalf("the answer for the selected MR must be accepted")
	}
	ask := mrs.onKey(uitest.KeyPress("L"))
	mrs.onKey(uitest.KeyPress("j")) // the cursor leaves !1 before the answer
	_, out := scr.Update(ask())
	if out != nil {
		t.Fatalf("the answer for !1 must be dropped, got a command")
	}
}

func TestLinksStaleGenerationIsDropped(t *testing.T) {
	client := linkedFake()
	env := newEnv(client)
	scr := New(env, Options{Path: path, Repo: nil})
	mrs := scr.tabs[0].(*mrsTab)
	mrs.SetSize(120, 30)
	scr.Update(mrs.loadFirst(true, false)())
	first := mrs.onKey(uitest.KeyPress("L"))
	second := mrs.onKey(uitest.KeyPress("L"))
	if _, out := scr.Update(first()); out != nil {
		t.Fatalf("the old request must be dropped")
	}
	if _, out := scr.Update(second()); out == nil {
		t.Fatalf("the current request must be accepted")
	}
}

func TestLinksErrorFallsBackToRow(t *testing.T) {
	client := linkedFake()
	client.FailNext("MergeRequests.Get", errors.New("boom"))
	d, _, _ := openLinked(t, client)
	d.Key("L")
	// The error toast, and the overlay by the row: only the rule matches there.
	d.RequireView("Links · !1", "ABC-123 — "+trackerURL)
	d.RequireNoView("#12 — ")
}

func TestLinksErrorWithoutRowLinks(t *testing.T) {
	client := newFake(1)
	client.FailNext("MergeRequests.Get", errors.New("boom"))
	d, _, _ := openLinked(t, client)
	d.Key("L")
	d.RequireNoView("Links · !1")
}
