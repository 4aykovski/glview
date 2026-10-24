package issueactions

import (
	"os"
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

var project = gitlab.ProjectRef{ID: 42, FullPath: path}

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

// host turns StateMsg into the toast, as a screen does.
type host struct{ env *app.Env }

func (host) Init() tea.Cmd { return nil }
func (h host) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	if m, ok := msg.(StateMsg); ok {
		return h, Done(h.env, m)
	}
	return h, nil
}
func (host) View() string          { return "host" }
func (host) Title() string         { return "host" }
func (host) KeyMap() common.KeyMap { return common.KeyMap{} }
func (host) SetSize(int, int)      {}

func start(t *testing.T, lang i18n.Lang) (*uitest.Driver, *app.Env, *fake.Client) {
	t.Helper()
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: project})
	c.AddIssue(path, gitlab.Issue{IID: 1318, Title: "Surge coefficient ignores zone overrides"})
	c.AddIssue(path, gitlab.Issue{IID: 1260, Title: "Old report", State: "closed"})
	env := app.NewEnv(lang)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	return uitest.New(t, app.New(env, host{env: env}, app.Options{})), env, c
}

func issue(t *testing.T, c *fake.Client, iid int64) gitlab.Issue {
	t.Helper()
	i, err := c.Issues().Get(t.Context(), project, iid)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func TestCloseAsksAndCloses(t *testing.T) {
	d, env, c := start(t, i18n.EN)
	d.Run(Toggle(env, common.Token{}, project, issue(t, c, 1318)))
	d.RequireView("Close issue #1318?", "Surge coefficient ignores zone overrides")
	if c.CallCount("Issues.SetState") != 0 {
		t.Fatal("nothing happens before the confirm")
	}
	d.Key("y")
	calls := c.CallsOf("Issues.SetState")
	if len(calls) != 1 || calls[0][1] != int64(1318) || calls[0][2] != true {
		t.Fatalf("SetState calls = %v", calls)
	}
	d.RequireView("Issue #1318 closed")
	if issue(t, c, 1318).State != "closed" {
		t.Fatal("the issue must be closed")
	}
}

func TestEnterIsCancel(t *testing.T) {
	d, env, c := start(t, i18n.EN)
	d.Run(Toggle(env, common.Token{}, project, issue(t, c, 1318)))
	d.Key("enter")
	if c.CallCount("Issues.SetState") != 0 {
		t.Fatal("enter in the confirm is Cancel")
	}
	d.RequireNoView("Close issue")
}

func TestReopenWithoutConfirm(t *testing.T) {
	d, env, c := start(t, i18n.RU)
	d.Run(Toggle(env, common.Token{}, project, issue(t, c, 1260)))
	d.RequireNoView("Закрыть issue")
	calls := c.CallsOf("Issues.SetState")
	if len(calls) != 1 || calls[0][1] != int64(1260) || calls[0][2] != false {
		t.Fatalf("SetState calls = %v", calls)
	}
	d.RequireView("Issue #1260 открыт снова")
	if issue(t, c, 1260).State != "opened" {
		t.Fatal("the issue must be open")
	}
}

func TestForbiddenIsToast(t *testing.T) {
	d, env, c := start(t, i18n.EN)
	c.FailNext("Issues.SetState", &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403})
	d.Run(Toggle(env, common.Token{}, project, issue(t, c, 1318)))
	d.Key("y")
	d.RequireView("Not enough permissions")
	if issue(t, c, 1318).State != "opened" {
		t.Fatal("the issue stays open")
	}
}
