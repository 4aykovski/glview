package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestTodos(t *testing.T) {
	c := New()
	ctx := context.Background()
	c.SetTodos(
		gitlab.Todo{ID: 1, TargetType: "MergeRequest", TargetIID: 479},
		gitlab.Todo{ID: 2, TargetType: "Issue", TargetIID: 1318},
		gitlab.Todo{ID: 3, TargetType: "Issue", TargetIID: 1312, State: "done"},
	)
	if n, err := c.Todos().Pending(ctx); err != nil || n != 2 {
		t.Fatalf("Pending = %d, %v", n, err)
	}
	p, _ := c.Todos().List(ctx, gitlab.TodoFilter{State: "pending", Type: "Issue"}, "", 20)
	if len(p.Items) != 1 || p.Items[0].ID != 2 || p.Total != 1 {
		t.Fatalf("pending issues = %+v", p)
	}
	p, _ = c.Todos().List(ctx, gitlab.TodoFilter{State: "done"}, "", 20)
	if len(p.Items) != 1 || p.Items[0].ID != 3 {
		t.Fatalf("done = %+v", p)
	}
	if err := c.Todos().MarkDone(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.Todos().Pending(ctx); n != 1 {
		t.Fatalf("Pending after MarkDone = %d", n)
	}
	if err := c.Todos().MarkDone(ctx, 99); !errors.Is(err, gitlab.ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	if err := c.Todos().MarkAllDone(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.Todos().Pending(ctx); n != 0 {
		t.Fatalf("Pending after MarkAllDone = %d", n)
	}
	c.SetTodoTotalUnknown(true)
	if n, _ := c.Todos().Pending(ctx); n != -1 {
		t.Fatalf("unknown total: %d", n)
	}
	c.FailNext("Todos.MarkDone", &gitlab.Error{Kind: gitlab.ErrNetwork})
	if err := c.Todos().MarkDone(ctx, 2); err == nil {
		t.Fatal("injected error lost")
	}
	if got := c.CallsOf("Todos.MarkDone"); len(got) != 3 {
		t.Fatalf("journal = %v", got)
	}
}
