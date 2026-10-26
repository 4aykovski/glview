package workspace

import (
	"regexp"
	"slices"
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

func linkedFake() *fake.Client {
	c := newFake()
	c.UpdateMR("platform/route-service", 482, func(m *gitlab.MergeRequest) {
		m.Title = "ABC-123: add surge pricing"
		m.Description = "Closes #12"
		m.DescriptionLinks = []gitlab.Link{{Text: "#12", URL: issueURL}}
	})
	return c
}

func openLinked(t *testing.T, client *fake.Client, perProject int) (*uitest.Driver, *app.Model) {
	t.Helper()
	env := newEnv(client)
	env.Settings.PerProject = perProject
	env.Settings.LinkRules = []mrlinks.Rule{{Re: regexp.MustCompile(`[A-Z]+-\d+`), URL: "https://tracker.example.com/browse/$0"}}
	withState(t, env, []string{"platform/route-service"}, nil)
	root := app.New(env, New(env, source, testScope()), app.Options{})
	return uitest.New(t, root), root
}

func TestGroupMRsTitleRuleIsOSC8(t *testing.T) {
	d, _ := openLinked(t, linkedFake(), 0)
	d.Key("2", "j")
	raw := d.Model.View().Content
	want := ansi.SetHyperlink(trackerURL) + "ABC-123" + ansi.ResetHyperlink()
	if strings.Count(raw, want) != 2 { // the row and the preview
		t.Fatalf("want ABC-123 as a link in the row and the preview, got %d", strings.Count(raw, want))
	}
	d.RequireNoView(trackerURL)
}

func TestGroupMRsLinksKeyUsesRowProject(t *testing.T) {
	client := linkedFake()
	d, root := openLinked(t, client, 0)
	var opened []string
	root.Env().Browser = func(u string) error { opened = append(opened, u); return nil }
	d.Key("2", "j", "L")
	calls := client.CallsOf("MergeRequests.Get")
	if len(calls) != 1 || calls[0][0] != "platform/route-service" || calls[0][1] != int64(482) {
		t.Fatalf("Get calls = %v", calls)
	}
	d.RequireView("Links · !482", "ABC-123 — "+trackerURL, "#12 — "+issueURL)
	d.Key("j", "enter")
	if len(opened) != 1 || opened[0] != issueURL {
		t.Fatalf("opened = %v", opened)
	}
}

func TestGroupMRsLinksKeyOnHeaderAndMore(t *testing.T) {
	client := linkedFake()
	d, root := openLinked(t, client, 1)
	hasL := func() bool {
		for _, b := range root.Top().KeyMap().Short {
			if slices.Contains(b.Keys(), "L") {
				return true
			}
		}
		return false
	}
	d.Key("2")
	d.RequireView("more") // "↳ N more": the group shows one MR of four
	d.Key("L")            // on the group header
	d.Key("j", "j")
	d.Key("L") // on the "more" line
	if n := client.CallCount("MergeRequests.Get"); n != 0 {
		t.Fatalf("L outside a record must not ask for details: %d", n)
	}
	d.RequireNoView("Links ·", "No links in the MR")
	if hasL() {
		t.Fatal("L is not offered on a \"more\" line")
	}
	d.Key("k")
	if !hasL() {
		t.Fatal("L is offered on a record")
	}
	d.Key("K")
	if hasL() {
		t.Fatal("L is not offered on a group header")
	}
}

func TestGroupMRsApproversKeyUsesRowProject(t *testing.T) {
	client := linkedFake()
	client.UpdateMR("platform/route-service", 482, func(m *gitlab.MergeRequest) {
		m.Approvals.Rules = []gitlab.ApprovalRule{{Name: "Backend", Required: 1, Eligible: []gitlab.User{{Username: "bob", Name: "Bob"}}}}
	})
	d, _ := openLinked(t, client, 0)
	d.Key("2", "j", "A")
	calls := client.CallsOf("MergeRequests.Get")
	if len(calls) != 1 || calls[0][0] != "platform/route-service" || calls[0][1] != int64(482) {
		t.Fatalf("Get calls = %v", calls)
	}
	d.RequireView("Approvers · !482", "Backend", "bob Bob")
}
