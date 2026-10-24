package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/ui/common"
)

// todoCountInterval is how often the root asks for the number of pending
// to-dos (SPEC §3.17). It is the one background request that does not come
// from the screen on top of the stack.
const todoCountInterval = 5 * time.Minute

// TodoScreen marks the To-Do screen: T finds it in the stack instead of
// opening a second one (SPEC §2.6).
type TodoScreen interface {
	Screen
	IsTodoScreen()
}

type (
	// TodoCountMsg is sent by the To-Do screen when it learns the number of
	// pending to-dos (N < 0: unknown, over 10 000); the root keeps it in Env
	// for the header.
	TodoCountMsg struct{ N int }

	todoTickMsg    struct{ tok common.Token }
	todoPendingMsg struct {
		tok common.Token
		n   int
		err error
	}
)

// TodoCount puts the number of pending to-dos on the wire to the root.
func TodoCount(n int) tea.Cmd { return msgCmd(TodoCountMsg{N: n}) }

// startTodoCount runs the counter once per session, as soon as there is a
// client and the To-Do screen is wired.
func (m *Model) startTodoCount() tea.Cmd {
	if m.todoStarted || m.env.Client == nil || m.env.Todos == nil {
		return nil
	}
	m.todoStarted = true
	return tea.Batch(m.fetchTodoCount(), m.scheduleTodoTick())
}

func (m *Model) scheduleTodoTick() tea.Cmd {
	tok := m.todoTick.Next()
	return tea.Tick(todoCountInterval, func(time.Time) tea.Msg { return todoTickMsg{tok: tok} })
}

// fetchTodoCount asks for the number; offline and after a 401 it does not.
func (m *Model) fetchTodoCount() tea.Cmd {
	if m.env.Client == nil || m.env.AuthLost || m.env.Net.Offline() {
		return nil
	}
	client, tok := m.env.Client, m.todoReq.Next()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		n, err := client.Todos().Pending(ctx)
		return todoPendingMsg{tok: tok, n: n, err: err}
	}
}

func (m *Model) onTodoTick(msg todoTickMsg) tea.Cmd {
	if !m.todoTick.Valid(msg.tok) {
		return nil
	}
	return tea.Batch(m.scheduleTodoTick(), m.fetchTodoCount())
}

func (m *Model) onTodoPending(msg todoPendingMsg) {
	if !m.todoReq.Valid(msg.tok) {
		return // a newer count (from the screen) is already in
	}
	if msg.err != nil {
		// The counter is a nicety: the old value stays, no toast.
		debuglog.L().Debug("to-do counter failed", "err", msg.err)
		return
	}
	m.env.TodoCount, m.env.TodoKnown = msg.n, true
}

func (m *Model) onTodoCount(msg TodoCountMsg) {
	m.todoReq.Next() // an answer still on the way is older than this one
	m.env.TodoCount, m.env.TodoKnown = msg.N, true
}

// openTodos handles T: the To-Do screen already in the stack becomes the
// top (everything above it is closed) and refreshes, otherwise a new one
// is pushed.
func (m *Model) openTodos() tea.Cmd {
	for i := len(m.stack) - 1; i >= 0; i-- {
		if _, ok := m.stack[i].(TodoScreen); !ok {
			continue
		}
		var cmds []tea.Cmd
		for j := len(m.stack) - 1; j > i; j-- {
			_, cmd := m.stack[j].Update(DeactivatedMsg{})
			cmds = append(cmds, cmd)
		}
		if len(m.stack) == i+1 {
			cmds = append(cmds, m.sendTop(DeactivatedMsg{}))
		}
		m.stack = m.stack[:i+1]
		m.registerWorkspace()
		return tea.Batch(append(cmds, m.sendTop(ActivatedMsg{}))...)
	}
	return m.push(m.env.Todos())
}
