package hybrid

import "github.com/4aykovski/glview/internal/gitlab"

// Todos are REST only: the list, the counter and the mutations.
func (c *Client) Todos() gitlab.TodoService { return c.rest.Todos() }
