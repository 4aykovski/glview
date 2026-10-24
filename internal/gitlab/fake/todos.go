package fake

import (
	"context"
	"slices"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

// AddTodo registers a to-do; the list serves them in the order added (the
// server's order: newest first). A zero State means pending.
func (c *Client) AddTodo(t gitlab.Todo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.State == "" {
		t.State = "pending"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	c.todos = append(c.todos, t)
}

// SetTodos replaces all registered to-dos.
func (c *Client) SetTodos(ts ...gitlab.Todo) {
	c.mu.Lock()
	c.todos = nil
	c.mu.Unlock()
	for _, t := range ts {
		c.AddTodo(t)
	}
}

// SetTodoTotalUnknown makes Pending and List report no total, like GitLab
// does for more than 10 000 to-dos.
func (c *Client) SetTodoTotalUnknown(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.todoNoTotal = v
}

// TodoState returns the state of a registered to-do ("" if unknown).
func (c *Client) TodoState(id int64) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.todos {
		if t.ID == id {
			return t.State
		}
	}
	return ""
}

func (c *Client) Todos() gitlab.TodoService { return todoService{c} }

type todoService struct{ c *Client }

func (s todoService) List(ctx context.Context, f gitlab.TodoFilter, cursor string, limit int) (gitlab.Page[gitlab.Todo], error) {
	if err := s.c.enter(ctx, "Todos.List", f, cursor, limit); err != nil {
		return gitlab.Page[gitlab.Todo]{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	state := f.State
	if state == "" {
		state = "pending"
	}
	var all []gitlab.Todo
	for _, t := range s.c.todos {
		if t.State == state && (f.Type == "" || t.TargetType == f.Type) {
			all = append(all, t)
		}
	}
	page := paginate(all, cursor, limit)
	if s.c.todoNoTotal {
		page.Total = -1
	}
	return page, nil
}

func (s todoService) Pending(ctx context.Context) (int, error) {
	if err := s.c.enter(ctx, "Todos.Pending"); err != nil {
		return 0, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.todoNoTotal {
		return -1, nil
	}
	n := 0
	for _, t := range s.c.todos {
		if t.State == "pending" {
			n++
		}
	}
	return n, nil
}

func (s todoService) MarkDone(ctx context.Context, id int64) error {
	if err := s.c.enter(ctx, "Todos.MarkDone", id); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	i := slices.IndexFunc(s.c.todos, func(t gitlab.Todo) bool { return t.ID == id })
	if i < 0 {
		return notFound("to-do")
	}
	s.c.todos[i].State = "done"
	return nil
}

func (s todoService) MarkAllDone(ctx context.Context) error {
	if err := s.c.enter(ctx, "Todos.MarkAllDone"); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	for i := range s.c.todos {
		s.c.todos[i].State = "done"
	}
	return nil
}
