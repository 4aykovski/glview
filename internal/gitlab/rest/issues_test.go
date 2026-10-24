package rest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

const issuesBase = "/api/v4/projects/platform%2Froute-service/issues"

var issueProject = gitlab.ProjectRef{FullPath: "platform/route-service"}

const issueListJSON = `[
 {"id":9001,"iid":1318,"title":"Surge coefficient ignores zone overrides","description":"Steps","state":"opened",
  "author":{"id":3,"username":"anna.k","name":"Anna K"},
  "assignees":[{"id":7,"username":"egor","name":"Egor S"}],
  "labels":["bug","pricing"],"milestone":{"title":"2026-Q4"},
  "user_notes_count":2,"merge_requests_count":1,"web_url":"https://gitlab.example.com/platform/route-service/-/issues/1318",
  "created_at":"2026-10-01T10:00:00Z","updated_at":"2026-10-22T10:00:00Z"},
 {"id":9002,"iid":1312,"title":"Duplicate slot assignment under load","state":"opened","labels":[],"merge_requests_count":0}]`

func TestIssuesListFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter gitlab.IssueFilter
		want   url.Values
		absent []string
	}{
		{"defaults", gitlab.IssueFilter{}, url.Values{"state": {"opened"}, "order_by": {"updated_at"}, "sort": {"desc"}, "per_page": {"20"}}, []string{"scope", "assignee_id", "milestone", "labels", "search"}},
		{"all drops state", gitlab.IssueFilter{State: "all"}, url.Values{"order_by": {"updated_at"}}, []string{"state"}},
		{"closed", gitlab.IssueFilter{State: "closed"}, url.Values{"state": {"closed"}}, nil},
		{"me", gitlab.IssueFilter{Assignee: "@me"}, url.Values{"scope": {"assigned_to_me"}}, []string{"assignee_username", "assignee_id"}},
		{"unassigned", gitlab.IssueFilter{Assignee: "none"}, url.Values{"assignee_id": {"None"}}, nil},
		{"username", gitlab.IssueFilter{Assignee: "anna.k"}, url.Values{"assignee_username": {"anna.k"}}, nil},
		{"no milestone", gitlab.IssueFilter{Milestone: "none"}, url.Values{"milestone": {"None"}}, nil},
		{"milestone", gitlab.IssueFilter{Milestone: "2026-Q4"}, url.Values{"milestone": {"2026-Q4"}}, nil},
		{"labels and search", gitlab.IssueFilter{Labels: []string{"bug", "pricing"}, Search: "surge"}, url.Values{"labels": {"bug,pricing"}, "search": {"surge"}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got url.Values
			c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
				"GET " + issuesBase: func(w http.ResponseWriter, r *http.Request) {
					got = r.URL.Query()
					w.Header().Set("X-Total", "12")
					w.Header().Set("X-Next-Page", "2")
					jsonReply(200, issueListJSON)(w, r)
				},
			})
			page, err := c.Issues().List(context.Background(), issueProject, tt.filter, "", 20)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tt.want {
				if got.Get(k) != v[0] {
					t.Errorf("%s = %q, want %q (query %v)", k, got.Get(k), v[0], got)
				}
			}
			for _, k := range tt.absent {
				if got.Has(k) {
					t.Errorf("%s must not be sent: %v", k, got)
				}
			}
			if page.Total != 12 || page.NextCursor != "2" || len(page.Items) != 2 {
				t.Fatalf("page = %+v", page)
			}
		})
	}
}

func TestIssuesListMapping(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET " + issuesBase: func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonReply(200, issueListJSON)(w, r)
		},
	})
	page, err := c.Issues().List(context.Background(), issueProject, gitlab.IssueFilter{}, "3", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "page=3") {
		t.Fatalf("query = %s", query)
	}
	i := page.Items[0]
	if i.IID != 1318 || i.Author.Username != "anna.k" || len(i.Assignees) != 1 || i.Assignees[0].Username != "egor" ||
		i.Milestone != "2026-Q4" || i.UserNotesCount != 2 || i.MergeRequestsCount != 1 || len(i.Labels) != 2 || i.Project != issueProject {
		t.Fatalf("issue = %+v", i)
	}
	if page.Items[1].Milestone != "" || page.Items[1].Author.Username != "" {
		t.Fatalf("issue 2 = %+v", page.Items[1])
	}
}

func TestIssuesNotesOldestFirst(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET " + issuesBase + "/1318/notes": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonReply(200, `[{"id":12,"body":"newest","author":{"username":"egor"},"created_at":"2026-10-22T10:00:00Z"},
			 {"id":11,"body":"older","system":true,"author":{"username":"anna.k"},"created_at":"2026-10-21T10:00:00Z"}]`)(w, r)
		},
	})
	notes, err := c.Issues().Notes(context.Background(), issueProject, 1318, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sort=desc", "per_page=50"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query %q lacks %s", query, want)
		}
	}
	if len(notes) != 2 || notes[0].ID != 11 || !notes[0].System || notes[1].ID != 12 || notes[1].Author.Username != "egor" {
		t.Fatalf("notes = %+v", notes)
	}
}

func TestIssuesDiscussionsAllPages(t *testing.T) {
	var pages []string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET " + issuesBase + "/1318/discussions": func(w http.ResponseWriter, r *http.Request) {
			pages = append(pages, r.URL.Query().Get("page"))
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("X-Next-Page", "2")
				jsonReply(200, `[{"id":"d1","notes":[{"id":1,"body":"one","author":{"username":"anna.k"}}]}]`)(w, r)
				return
			}
			jsonReply(200, `[{"id":"d2","notes":[{"id":2,"body":"two","author":{"username":"egor"}}]}]`)(w, r)
		},
	})
	ds, err := c.Issues().Discussions(context.Background(), issueProject, 1318)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 2 || ds[0].ID != "d1" || ds[1].Notes[0].Body != "two" || len(pages) != 2 {
		t.Fatalf("ds = %+v, pages = %v", ds, pages)
	}
}

func TestIssuesRelatedMRs(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET " + issuesBase + "/1318/related_merge_requests": jsonReply(200, `[{"iid":479,"title":"Fix slot race","state":"opened","web_url":"https://gitlab.example.com/platform/route-service/-/merge_requests/479"}]`),
	})
	rel, err := c.Issues().RelatedMRs(context.Background(), issueProject, 1318)
	if err != nil {
		t.Fatal(err)
	}
	if len(rel) != 1 || rel[0].IID != 479 || rel[0].State != gitlab.MRStateOpened || rel[0].Title != "Fix slot race" {
		t.Fatalf("rel = %+v", rel)
	}
}

func TestIssuesCommentAndState(t *testing.T) {
	var noteBody, putBody string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST " + issuesBase + "/1318/notes": func(w http.ResponseWriter, r *http.Request) {
			noteBody = readBody(r)
			jsonReply(201, `{"id":99,"body":"Беру","author":{"username":"egor"}}`)(w, r)
		},
		"PUT " + issuesBase + "/1318": func(w http.ResponseWriter, r *http.Request) {
			putBody = readBody(r)
			jsonReply(200, `{"id":9001,"iid":1318,"state":"closed"}`)(w, r)
		},
	})
	n, err := c.Issues().Comment(context.Background(), issueProject, 1318, "Беру")
	if err != nil || n.ID != 99 || n.Body != "Беру" {
		t.Fatalf("note = %+v, err = %v", n, err)
	}
	if !strings.Contains(noteBody, `"body":"Беру"`) {
		t.Fatalf("body = %s", noteBody)
	}
	i, err := c.Issues().SetState(context.Background(), issueProject, 1318, true)
	if err != nil || i.State != "closed" || !strings.Contains(putBody, `"state_event":"close"`) {
		t.Fatalf("issue = %+v, err = %v, body = %s", i, err, putBody)
	}
	if _, err := c.Issues().SetState(context.Background(), issueProject, 1318, false); err != nil || !strings.Contains(putBody, `"state_event":"reopen"`) {
		t.Fatalf("err = %v, body = %s", err, putBody)
	}
}

func TestIssuesForbidden(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST " + issuesBase + "/1318/notes": jsonReply(403, `{"message":"403 Forbidden"}`),
	})
	_, err := c.Issues().Comment(context.Background(), issueProject, 1318, "x")
	if !errors.Is(err, gitlab.ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
}

func TestMRComment(t *testing.T) {
	var body string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/platform%2Froute-service/merge_requests/479/notes": func(w http.ResponseWriter, r *http.Request) {
			body = readBody(r)
			jsonReply(201, `{"id":5,"body":"LGTM","author":{"username":"egor"}}`)(w, r)
		},
	})
	n, err := c.MergeRequests().Comment(context.Background(), issueProject, 479, "LGTM")
	if err != nil || n.ID != 5 || !strings.Contains(body, `"body":"LGTM"`) {
		t.Fatalf("note = %+v, err = %v, body = %s", n, err, body)
	}
}

func TestProjectMilestones(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/platform%2Froute-service/milestones": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonReply(200, `[{"title":"2026-Q4"},{"title":"2026-Q3"}]`)(w, r)
		},
	})
	ms, err := c.Projects().Milestones(context.Background(), issueProject, "2026")
	if err != nil || len(ms) != 2 || ms[0] != "2026-Q4" {
		t.Fatalf("ms = %v, err = %v", ms, err)
	}
	for _, want := range []string{"state=active", "search=2026", "per_page=20"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query %q lacks %s", query, want)
		}
	}
}

func readBody(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

// X-Total: 0 is a real zero; no header means unknown.
func TestListTotalFollowsTheHeader(t *testing.T) {
	for _, tt := range []struct {
		name, header string
		want         int
	}{{"zero", "0", 0}, {"seven", "7", 7}, {"absent", "", -1}} {
		t.Run(tt.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				if tt.header != "" {
					w.Header().Set("X-Total", tt.header)
				}
				jsonReply(200, "[]")(w, r)
			}
			c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
				"GET " + issuesBase: handler,
				"GET /api/v4/projects/platform%2Froute-service/repository/tags": handler,
			})
			is, err := c.Issues().List(context.Background(), issueProject, gitlab.IssueFilter{}, "", 20)
			if err != nil || is.Total != tt.want {
				t.Fatalf("issues: total = %d err = %v, want %d", is.Total, err, tt.want)
			}
			tags, err := c.Tags().List(context.Background(), issueProject, "", "", 20)
			if err != nil || tags.Total != tt.want {
				t.Fatalf("tags: total = %d err = %v, want %d", tags.Total, err, tt.want)
			}
		})
	}
}
