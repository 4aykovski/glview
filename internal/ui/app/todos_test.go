package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

// todoStub is a To-Do screen: it marks itself for T and counts activations.
type todoStub struct {
	*dummy
	activated, deactivated int
}

func (s *todoStub) IsTodoScreen() {}

func (s *todoStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg.(type) {
	case ActivatedMsg:
		s.activated++
	case DeactivatedMsg:
		s.deactivated++
	}
	_, cmd := s.dummy.Update(msg)
	return s, cmd
}

// capturing is a screen with a focused input.
type capturing struct{ *dummy }

func (c capturing) CapturesInput() bool { return true }

func (c capturing) Update(msg tea.Msg) (Screen, tea.Cmd) {
	_, cmd := c.dummy.Update(msg)
	return c, cmd
}

// headerScreen draws the real header, so tests see the counter.
type headerScreen struct {
	*dummy
	updated time.Time
}

func (h *headerScreen) View() string {
	return common.Header(h.env.Styles, "ctx", h.env.HeaderRight(h.updated), h.w)
}

func (h *headerScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	_, cmd := h.dummy.Update(msg)
	return h, cmd
}

func todoRoot(t *testing.T, first func(*Env) Screen, todos func(*Env) Screen) (*uitest.Driver, *Model, *fake.Client) {
	t.Helper()
	client := fake.New()
	env := NewEnv(i18n.EN)
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	if todos != nil {
		env.Todos = func() Screen { return todos(env) }
	}
	root := New(env, first(env), Options{})
	return uitest.New(t, root), root, client
}

func plain(env *Env) Screen { return newDummy(env, "A") }

func TestTOpensTodo(t *testing.T) {
	var stub *todoStub
	d, root, _ := todoRoot(t, plain, func(env *Env) Screen {
		stub = &todoStub{dummy: newDummy(env, "todo")}
		return stub
	})
	d.Key("T")
	if root.Depth() != 2 || root.Top() != Screen(stub) {
		t.Fatalf("depth %d, top %T", root.Depth(), root.Top())
	}
	d.Key("esc")
	d.RequireView("screen=A")
}

func TestTIsOffWithoutTodoScreen(t *testing.T) {
	d, root, _ := todoRoot(t, plain, nil)
	d.Key("T")
	if root.Depth() != 1 {
		t.Fatalf("depth = %d", root.Depth())
	}
	for _, b := range root.globalBindings() {
		if b.Help().Key == "T" && b.Visible() {
			t.Fatal("T must be hidden in help without a To-Do screen")
		}
	}
}

func TestTIsInGlobalHelp(t *testing.T) {
	d, _, _ := todoRoot(t, plain, func(env *Env) Screen { return &todoStub{dummy: newDummy(env, "todo")} })
	d.Key("?")
	d.RequireView("T", "To-Do")
}

func TestTReturnsToTodoInTheStack(t *testing.T) {
	var stub *todoStub
	d, root, _ := todoRoot(t, plain, func(env *Env) Screen {
		stub = &todoStub{dummy: newDummy(env, "todo")}
		return stub
	})
	d.Key("T")
	d.Key("n") // a child over To-Do
	d.Key("n")
	if root.Depth() != 4 {
		t.Fatalf("depth = %d", root.Depth())
	}
	before := stub.activated
	d.Key("T")
	if root.Depth() != 2 || root.Top() != Screen(stub) {
		t.Fatalf("depth %d, top %T: To-Do must become the top, the rest closed", root.Depth(), root.Top())
	}
	if stub.activated != before+1 {
		t.Fatalf("To-Do activated %d times, want %d (it refreshes)", stub.activated, before+1)
	}
	// And on the To-Do screen itself, T refreshes it and keeps one copy.
	d.Key("T")
	if root.Depth() != 2 || stub.activated != before+2 {
		t.Fatalf("depth %d, activated %d", root.Depth(), stub.activated)
	}
}

func TestTInInputGoesToTheInput(t *testing.T) {
	d, root, _ := todoRoot(t, func(env *Env) Screen { return capturing{newDummy(env, "form")} },
		func(env *Env) Screen { return &todoStub{dummy: newDummy(env, "todo")} })
	d.Key("T")
	if root.Depth() != 1 {
		t.Fatal("T typed into an input must not open To-Do")
	}
}

func TestTUnderAnOverlayIsIgnored(t *testing.T) {
	d, root, _ := todoRoot(t, plain, func(env *Env) Screen { return &todoStub{dummy: newDummy(env, "todo")} })
	d.Key("x") // a confirm
	if root.Overlay() == nil {
		t.Fatal("no overlay")
	}
	d.Key("T")
	if root.Depth() != 1 || root.Overlay() == nil {
		t.Fatalf("depth %d, overlay %v", root.Depth(), root.Overlay())
	}
}

func counterRoot(t *testing.T, pending int) (*uitest.Driver, *Model, *fake.Client) {
	t.Helper()
	var todos []gitlab.Todo
	for i := range pending {
		todos = append(todos, gitlab.Todo{ID: int64(i + 1), TargetType: "Issue", TargetIID: int64(i + 1)})
	}
	client := fake.New()
	client.SetTodos(todos...)
	env := NewEnv(i18n.EN)
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) }
	env.Todos = func() Screen { return newDummy(env, "todo") }
	first := &headerScreen{dummy: newDummy(env, "A"), updated: env.Now().Add(-5 * time.Minute)}
	root := New(env, first, Options{})
	return uitest.New(t, root), root, client
}

func TestHeaderCounter(t *testing.T) {
	d, root, client := counterRoot(t, 5)
	d.RequireView("@egor · To-Do 5 · ⟳ 5m")
	if client.CallCount("Todos.Pending") != 1 {
		t.Fatalf("Pending calls = %d, want 1 at start", client.CallCount("Todos.Pending"))
	}
	if !root.env.TodoKnown || root.env.TodoCount != 5 {
		t.Fatalf("env counter = %d (known %v)", root.env.TodoCount, root.env.TodoKnown)
	}
}

func TestHeaderCounterZeroAndUnknown(t *testing.T) {
	d, _, _ := counterRoot(t, 0)
	d.RequireView("@egor · ⟳ 5m")
	d.RequireNoView("To-Do")

	// Before the answer the counter is not known: nothing is shown.
	client := fake.New()
	env := NewEnv(i18n.EN)
	env.Client, env.User = client, client.User
	env.Todos = func() Screen { return newDummy(env, "todo") }
	if got := env.HeaderRight(time.Time{}); strings.Contains(got, "To-Do") {
		t.Fatalf("header = %q", got)
	}
}

func TestHeaderCounterNoTotal(t *testing.T) {
	client := fake.New()
	client.SetTodoTotalUnknown(true)
	env := NewEnv(i18n.EN)
	env.Client, env.User = client, client.User
	env.Todos = func() Screen { return newDummy(env, "todo") }
	root := New(env, &headerScreen{dummy: newDummy(env, "A")}, Options{})
	d := uitest.New(t, root)
	d.RequireView("To-Do 9999+")
}

func TestCounterTickAsksAgainOnlyOnline(t *testing.T) {
	d, root, client := counterRoot(t, 5)
	client.AddTodo(gitlab.Todo{ID: 50, TargetType: "Issue", TargetIID: 50})
	d.Send(todoTickMsg{tok: root.todoTick.Current()})
	if client.CallCount("Todos.Pending") != 2 {
		t.Fatalf("Pending calls = %d, want 2 after a tick", client.CallCount("Todos.Pending"))
	}
	d.RequireView("To-Do 6")

	// Offline: no request.
	root.env.Net = gitlab.NewMonitor()
	root.env.Net.Report(&gitlab.Error{Kind: gitlab.ErrNetwork})
	d.Send(todoTickMsg{tok: root.todoTick.Current()})
	if client.CallCount("Todos.Pending") != 2 {
		t.Fatalf("Pending asked offline: %d calls", client.CallCount("Todos.Pending"))
	}
	root.env.Net.Report(nil)

	// Authorization lost: no request.
	root.env.AuthLost = true
	d.Send(todoTickMsg{tok: root.todoTick.Current()})
	if client.CallCount("Todos.Pending") != 2 {
		t.Fatalf("Pending asked after a 401: %d calls", client.CallCount("Todos.Pending"))
	}
}

func TestCounterStaleTickIsIgnored(t *testing.T) {
	d, root, client := counterRoot(t, 5)
	old := root.todoTick.Current()
	d.Send(todoTickMsg{tok: old}) // schedules a new tick: old is stale now
	n := client.CallCount("Todos.Pending")
	d.Send(todoTickMsg{tok: old})
	if client.CallCount("Todos.Pending") != n {
		t.Fatal("a tick of an older generation must be dropped")
	}
}

func TestCounterErrorKeepsTheValueAndShowsNothing(t *testing.T) {
	d, root, client := counterRoot(t, 5)
	client.FailNext("Todos.Pending", &gitlab.Error{Kind: gitlab.ErrServer})
	d.Send(todoTickMsg{tok: root.todoTick.Current()})
	d.RequireView("To-Do 5")
	if root.toast != nil {
		t.Fatalf("the counter error must not be shown: %+v", root.toast)
	}
}

func TestCounterFromTheScreenWinsOverASlowAnswer(t *testing.T) {
	d, root, _ := counterRoot(t, 5)
	stale := todoPendingMsg{tok: root.todoReq.Current(), n: 5}
	d.Send(TodoCountMsg{N: 4})
	d.Send(stale)
	d.RequireView("To-Do 4")
}

func TestCounterRestartsAfterAuthRestored(t *testing.T) {
	d, root, client := counterRoot(t, 5)
	root.env.AuthLost = true
	n := client.CallCount("Todos.Pending")
	d.Send(authCheckedMsg{user: client.User})
	if client.CallCount("Todos.Pending") != n+1 {
		t.Fatal("the counter must be asked again once the token works")
	}
}

func TestNoCounterRequestWithoutTodoScreen(t *testing.T) {
	d, _, client := todoRoot(t, plain, nil)
	_ = d
	if client.CallCount("Todos.Pending") != 0 {
		t.Fatal("the counter is off without the To-Do screen")
	}
}

func TestHeaderDropsTodoCounterFirst(t *testing.T) {
	st := common.NewStyles(common.TerminalTheme(true))
	right := "@egor" + common.Optional(" · To-Do 5") + " · ⟳ 5m"
	long := "a-rather-long-project-context-that-fills-the-line"
	wide := common.Header(st, long, right, 100)
	if !strings.Contains(wide, "To-Do 5") {
		t.Fatalf("fits: %q", wide)
	}
	// Left 'gitlab-tui  ' + 49 = 61, right 'To-Do 5' variant is 25: 86 < 90.
	// At 80 the line is too narrow for all of it: the counter goes.
	narrow := common.Header(st, long, right, 80)
	if strings.Contains(narrow, "To-Do") || !strings.Contains(narrow, "@egor") || !strings.Contains(narrow, "⟳ 5m") || !strings.Contains(narrow, long) {
		t.Fatalf("narrow: %q", narrow)
	}
}
