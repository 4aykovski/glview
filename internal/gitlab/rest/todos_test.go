package rest

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

// One response with targets of different shapes: MR, issue, commit, epic
// without a project.
const todoListJSON = `[
 {"id":101,"project":{"id":5,"name":"route-service","path_with_namespace":"platform/route-service"},
  "author":{"id":3,"username":"anna.k","name":"Anna K"},"action_name":"review_requested","target_type":"MergeRequest",
  "target":{"id":8001,"iid":479,"project_id":5,"title":"Slot allocator rewrite","state":"opened","assignees":[{"id":7,"username":"egor"}],
            "labels":["backend"],"milestone":{"id":2,"title":"2026-Q4"},"sha":"abc","squash":false,"web_url":"https://gitlab.example.com/platform/route-service/-/merge_requests/479"},
  "target_url":"https://gitlab.example.com/platform/route-service/-/merge_requests/479","body":"Slot allocator rewrite","state":"pending","created_at":"2026-10-24T08:00:00Z"},
 {"id":102,"project":{"id":5,"path_with_namespace":"platform/route-service"},
  "author":{"id":4,"username":"d.orlov","name":"D Orlov"},"action_name":"mentioned","target_type":"Issue",
  "target":{"id":9001,"iid":1318,"title":"Surge coefficient ignores zone overrides","state":"opened","confidential":false,"time_stats":{"time_estimate":0}},
  "target_url":"https://gitlab.example.com/platform/route-service/-/issues/1318","body":"@egor look, please","state":"pending","created_at":"2026-10-24T07:00:00Z"},
 {"id":103,"project":{"id":5,"path_with_namespace":"platform/route-service"},
  "author":{"id":3,"username":"anna.k"},"action_name":"build_failed","target_type":"Commit",
  "target":{"id":"0123456789abcdef0123456789abcdef01234567","short_id":"01234567","title":"Fix slot overflow","author_name":"Anna K"},
  "target_url":"https://gitlab.example.com/platform/route-service/-/commit/0123456789abcdef","body":"Fix slot overflow","state":"pending","created_at":"2026-10-23T08:00:00Z"},
 {"id":104,"project":null,"author":{"id":4,"username":"d.orlov"},"action_name":"okr_checkin_requested","target_type":"Epic",
  "target":{"id":77,"iid":12,"title":"Q4 planning","state":"opened"},
  "target_url":"https://gitlab.example.com/groups/platform/-/epics/12","body":"Q4 planning","state":"pending","created_at":"2026-10-22T08:00:00Z"}]`

func TestTodosList(t *testing.T) {
	var got url.Values
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/todos": func(w http.ResponseWriter, r *http.Request) {
			got = r.URL.Query()
			w.Header().Set("X-Total", "12")
			w.Header().Set("X-Next-Page", "2")
			jsonReply(200, todoListJSON)(w, r)
		},
	})
	page, err := c.Todos().List(context.Background(), gitlab.TodoFilter{State: "done", Type: "MergeRequest"}, "3", 20)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"state": "done", "type": "MergeRequest", "page": "3", "per_page": "20"} {
		if got.Get(k) != v {
			t.Errorf("%s = %q, want %q (query %v)", k, got.Get(k), v, got)
		}
	}
	if page.Total != 12 || page.NextCursor != "2" || len(page.Items) != 4 {
		t.Fatalf("page = %+v", page)
	}
	mr, issue, commit, epic := page.Items[0], page.Items[1], page.Items[2], page.Items[3]
	if mr.ID != 101 || mr.Project == nil || mr.Project.FullPath != "platform/route-service" || mr.Project.ID != 5 ||
		mr.Author.Username != "anna.k" || mr.Action != "review_requested" || mr.TargetType != "MergeRequest" ||
		mr.TargetIID != 479 || mr.TargetTitle != "Slot allocator rewrite" || mr.TargetState != "opened" ||
		mr.State != "pending" || !mr.CreatedAt.Equal(time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("mr = %+v", mr)
	}
	if issue.TargetType != "Issue" || issue.TargetIID != 1318 || issue.Body != "@egor look, please" {
		t.Errorf("issue = %+v", issue)
	}
	if commit.TargetType != "Commit" || commit.TargetIID != 0 || commit.TargetRef != "01234567" || commit.TargetTitle != "Fix slot overflow" {
		t.Errorf("commit = %+v", commit)
	}
	if epic.Project != nil || epic.Action != "okr_checkin_requested" || epic.TargetType != "Epic" {
		t.Errorf("epic = %+v", epic)
	}
}

func TestTodosListDefaults(t *testing.T) {
	var got url.Values
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/todos": func(w http.ResponseWriter, r *http.Request) {
			got = r.URL.Query()
			jsonReply(200, `[]`)(w, r)
		},
	})
	page, err := c.Todos().List(context.Background(), gitlab.TodoFilter{}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("state") != "pending" || got.Has("type") || got.Has("page") || got.Get("per_page") != "20" {
		t.Errorf("query = %v", got)
	}
	if page.Total != -1 || page.NextCursor != "" || len(page.Items) != 0 {
		t.Errorf("page = %+v", page)
	}
}

func TestTodosPending(t *testing.T) {
	var got url.Values
	total := "7"
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/todos": func(w http.ResponseWriter, r *http.Request) {
			got = r.URL.Query()
			if total != "" {
				w.Header().Set("X-Total", total)
			}
			jsonReply(200, `[]`)(w, r)
		},
	})
	n, err := c.Todos().Pending(context.Background())
	if err != nil || n != 7 {
		t.Fatalf("Pending = %d, %v", n, err)
	}
	if got.Get("state") != "pending" || got.Get("per_page") != "1" {
		t.Errorf("query = %v", got)
	}
	total = ""
	if n, err = c.Todos().Pending(context.Background()); err != nil || n != -1 {
		t.Fatalf("without X-Total: %d, %v", n, err)
	}
}

func TestTodosMarkDone(t *testing.T) {
	var calls []string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/todos/101/mark_as_done": func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, r.URL.Path)
			jsonReply(200, `{"id":101,"state":"done"}`)(w, r)
		},
		"POST /api/v4/todos/mark_as_done": func(w http.ResponseWriter, r *http.Request) {
			calls = append(calls, r.URL.Path)
			w.WriteHeader(204)
		},
	})
	if err := c.Todos().MarkDone(context.Background(), 101); err != nil {
		t.Fatal(err)
	}
	if err := c.Todos().MarkAllDone(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "/api/v4/todos/101/mark_as_done" || calls[1] != "/api/v4/todos/mark_as_done" {
		t.Errorf("calls = %v", calls)
	}
}

func TestTodosErrors(t *testing.T) {
	for status, want := range map[int]error{401: gitlab.ErrUnauthorized, 403: gitlab.ErrForbidden} {
		c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
			"/api/v4/todos":                   jsonReply(status, `{"message":"nope"}`),
			"/api/v4/todos/101/mark_as_done":  jsonReply(status, `{"message":"nope"}`),
			"POST /api/v4/todos/mark_as_done": jsonReply(status, `{"message":"nope"}`),
		})
		ctx := context.Background()
		if _, err := c.Todos().List(ctx, gitlab.TodoFilter{}, "", 20); !errors.Is(err, want) {
			t.Errorf("%d List: %v", status, err)
		}
		if _, err := c.Todos().Pending(ctx); !errors.Is(err, want) {
			t.Errorf("%d Pending: %v", status, err)
		}
		if err := c.Todos().MarkDone(ctx, 101); !errors.Is(err, want) {
			t.Errorf("%d MarkDone: %v", status, err)
		}
		if err := c.Todos().MarkAllDone(ctx); !errors.Is(err, want) {
			t.Errorf("%d MarkAllDone: %v", status, err)
		}
	}
}
