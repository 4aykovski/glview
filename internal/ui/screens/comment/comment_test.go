package comment

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

type host struct{}

func (host) Init() tea.Cmd                          { return nil }
func (h host) Update(tea.Msg) (app.Screen, tea.Cmd) { return h, nil }
func (host) View() string                           { return "host" }
func (host) Title() string                          { return "host" }
func (host) KeyMap() common.KeyMap                  { return common.KeyMap{} }
func (host) SetSize(int, int)                       {}

var issueTarget = Target{Project: project, Kind: KindIssue, IID: 1318, Title: "Surge coefficient ignores zone overrides"}
var mrTarget = Target{Project: project, Kind: KindMR, IID: 479, Title: "Fix race in courier slot allocator"}

func newFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: project})
	c.AddIssue(path, gitlab.Issue{IID: 1318, Title: issueTarget.Title})
	c.AddMR(path, gitlab.MergeRequest{IID: 479, Title: mrTarget.Title})
	return c
}

type setup struct {
	d    *uitest.Driver
	env  *app.Env
	c    *fake.Client
	root *app.Model
}

func start(t *testing.T, c *fake.Client, lang i18n.Lang) *setup {
	t.Helper()
	env := app.NewEnv(lang)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	root := app.New(env, host{}, app.Options{})
	return &setup{d: uitest.New(t, root), env: env, c: c, root: root}
}

func (s *setup) open(t Target) {
	s.d.Run(Open(s.env, t))
}

func (s *setup) text(str string) {
	for _, r := range str {
		s.d.Key(string(r))
	}
}

func sent(d *uitest.Driver) *SentMsg {
	for _, m := range d.Msgs {
		if s, ok := m.(SentMsg); ok {
			return &s
		}
	}
	return nil
}

func TestSendIssueCommentViaButton(t *testing.T) {
	s := start(t, newFake(), i18n.EN)
	s.open(issueTarget)
	s.d.RequireView("Comment · #1318 Surge coefficient ignores zone overrides",
		"markdown · tab — to the button · alt+enter — send", "esc close", "enter Send")
	s.text("Беру")
	s.d.Key("tab", "enter")
	calls := s.c.CallsOf("Issues.Comment")
	if len(calls) != 1 || calls[0][1] != int64(1318) || calls[0][2] != "Беру" {
		t.Fatalf("Comment calls = %v", calls)
	}
	m := sent(s.d)
	if m == nil || m.Target != issueTarget || m.Note.Body != "Беру" {
		t.Fatalf("SentMsg = %+v", m)
	}
	s.d.RequireView("Comment added")
	s.d.RequireNoView("Comment · #1318")
}

func TestEnterInFieldAddsLine(t *testing.T) {
	s := start(t, newFake(), i18n.EN)
	s.open(issueTarget)
	s.text("строка")
	s.d.Key("enter")
	s.text("вторая")
	if s.c.CallCount("Issues.Comment") != 0 {
		t.Fatal("enter in the field must not send")
	}
	f := s.root.Overlay().(*Form)
	if f.Text() != "строка\nвторая" {
		t.Fatalf("text = %q", f.Text())
	}
}

func TestAltEnterSends(t *testing.T) {
	s := start(t, newFake(), i18n.EN)
	s.open(issueTarget)
	s.text("Беру")
	s.d.Key("alt+enter")
	if calls := s.c.CallsOf("Issues.Comment"); len(calls) != 1 || calls[0][2] != "Беру" {
		t.Fatalf("Comment calls = %v", calls)
	}
}

func TestTrailingBlankLinesAreNotSent(t *testing.T) {
	s := start(t, newFake(), i18n.EN)
	s.open(issueTarget)
	s.text("Беру")
	s.d.Key("enter", "enter")
	s.d.Key("alt+enter")
	if calls := s.c.CallsOf("Issues.Comment"); len(calls) != 1 || calls[0][2] != "Беру" {
		t.Fatalf("Comment calls = %v", calls)
	}
}

func TestEmptyCommentIsNotSent(t *testing.T) {
	s := start(t, newFake(), i18n.EN)
	s.open(issueTarget)
	s.d.Key("space", "enter", "space")
	s.d.Key("tab", "enter")
	s.d.RequireView("comment is empty")
	if s.c.CallCount("Issues.Comment") != 0 {
		t.Fatal("an empty comment must not be sent")
	}
	s.d.Key("tab")
	s.text("x")
	s.d.RequireNoView("comment is empty")
}

func TestDraftSurvivesEsc(t *testing.T) {
	s := start(t, newFake(), i18n.EN)
	s.open(issueTarget)
	s.text("Беру")
	s.d.Key("esc")
	s.d.RequireNoView("Comment · #1318")
	if s.c.CallCount("Issues.Comment") != 0 {
		t.Fatal("esc must not send")
	}
	s.open(issueTarget)
	if f := s.root.Overlay().(*Form); f.Text() != "Беру" {
		t.Fatalf("draft = %q", f.Text())
	}
	// another object has no draft
	s.d.Key("esc")
	s.open(mrTarget)
	if f := s.root.Overlay().(*Form); f.Text() != "" {
		t.Fatalf("MR draft = %q", f.Text())
	}
}

func TestSendClearsDraft(t *testing.T) {
	s := start(t, newFake(), i18n.EN)
	s.open(issueTarget)
	s.text("Беру")
	s.d.Key("esc")
	s.open(issueTarget)
	s.d.Key("alt+enter")
	s.open(issueTarget)
	if f := s.root.Overlay().(*Form); f.Text() != "" {
		t.Fatalf("draft after a successful send = %q", f.Text())
	}
}

func TestErrorKeepsTextAndUnlocks(t *testing.T) {
	c := newFake()
	c.FailNext("Issues.Comment", &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403})
	s := start(t, c, i18n.EN)
	s.open(issueTarget)
	s.text("Беру")
	s.d.Key("alt+enter")
	s.d.RequireView("Comment · #1318", "Not enough permissions")
	if f := s.root.Overlay().(*Form); f.Text() != "Беру" || f.sending {
		t.Fatalf("text = %q sending = %v", f.Text(), f.sending)
	}
	// the form is unlocked: a second try goes through
	s.d.Key("alt+enter")
	if s.c.CallCount("Issues.Comment") != 2 {
		t.Fatal("the second send must go through")
	}
	s.d.RequireNoView("Comment · #1318")
}

func TestSendingLocksTheForm(t *testing.T) {
	c := newFake()
	c.SetDelay("Issues.Comment", 300*time.Millisecond)
	s := start(t, c, i18n.EN)
	s.open(issueTarget)
	s.text("Беру")
	s.d.Key("alt+enter")
	s.d.RequireView("sending…")
	s.d.Key("esc", "x", "alt+enter")
	s.d.RequireView("Comment · #1318", "sending…")
	f := s.root.Overlay().(*Form)
	if f.Text() != "Беру" {
		t.Fatalf("a locked form takes no input: %q", f.Text())
	}
	if n := c.CallCount("Issues.Comment"); n != 1 {
		t.Fatalf("calls = %d", n)
	}
}

func TestAnswerOfClosedFormDoesNotReachNewForm(t *testing.T) {
	c := newFake()
	c.SetDelay("Issues.Comment", 300*time.Millisecond)
	s := start(t, c, i18n.EN)
	s.open(issueTarget)
	s.text("Беру")
	s.d.Key("alt+enter")
	old := s.root.Overlay().(*Form)

	// The old form goes away (for instance the app replaced the overlay) and
	// a new one for the same issue opens with its own text.
	s.d.Send(app.CloseOverlayMsg{})
	s.open(issueTarget)
	nf := s.root.Overlay().(*Form)
	s.text("новый")
	s.d.Send(old.Answer(gitlab.Note{ID: 5, Body: "Беру"}, nil))
	if s.root.Overlay() != app.Overlay(nf) || nf.Text() != "новый" {
		t.Fatalf("the new form must stay as it was: %q", nf.Text())
	}
	s.d.RequireView("Comment · #1318")

	// The comment exists though: the old form still reports it.
	_, cmd := old.Update(old.Answer(gitlab.Note{ID: 5, Body: "Беру"}, nil))
	s.d.Run(cmd)
	if m := sent(s.d); m == nil || m.Note.ID != 5 {
		t.Fatalf("SentMsg = %+v", m)
	}
	s.d.RequireView("Comment added")
}

func TestMRComment(t *testing.T) {
	s := start(t, newFake(), i18n.RU)
	s.open(mrTarget)
	s.d.RequireView("Комментарий · !479 Fix race in courier slot allocator", "markdown · tab — к кнопке · alt+enter — отправить", "esc закрыть", "enter Отправить")
	s.text("LGTM")
	s.d.Key("alt+enter")
	if calls := s.c.CallsOf("MergeRequests.Comment"); len(calls) != 1 || calls[0][1] != int64(479) || calls[0][2] != "LGTM" {
		t.Fatalf("calls = %v", calls)
	}
	s.d.RequireView("Комментарий добавлен")
	if m := sent(s.d); m == nil || m.Target.Kind != KindMR {
		t.Fatalf("SentMsg = %+v", m)
	}
}

func TestGoldenForm(t *testing.T) {
	s := start(t, newFake(), i18n.EN)
	s.open(issueTarget)
	s.text("Беру в работу")
	s.d.Key("enter")
	s.text("Посмотрю сегодня")
	s.d.RequireGoldenSizes("comment")
}

func TestGoldenFormButtonRU(t *testing.T) {
	s := start(t, newFake(), i18n.RU)
	s.open(mrTarget)
	s.text("LGTM")
	s.d.Key("tab")
	s.d.RequireGolden("comment-button-ru")
}
