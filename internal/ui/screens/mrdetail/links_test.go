package mrdetail

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/mrlinks"
)

const (
	trackerURL = "https://tracker.example.com/browse/ABC-123"
	issueURL   = "https://gitlab.example.com/platform/route-service/-/issues/12"
)

func linkedMR() gitlab.MergeRequest {
	mr := baseMR()
	mr.Title = "ABC-123: fix slot race"
	mr.Description = "Closes #12\n\nSee ABC-123 and https://docs.example.com/slots for context.\n"
	mr.TitleLinks = []gitlab.Link{{Text: "ABC-123", URL: trackerURL}}
	mr.DescriptionLinks = []gitlab.Link{{Text: "#12", URL: issueURL}, {Text: "ABC-123", URL: trackerURL}}
	return mr
}

func TestTitleAndDescriptionLinksAreOSC8(t *testing.T) {
	d, _, _, _ := setup(t, linkedMR())
	raw := d.Model.View().Content
	for _, want := range []string{
		ansi.SetHyperlink(trackerURL) + "ABC-123" + ansi.ResetHyperlink(),
		ansi.SetHyperlink(issueURL) + "#12" + ansi.ResetHyperlink(),
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("view lacks %q", want)
		}
	}
	if strings.Count(raw, ansi.SetHyperlink(trackerURL)) < 2 {
		t.Errorf("ABC-123 must be a link in the title and in the description")
	}
	// The URL is not written next to the text.
	d.RequireNoView(issueURL, trackerURL)

	// The visible text is the same without links.
	plain := linkedMR()
	plain.TitleLinks, plain.DescriptionLinks = nil, nil
	d2, _, _, _ := setup(t, plain)
	if d.View() != d2.View() {
		t.Errorf("links changed the visible text:\n%s\n---\n%s", d.View(), d2.View())
	}
}

func TestWholeTokenOnly(t *testing.T) {
	mr := baseMR()
	mr.Description = "see #12 and #1\n"
	mr.DescriptionLinks = []gitlab.Link{{Text: "#1", URL: "https://gitlab.example.com/i/1"}}
	d, _, _, _ := setup(t, mr)
	raw := d.Model.View().Content
	if strings.Count(raw, ansi.SetHyperlink("https://gitlab.example.com/i/1")) != 1 {
		t.Errorf("#1 must match only itself, not #12")
	}
}

func TestLinksOverlayOpen(t *testing.T) {
	d, root, _, _ := setup(t, linkedMR())
	var opened []string
	root.Env().Browser = func(u string) error { opened = append(opened, u); return nil }
	d.Key("L")
	d.RequireView("Links · !479", "ABC-123 — "+trackerURL, "#12 — "+issueURL, "https://docs.example.com/slots", "1/3")
	d.Key("j", "enter")
	if len(opened) != 1 || opened[0] != issueURL {
		t.Fatalf("opened = %v", opened)
	}
	d.RequireNoView("Links · !479")
}

func TestLinksOverlayCopy(t *testing.T) {
	d, _, _, _ := setup(t, linkedMR())
	d.Key("L", "y")
	d.RequireView("copied: link")
	d.RequireNoView("Links · !479")
}

func TestLinksOverlayEsc(t *testing.T) {
	d, _, _, _ := setup(t, linkedMR())
	d.Key("L", "esc")
	d.RequireNoView("Links · !479")
	d.RequireView(path + " › MRs › !479")
}

func TestNoLinksToast(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Key("L")
	d.RequireView("No links in the MR")
	d.RequireNoView("Links · !479")
}

func TestLinksFromConfigRules(t *testing.T) {
	mr := baseMR()
	mr.Title = "ABC-123: fix"
	d, root, scr, _ := setup(t, mr)
	root.Env().Settings.LinkRules = rules(t, `[A-Z]+-\d+`, "https://tracker.example.com/browse/$0")
	scr.linksOK = false
	d.Key("L")
	d.RequireView("ABC-123 — " + trackerURL)
}

func TestLinksKeyInFooter(t *testing.T) {
	d, _, _, _ := setup(t, linkedMR())
	d.Send(tea.WindowSizeMsg{Width: 200, Height: 40})
	d.RequireView("L links")
	d.Key("ctrl+l")
	d.RequireView("L ссылки")
}

func TestGoldenLinksOverlay(t *testing.T) {
	d, _, _, _ := setup(t, linkedMR())
	d.Key("L")
	d.RequireGoldenSizes("mrdetail-links")
}

func rules(t *testing.T, re, url string) []mrlinks.Rule {
	t.Helper()
	return []mrlinks.Rule{{Re: regexp.MustCompile(re), URL: url}}
}
