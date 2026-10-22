package hybrid

import (
	"context"
	"encoding/json"
	"fmt"
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

// wsServer answers overview batches for projects g/p0…g/pN and the tag
// requests; it counts requests by kind.
type wsServer struct {
	mu          sync.Mutex
	gql         []string // "full", "p1", "p2", "search"
	tags        int
	searchFails bool
	rest        []string
	// fieldErr makes a field of a project null with a GraphQL error
	// addressed to it (path → field alias, e.g. "dp" or "review").
	fieldErr map[string]string
	p2Paths  []string // projects asked in phase 2 requests
}

func (s *wsServer) handler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	q := req.Query
	kind := "p2"
	switch {
	case strings.Contains(q, "query Search"):
		kind = "search"
	case strings.Contains(q, "WsOverview") && strings.Contains(q, "dp: pipelines"):
		kind = "full"
	case strings.Contains(q, "WsOverview"):
		kind = "p1"
	}
	s.mu.Lock()
	s.gql = append(s.gql, kind)
	fails := s.searchFails
	s.mu.Unlock()
	if kind == "search" {
		if fails {
			w.Write([]byte(`{"errors":[{"message":"Field 'projects' doesn't accept argument 'searchNamespaces'"}]}`))
			return
		}
		w.Write([]byte(`{"data":{"projects":{"nodes":[{"fullPath":"g/p0"}]},"groups":{"nodes":[]}}}`))
		return
	}
	data := map[string]any{}
	var errs []any
	for k, v := range req.Variables {
		if !strings.HasPrefix(k, "p") {
			continue
		}
		path := v.(string)
		id := strings.TrimPrefix(path, "g/p")
		node := map[string]any{}
		if kind != "p2" {
			node["id"] = "gid://gitlab/Project/" + id
			node["name"] = "p" + id
			node["repository"] = map[string]any{"rootRef": "main"}
			node["opened"] = map[string]any{"count": 3}
			node["review"] = map[string]any{"count": 1}
		}
		if kind != "p1" {
			node["dp"] = map[string]any{"nodes": []any{map[string]any{"id": "gid://gitlab/Ci::Pipeline/1" + id, "status": "SUCCESS"}}}
		}
		if field, ok := s.fieldErr[path]; ok {
			if _, asked := node[field]; asked {
				node[field] = nil
				errs = append(errs, map[string]any{"message": "insufficient permissions for " + field, "path": []any{k, field}})
			}
		}
		if kind == "p2" {
			s.mu.Lock()
			s.p2Paths = append(s.p2Paths, path)
			s.mu.Unlock()
		}
		data[k] = node
	}
	body := map[string]any{"data": data}
	if len(errs) > 0 {
		body["errors"] = errs
	}
	json.NewEncoder(w).Encode(body)
}

func (s *wsServer) count(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, k := range s.gql {
		if k == kind {
			n++
		}
	}
	return n
}

func newWS(t *testing.T) (*Client, *wsServer) {
	s := &wsServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/graphql", s.handler)
	mux.HandleFunc("GET /api/v4/projects/{id}/repository/tags", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.tags++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"name":"v1.0.` + r.PathValue("id") + `"}]`))
	})
	mux.HandleFunc("GET /api/v4/projects", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.rest = append(s.rest, "projects?"+r.URL.RawQuery)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":1,"path_with_namespace":"g/p1"}]`))
	})
	mux.HandleFunc("GET /api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":9,"full_path":"g"}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	hc, _ := httpx.NewClient(httpx.Options{})
	r, err := rest.New(srv.URL, "tok", hc)
	if err != nil {
		t.Fatal(err)
	}
	return New(r, graphql.New(srv.URL, "tok", hc, 10), "egor"), s
}

func refs(n int) []gitlab.ProjectRef {
	out := make([]gitlab.ProjectRef, n)
	for i := range out {
		out[i] = gitlab.ProjectRef{FullPath: fmt.Sprintf("g/p%d", i+1)}
	}
	return out
}

// final collects the last report of every project.
func final(ch <-chan gitlab.BatchResult[gitlab.ProjectOverview]) map[string]gitlab.ProjectOverview {
	out := map[string]gitlab.ProjectOverview{}
	for res := range ch {
		for _, g := range res.Groups {
			if g.Err == nil && len(g.Items) > 0 {
				out[g.Project.FullPath] = g.Items[0]
			}
		}
	}
	return out
}

func TestOverviewPhasesAndRootRefCache(t *testing.T) {
	c, s := newWS(t)
	ctx := context.Background()
	got := final(c.Workspace().ProjectsOverview(ctx, refs(10), gitlab.OverviewOptions{}))
	if s.count("p1") != 1 || s.count("p2") != 1 || s.count("full") != 0 {
		t.Fatalf("first overview: %v", s.gql)
	}
	ov := got["g/p3"]
	if ov.Phase != 2 || ov.ID != 3 || ov.OpenMRs != 3 || ov.ReviewMRs != 1 || ov.DefaultPipeline == nil || ov.LastTag != "v1.0.3" || !ov.TagKnown {
		t.Fatalf("overview = %+v", ov)
	}
	if s.tags != 10 {
		t.Fatalf("tag requests = %d", s.tags)
	}

	s.gql = nil
	got = final(c.Workspace().ProjectsOverview(ctx, refs(10), gitlab.OverviewOptions{}))
	if len(s.gql) != 1 || s.gql[0] != "full" {
		t.Fatalf("second overview must be one combined request per batch: %v", s.gql)
	}
	if s.tags != 10 {
		t.Fatalf("tags within 10 minutes must come from the cache: %d", s.tags)
	}
	if got["g/p3"].LastTag != "v1.0.3" || got["g/p3"].DefaultPipeline == nil {
		t.Fatalf("cached overview = %+v", got["g/p3"])
	}

	final(c.Workspace().ProjectsOverview(ctx, refs(10), gitlab.OverviewOptions{ForceTags: true}))
	if s.tags != 20 {
		t.Fatalf("r must refetch every tag: %d", s.tags)
	}

	got = final(c.Workspace().ProjectsOverview(ctx, refs(10), gitlab.OverviewOptions{ForceTags: true, SkipTags: true}))
	if s.tags != 20 {
		t.Fatalf("SkipTags asks no tags: %d", s.tags)
	}
	if ov := got["g/p3"]; ov.TagKnown || ov.Phase != 2 || ov.DefaultPipeline == nil {
		t.Fatalf("overview without tags = %+v", ov)
	}
}

func TestOverviewKnownRootRefsFromSnapshot(t *testing.T) {
	c, s := newWS(t)
	known := map[string]string{}
	for _, p := range refs(3) {
		known[p.FullPath] = "main"
	}
	final(c.Workspace().ProjectsOverview(context.Background(), refs(3), gitlab.OverviewOptions{RootRefs: known}))
	if len(s.gql) != 1 || s.gql[0] != "full" {
		t.Fatalf("requests = %v", s.gql)
	}
}

func TestOverviewKeepsRowWithFieldError(t *testing.T) {
	c, s := newWS(t)
	s.fieldErr = map[string]string{"g/p2": "dp"}
	known := map[string]string{}
	for _, p := range refs(3) {
		known[p.FullPath] = "main"
	}
	got := final(c.Workspace().ProjectsOverview(context.Background(), refs(3), gitlab.OverviewOptions{RootRefs: known}))
	ov, ok := got["g/p2"]
	if !ok {
		t.Fatalf("row with a field error was dropped: %v", got)
	}
	if ov.ID != 2 || ov.Name != "p2" || ov.OpenMRs != 3 || ov.ReviewMRs != 1 || ov.Phase != 2 || ov.DefaultPipeline != nil {
		t.Fatalf("overview = %+v", ov)
	}
	if ov.LastTag != "v1.0.2" || !ov.TagKnown {
		t.Fatalf("tag of the row was not fetched: %+v", ov)
	}
	if got["g/p1"].DefaultPipeline == nil {
		t.Fatalf("other rows must keep their pipeline: %+v", got["g/p1"])
	}
}

func TestOverviewFieldErrorStillRunsPhase2(t *testing.T) {
	c, s := newWS(t)
	s.fieldErr = map[string]string{"g/p2": "review", "g/p3": "dp"}
	got := final(c.Workspace().ProjectsOverview(context.Background(), refs(3), gitlab.OverviewOptions{}))
	if s.count("p2") != 1 {
		t.Fatalf("requests = %v", s.gql)
	}
	s.mu.Lock()
	p2 := strings.Join(s.p2Paths, ",")
	s.mu.Unlock()
	if !strings.Contains(p2, "g/p2") {
		t.Fatalf("phase 2 must include the row with a field error: %s", p2)
	}
	ov := got["g/p2"]
	if ov.Name != "p2" || ov.OpenMRs != 3 || ov.ReviewMRs != 0 || ov.Phase != 2 || ov.DefaultPipeline == nil || !ov.TagKnown {
		t.Fatalf("overview g/p2 = %+v", ov)
	}
	ov = got["g/p3"]
	if ov.Name != "p3" || ov.OpenMRs != 3 || ov.Phase != 2 || ov.DefaultPipeline != nil || !ov.TagKnown {
		t.Fatalf("overview g/p3 = %+v", ov)
	}
}

func TestSearchFallsBackToREST(t *testing.T) {
	c, s := newWS(t)
	ps, _, err := c.Search().Search(context.Background(), "route")
	if err != nil || len(ps) != 1 || ps[0].FullPath != "g/p0" || len(s.rest) != 0 {
		t.Fatalf("graphql search: %+v %v %v", ps, err, s.rest)
	}
	s.searchFails = true
	ps, gs, err := c.Search().Search(context.Background(), "route")
	if err != nil || len(ps) != 1 || ps[0].FullPath != "g/p1" || len(gs) != 1 {
		t.Fatalf("rest search: %+v %+v %v", ps, gs, err)
	}
	if len(s.rest) != 1 || !strings.Contains(s.rest[0], "search=route") {
		t.Fatalf("rest = %v", s.rest)
	}
}
