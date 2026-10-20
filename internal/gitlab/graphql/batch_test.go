package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/httpx"
)

const mrFragment = `fragment WsMRGroup on Project {
  fullPath
  mergeRequests(state: opened, first: $first, sort: UPDATED_DESC) {
    count
    nodes {
      iid title draft updatedAt webUrl sourceBranch targetBranch detailedMergeStatus
      author { username name }
      headPipeline { id status }
      approvedBy { nodes { username } }
    }
  }
}
`

type mrRow struct {
	IID   string `json:"iid"`
	Title string `json:"title"`
}

func mrSpec() BatchSpec[mrRow] {
	return BatchSpec[mrRow]{
		Kind:         "ws.mrs",
		Fragment:     mrFragment,
		FragmentName: "WsMRGroup",
		VarDecls:     "$first: Int!",
		Vars:         map[string]any{"first": 3},
		Decode: func(raw json.RawMessage) ([]mrRow, int, error) {
			var p struct {
				MergeRequests struct {
					Count int     `json:"count"`
					Nodes []mrRow `json:"nodes"`
				} `json:"mergeRequests"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, 0, err
			}
			return p.MergeRequests.Nodes, p.MergeRequests.Count, nil
		},
	}
}

// fakeServer answers batch queries. Paths listed in missing resolve to null;
// complexityPer > 0 makes queries with score = aliases*complexityPer fail
// when above 250.
type fakeServer struct {
	mu            sync.Mutex
	missing       map[string]bool
	aliasErr      map[string]string
	complexityPer int
	requests      []int // alias count per request
	queries       []string
}

func (f *fakeServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"401 Unauthorized"}`))
			return
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		n := 0
		for k := range req.Variables {
			if strings.HasPrefix(k, "p") {
				n++
			}
		}
		f.mu.Lock()
		f.requests = append(f.requests, n)
		f.queries = append(f.queries, req.Query)
		f.mu.Unlock()

		if f.complexityPer > 0 && n*f.complexityPer > 250 {
			fmt.Fprintf(w, `{"errors":[{"message":"Query has complexity of %d, which exceeds max complexity of 250"}]}`, n*f.complexityPer)
			return
		}
		data := map[string]any{}
		var errs []map[string]any
		for i := 0; i < n; i++ {
			alias := fmt.Sprintf("p%d", i)
			path := req.Variables[alias].(string)
			if f.missing[path] {
				data[alias] = nil
				continue
			}
			if msg, ok := f.aliasErr[path]; ok {
				data[alias] = nil
				errs = append(errs, map[string]any{"message": msg, "path": []any{alias, "mergeRequests"}})
				continue
			}
			data[alias] = map[string]any{
				"fullPath": path,
				"mergeRequests": map[string]any{
					"count": 7,
					"nodes": []map[string]any{{"iid": "1", "title": path + " MR"}},
				},
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data, "errors": errs})
	}
}

func newClient(t *testing.T, f *fakeServer, batch int) *Client {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	hc, _ := httpx.NewClient(httpx.Options{})
	return New(srv.URL, "tok", hc, batch)
}

func collect[T any](ch <-chan gitlab.BatchResult[T]) []gitlab.ProjectGroup[T] {
	var all []gitlab.ProjectGroup[T]
	for r := range ch {
		all = append(all, r.Groups...)
	}
	return all
}

func paths(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("group-%02d/project-with-a-long-name-%02d", i, i)
	}
	return out
}

func TestBatchNullProjectIsolated(t *testing.T) {
	ps := paths(10)
	f := &fakeServer{missing: map[string]bool{ps[3]: true}}
	c := newClient(t, f, 10)
	groups := collect(Batch(context.Background(), c, mrSpec(), ps))
	if len(groups) != 10 {
		t.Fatalf("groups = %d", len(groups))
	}
	SortGroups(groups, ps)
	for i, g := range groups {
		if i == 3 {
			if !errors.Is(g.Err, gitlab.ErrNotFound) {
				t.Fatalf("missing project err = %v", g.Err)
			}
			continue
		}
		if g.Err != nil || g.Total != 7 || len(g.Items) != 1 || g.Items[0].Title != ps[i]+" MR" {
			t.Fatalf("group %d = %+v", i, g)
		}
	}
}

func TestBatchAliasErrorIsolated(t *testing.T) {
	ps := paths(3)
	f := &fakeServer{aliasErr: map[string]string{ps[1]: "boom"}}
	groups := collect(Batch(context.Background(), newClient(t, f, 10), mrSpec(), ps))
	SortGroups(groups, ps)
	if groups[1].Err == nil || !strings.Contains(groups[1].Err.Error(), "boom") {
		t.Fatalf("alias error = %v", groups[1].Err)
	}
	if groups[0].Err != nil || groups[2].Err != nil {
		t.Fatal("other groups must succeed")
	}
}

func TestBatchSplitsIntoChunks(t *testing.T) {
	f := &fakeServer{}
	c := newClient(t, f, 4)
	groups := collect(Batch(context.Background(), c, mrSpec(), paths(10)))
	if len(groups) != 10 {
		t.Fatalf("groups = %d", len(groups))
	}
	if len(f.requests) != 3 {
		t.Fatalf("requests = %v, want 3 chunks (4+4+2)", f.requests)
	}
}

func TestComplexityShrinksAndRemembers(t *testing.T) {
	f := &fakeServer{complexityPer: 40} // 10 aliases → 400 > 250
	c := newClient(t, f, 10)
	groups := collect(Batch(context.Background(), c, mrSpec(), paths(10)))
	if len(groups) != 10 {
		t.Fatalf("groups = %d", len(groups))
	}
	for _, g := range groups {
		if g.Err != nil {
			t.Fatalf("group error after shrinking: %v", g.Err)
		}
	}
	if f.requests[0] != 10 {
		t.Fatalf("first request had %d aliases", f.requests[0])
	}
	for _, n := range f.requests[1:] {
		if n != 5 {
			t.Fatalf("retries must use 5 projects, got %v", f.requests)
		}
	}

	f.mu.Lock()
	f.requests = nil
	f.mu.Unlock()
	collect(Batch(context.Background(), c, mrSpec(), paths(10)))
	for _, n := range f.requests {
		if n != 5 {
			t.Fatalf("learned size not reused: %v", f.requests)
		}
	}
}

func TestQuerySizeFor50Projects(t *testing.T) {
	q, vars := BuildQuery(mrSpec(), paths(50))
	if len(q) >= 10000 {
		t.Fatalf("query is %d chars", len(q))
	}
	if vars["p49"] != paths(50)[49] || vars["first"] != 3 {
		t.Fatalf("vars = %v", vars)
	}
	if strings.Contains(q, "group-00/") {
		t.Fatal("paths must be passed as variables, not inlined")
	}
}

func TestBatchUnauthorized(t *testing.T) {
	f := &fakeServer{}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	hc, _ := httpx.NewClient(httpx.Options{})
	c := New(srv.URL, "wrong", hc, 10)
	var results []gitlab.BatchResult[mrRow]
	for r := range Batch(context.Background(), c, mrSpec(), paths(2)) {
		results = append(results, r)
	}
	if len(results) != 1 || !errors.Is(results[0].Err, gitlab.ErrUnauthorized) {
		t.Fatalf("results = %+v", results)
	}
	for _, g := range results[0].Groups {
		if !errors.Is(g.Err, gitlab.ErrUnauthorized) {
			t.Fatal("batch error must be copied to groups")
		}
	}
}

func TestBatchCancelClosesChannel(t *testing.T) {
	f := &fakeServer{}
	c := newClient(t, f, 1)
	ctx, cancel := context.WithCancel(context.Background())
	ch := Batch(ctx, c, mrSpec(), paths(20))
	<-ch
	cancel()
	for range ch {
	}
}

func TestShrink(t *testing.T) {
	cases := []struct{ n, score, limit, want int }{
		{10, 400, 250, 5},
		{10, 260, 250, 8},
		{2, 1000, 250, 1},
		{10, 251, 250, 8},
		{4, 0, 0, 2},
	}
	for _, c := range cases {
		if got := shrink(c.n, c.score, c.limit); got != c.want {
			t.Errorf("shrink(%d,%d,%d) = %d, want %d", c.n, c.score, c.limit, got, c.want)
		}
	}
}

func TestDoDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"currentUser":{"username":"egor"}}}`))
	}))
	defer srv.Close()
	hc, _ := httpx.NewClient(httpx.Options{})
	c := New(srv.URL, "tok", hc, 10)
	var out struct {
		CurrentUser struct{ Username string } `json:"currentUser"`
	}
	if err := c.Do(context.Background(), "{ currentUser { username } }", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.CurrentUser.Username != "egor" {
		t.Fatalf("out = %+v", out)
	}
}
