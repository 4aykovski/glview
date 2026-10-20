package hybrid

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/graphql"
	"github.com/4aykovski/glview/internal/gitlab/httpx"
	"github.com/4aykovski/glview/internal/gitlab/rest"
)

func TestRoutesByTransport(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	record := func(s string) { mu.Lock(); hits = append(hits, s); mu.Unlock() }

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.Contains(string(b), "query ProjectMRs"):
			record("graphql:ProjectMRs")
			w.Write([]byte(`{"data":{"project":{"m":{"count":1,"pageInfo":{"hasNextPage":false},"nodes":[{"iid":"7","title":"t","state":"opened","detailedMergeStatus":"MERGEABLE","approvedBy":{"nodes":[{"username":"egor"}]}}]}}}}`))
		case strings.Contains(string(b), "query MR("):
			record("graphql:MR")
			w.Write([]byte(`{"data":{"project":{"mergeRequest":{"iid":"7","title":"t","state":"opened"}}}}`))
		case strings.Contains(string(b), "query MRPipelines"):
			record("graphql:MRPipelines")
			w.Write([]byte(`{"data":{"project":{"mergeRequest":{"pipelines":{"nodes":[]}}}}}`))
		default:
			record("graphql:other")
			w.Write([]byte(`{"data":{}}`))
		}
	})
	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/7/approve", func(w http.ResponseWriter, r *http.Request) {
		record("rest:approve")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write([]byte(`{"id":1}`))
	})
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/7", func(w http.ResponseWriter, r *http.Request) {
		record("rest:recheck=" + r.URL.Query().Get("with_merge_status_recheck"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"iid":7,"detailed_merge_status":"checking"}`))
	})
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/7/commits", func(w http.ResponseWriter, r *http.Request) {
		record("rest:commits")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":"a1a1a1","short_id":"a1a1","title":"fix"}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	hc, _ := httpx.NewClient(httpx.Options{})
	r, err := rest.New(srv.URL, "tok", hc)
	if err != nil {
		t.Fatal(err)
	}
	c := New(r, graphql.New(srv.URL, "tok", hc, 10), "egor")

	ctx := context.Background()
	p := gitlab.ProjectRef{ID: 42, FullPath: "g/p"}
	page, err := c.MergeRequests().List(ctx, p, gitlab.MRFilter{State: gitlab.MRStateOpened}, "", 20)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list: %+v %v", page, err)
	}
	if !page.Items[0].Approvals.ApprovedByMe {
		t.Error("the current user must be passed to GraphQL mapping")
	}
	if _, err := c.MergeRequests().Get(ctx, p, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := c.MergeRequests().Pipelines(ctx, p, 7); err != nil {
		t.Fatal(err)
	}
	if err := c.MergeRequests().Approve(ctx, p, 7, "sha"); err != nil {
		t.Fatal(err)
	}
	if st, err := c.MergeRequests().RecheckMergeStatus(ctx, p, 7); err != nil || st != "checking" {
		t.Fatalf("recheck: %q %v", st, err)
	}
	if cs, err := c.MergeRequests().Commits(ctx, p, 7); err != nil || len(cs) != 1 {
		t.Fatalf("commits: %+v %v", cs, err)
	}
	want := "graphql:ProjectMRs,graphql:MR,graphql:MRPipelines,rest:approve,rest:recheck=true,rest:commits"
	if got := strings.Join(hits, ","); got != want {
		t.Fatalf("hits = %s, want %s", got, want)
	}
}
