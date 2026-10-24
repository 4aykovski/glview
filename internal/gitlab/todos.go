package gitlab

import (
	"context"
	"time"
)

// Todo is a to-do item of the current user (REST /todos, SPEC §4).
type Todo struct {
	ID          int64
	Project     *ProjectRef // nil — a to-do without a project (group epic)
	Author      User
	Action      string // action_name as is: review_requested, mentioned, ...
	TargetType  string // MergeRequest | Issue | Commit | Epic | ..., as is
	TargetIID   int64  // 0 — the target has no IID (commit)
	TargetRef   string // commit: short SHA; otherwise ""
	TargetTitle string
	TargetState string // opened | merged | closed ..., as is
	TargetURL   string
	Body        string
	State       string // pending | done
	CreatedAt   time.Time
}

// TodoFilter selects to-dos; the zero value means pending of any type.
type TodoFilter struct {
	State string // pending | done
	Type  string // "" | MergeRequest | Issue
}

// TodoService lists and resolves the current user's to-dos (REST).
type TodoService interface {
	List(ctx context.Context, f TodoFilter, cursor string, limit int) (Page[Todo], error)
	// Pending is the number of pending to-dos (X-Total; -1 if the header is
	// missing).
	Pending(ctx context.Context) (int, error)
	MarkDone(ctx context.Context, id int64) error
	MarkAllDone(ctx context.Context) error
}
