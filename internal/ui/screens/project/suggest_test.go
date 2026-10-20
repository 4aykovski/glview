package project

import (
	"testing"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

func typeAll(d interface{ Key(...string) }, s string) {
	for _, r := range s {
		d.Key(string(r))
	}
}

func TestSuggestKeysAndStateValues(t *testing.T) {
	d, _, scr := open(t, newFake(2), nil)
	mrs := scr.tabs[0].(*mrsTab)
	d.Key("/")
	d.RequireView("state:", "MR state  opened", "scope:", "whose MRs", "reviewer:", "draft:", "labels:")
	typeAll(d, "state:")
	d.RequireView("opened", "merged", "closed", "all")
	d.Key("down", "tab")
	if mrs.fl.Value() != "state:merged " {
		t.Fatalf("value = %q", mrs.fl.Value())
	}
	d.Key("enter") // the list shows keys for the empty word: enter applies
	if mrs.filter.State != gitlab.MRStateMerged {
		t.Fatalf("state = %q", mrs.filter.State)
	}
}

func TestSuggestReviewerFromServer(t *testing.T) {
	client := newFake(2)
	client.SetMembers(path, gitlab.User{Username: "anna.k", Name: "Anna K"}, gitlab.User{Username: "andrey", Name: "Andrey"}, gitlab.User{Username: "bob", Name: "Bob"})
	env := newEnv(client)
	env.Cache = cache.New()
	scr := New(env, path, nil)
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	mrs := scr.tabs[0].(*mrsTab)
	d.Key("/")
	typeAll(d, "reviewer:an")
	d.RequireView("any") // the static value shows at once
	d.RequireNoView("Anna K")
	d.Send(mrs.fl.DebounceMsg())
	d.RequireView("anna.k", "Anna K", "andrey")
	d.RequireNoView("Bob")
	// cached for 5 minutes: asking again does not hit the server
	n := client.CallCount("Projects.Members")
	d.Key("backspace", "n")
	d.Send(mrs.fl.DebounceMsg())
	if client.CallCount("Projects.Members") != n {
		t.Fatal("suggestions must be cached")
	}
}

func TestSuggestLabelsOneCallForFastTyping(t *testing.T) {
	client := newFake(2)
	client.SetLabels(path, "backend", "bug", "docs")
	d, _, scr := open(t, client, nil)
	mrs := scr.tabs[0].(*mrsTab)
	d.Key("/")
	typeAll(d, "labels:back")
	if client.CallCount("Projects.Labels") != 0 {
		t.Fatal("typing alone must not query the server")
	}
	d.Send(mrs.fl.DebounceMsg())
	if client.CallCount("Projects.Labels") != 1 {
		t.Fatalf("labels calls = %d", client.CallCount("Projects.Labels"))
	}
	d.RequireView("backend")
	d.RequireNoView("docs")
}

func TestSuggestSourceErrorDoesNotBreakTyping(t *testing.T) {
	client := newFake(2)
	client.FailNext("Projects.Labels", &gitlab.Error{Kind: gitlab.ErrServer})
	d, _, scr := open(t, client, nil)
	mrs := scr.tabs[0].(*mrsTab)
	d.Key("/")
	typeAll(d, "labels:b")
	d.Send(mrs.fl.DebounceMsg())
	typeAll(d, "ug")
	d.Key("enter")
	if len(mrs.filter.Labels) != 1 || mrs.filter.Labels[0] != "bug" {
		t.Fatalf("labels = %v", mrs.filter.Labels)
	}
}

func TestPipelineRefSuggestionsWithNotes(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	client.SetBranches(path, "main", "feat/surge", "feat/slots")
	client.SetTags(path, gitlab.Tag{Name: "feat-1.0"}, gitlab.Tag{Name: "v1"})
	d, _, tab := openPipelines(t, client)
	d.Key("/")
	typeAll(d, "ref:feat")
	d.Send(tab.fl.DebounceMsg())
	d.RequireView("feat/surge", "branch", "feat/slots", "feat-1.0", "tag")
	d.RequireNoView("v1 ")
}

func TestPipelineUserAtMeAndStatusValues(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	d, _, tab := openPipelines(t, client)
	d.Key("/")
	typeAll(d, "status:")
	d.RequireView("created", "running", "failed")
	d.Key("ctrl+u")
	for range 10 {
		d.Key("backspace")
	}
	typeAll(d, "user:@me")
	d.Key("enter")
	if tab.filter.Username != "egor" {
		t.Fatalf("@me must become the login, got %q", tab.filter.Username)
	}
}
