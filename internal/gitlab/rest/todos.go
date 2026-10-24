package rest

import (
	"context"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
)

type todos struct{ c *Client }

func (c *Client) Todos() gitlab.TodoService { return todos{c} }

// List returns one page of the current user's to-dos in the server's order
// (newest first).
func (s todos) List(ctx context.Context, f gitlab.TodoFilter, cursor string, limit int) (gitlab.Page[gitlab.Todo], error) {
	if limit <= 0 {
		limit = refListSize
	}
	state := f.State
	if state == "" {
		state = "pending"
	}
	opt := &gl.ListTodosOptions{
		ListOptions: gl.ListOptions{PerPage: int64(limit)},
		State:       gl.Ptr(state),
	}
	if f.Type != "" {
		opt.Type = gl.Ptr(f.Type)
	}
	if n, err := strconv.ParseInt(cursor, 10, 64); err == nil {
		opt.Page = n
	}
	list, resp, err := s.c.api.Todos.ListTodos(opt, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Page[gitlab.Todo]{}, mapError(err)
	}
	page := gitlab.Page[gitlab.Todo]{Total: totalOf(resp)}
	if resp.NextPage > 0 {
		page.NextCursor = strconv.FormatInt(resp.NextPage, 10)
	}
	for _, t := range list {
		page.Items = append(page.Items, mapTodo(t))
	}
	return page, nil
}

// Pending asks for one pending to-do and reads X-Total.
func (s todos) Pending(ctx context.Context) (int, error) {
	opt := &gl.ListTodosOptions{ListOptions: gl.ListOptions{PerPage: 1}, State: gl.Ptr("pending")}
	_, resp, err := s.c.api.Todos.ListTodos(opt, gl.WithContext(ctx))
	if err != nil {
		return 0, mapError(err)
	}
	return totalOf(resp), nil
}

func (s todos) MarkDone(ctx context.Context, id int64) error {
	_, err := s.c.api.Todos.MarkTodoAsDone(id, gl.WithContext(ctx))
	return mapError(err)
}

func (s todos) MarkAllDone(ctx context.Context) error {
	_, err := s.c.api.Todos.MarkAllTodosAsDone(gl.WithContext(ctx))
	return mapError(err)
}

func mapTodo(t *gl.Todo) gitlab.Todo {
	out := gitlab.Todo{
		ID: t.ID, Action: string(t.ActionName), TargetType: string(t.TargetType),
		TargetURL: t.TargetURL, Body: t.Body, State: t.State, CreatedAt: deref(t.CreatedAt),
	}
	if a := t.Author; a != nil {
		out.Author = gitlab.User{ID: a.ID, Username: a.Username, Name: a.Name, WebURL: a.WebURL}
	}
	if t.Project != nil {
		out.Project = &gitlab.ProjectRef{ID: t.Project.ID, FullPath: t.Project.PathWithNamespace}
	}
	if tg := t.Target; tg != nil {
		out.TargetIID = tg.IID
		out.TargetTitle = tg.Title
		out.TargetState = tg.State
		// A commit has no IID; its id is the full SHA.
		if sha, ok := tg.ID.(string); ok && out.TargetType == "Commit" {
			out.TargetRef = sha[:min(len(sha), 8)]
		}
	}
	return out
}
