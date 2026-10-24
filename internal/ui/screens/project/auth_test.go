package project

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

// TestAuthLostStopsPollingOfEveryTab: after a 401 no tab of the project
// screen sends a request on its poll tick (SPEC §11.1), and after "r" the
// tabs poll again.
func TestAuthLostStopsPollingOfEveryTab(t *testing.T) {
	client := goldenFake()
	d, root, scr := open(t, client, goldenRepo(t))
	tokens := map[string]func() common.Token{
		"mrs":       func() common.Token { return scr.tabs[TabMRs].(*mrsTab).poll.Current() },
		"pipelines": func() common.Token { return scr.tabs[TabPipelines].(*pipelinesTab).poll.Current() },
		"issues":    func() common.Token { return scr.tabs[TabIssues].(*issuesTab).poll.Current() },
		"tags":      func() common.Token { return scr.tabs[TabTags].(*tagsTab).poll.Current() },
	}
	for i, name := range []string{"mrs", "pipelines", "issues", "tags"} {
		d.Key(string(rune('1' + i)))
		d.Send(app.AuthLostMsg{Err: &gitlab.Error{Kind: gitlab.ErrUnauthorized, Status: 401}})
		client.ResetCalls()
		d.Send(common.PollTickMsg{Token: tokens[name]()})
		if n := len(client.Calls()); n != 0 {
			t.Fatalf("%s: %d requests after a 401: %v", name, n, client.Calls())
		}
		d.RequireView("✗ auth")
		d.Key("r") // the token is valid again
		if root.Env().AuthLost {
			t.Fatalf("%s: r must restore the session", name)
		}
		client.ResetCalls()
		d.Send(common.PollTickMsg{Token: tokens[name]()})
		if len(client.Calls()) == 0 {
			t.Fatalf("%s: polling must resume after the token is accepted", name)
		}
	}
}
