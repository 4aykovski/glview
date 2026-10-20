package rest

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestStarred(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonReply(200, `[{"id":42,"path_with_namespace":"platform/route-service","name":"route-service","last_activity_at":"2026-10-19T10:00:00Z"}]`)(w, r)
		},
	})
	ps, err := c.Projects().Starred(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].ID != 42 || ps[0].FullPath != "platform/route-service" || !ps[0].Starred || ps[0].LastActivityAt.IsZero() {
		t.Fatalf("ps = %+v", ps)
	}
	for _, want := range []string{"starred=true", "simple=true", "per_page=100"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query %q lacks %s", query, want)
		}
	}
}

func TestListGroupProjects(t *testing.T) {
	var queries []string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/groups/9/projects": func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.RawQuery)
			page := r.URL.Query().Get("page")
			var items []string
			for i := range 100 {
				items = append(items, fmt.Sprintf(`{"id":%s%03d,"path_with_namespace":"platform/p%s%03d"}`, page, i, page, i))
			}
			w.Header().Set("X-Total", "342")
			w.Header().Set("X-Next-Page", "2")
			jsonReply(200, "["+strings.Join(items, ",")+"]")(w, r)
		},
	})
	ps, total, err := c.Projects().ListGroupProjects(context.Background(), 9, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 100 || total != 342 {
		t.Fatalf("len = %d total = %d", len(ps), total)
	}
	if len(queries) != 1 {
		t.Fatalf("requests = %d, the limit fits one page", len(queries))
	}
	for _, want := range []string{"include_subgroups=true", "archived=false", "order_by=last_activity_at", "per_page=100", "simple=true"} {
		if !strings.Contains(queries[0], want) {
			t.Fatalf("query %q lacks %s", queries[0], want)
		}
	}
	ps, _, _ = c.Projects().ListGroupProjects(context.Background(), 9, 150)
	if len(ps) != 150 || len(queries) != 3 {
		t.Fatalf("len = %d requests = %d", len(ps), len(queries))
	}
}

func TestLatestTag(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/repository/tags": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonReply(200, `[{"name":"v1.14.2"}]`)(w, r)
		},
		"GET /api/v4/projects/7/repository/tags": jsonReply(200, `[]`),
	})
	tag, err := c.LatestTag(context.Background(), ref42)
	if err != nil || tag != "v1.14.2" {
		t.Fatalf("tag = %q %v", tag, err)
	}
	if !strings.Contains(query, "order_by=updated") || !strings.Contains(query, "per_page=1") {
		t.Fatalf("query = %q", query)
	}
	ref7 := ref42
	ref7.ID = 7
	if tag, err := c.LatestTag(context.Background(), ref7); err != nil || tag != "" {
		t.Fatalf("no tags: %q %v", tag, err)
	}
}

func TestSearchFallback(t *testing.T) {
	var pq, gq string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects": func(w http.ResponseWriter, r *http.Request) {
			pq = r.URL.RawQuery
			jsonReply(200, `[{"id":42,"path_with_namespace":"platform/route-service"}]`)(w, r)
		},
		"GET /api/v4/groups": func(w http.ResponseWriter, r *http.Request) {
			gq = r.URL.RawQuery
			jsonReply(200, `[{"id":9,"full_path":"platform","name":"platform"}]`)(w, r)
		},
	})
	ps, gs, err := c.Search().Search(context.Background(), "route")
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || len(gs) != 1 || gs[0].FullPath != "platform" || gs[0].ID != 9 {
		t.Fatalf("ps = %+v gs = %+v", ps, gs)
	}
	if !strings.Contains(pq, "search=route") || !strings.Contains(pq, "search_namespaces=true") || !strings.Contains(gq, "search=route") {
		t.Fatalf("queries %q %q", pq, gq)
	}
}

var ref42 = gitlab.ProjectRef{ID: 42, FullPath: "platform/route-service"}
