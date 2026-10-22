package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestBuildQueryPerProjectVariables(t *testing.T) {
	c := New("https://gitlab.example.com", "tok", nil, 10)
	ps := paths(50)
	refs := map[string]string{}
	for i, p := range ps {
		refs[p] = fmt.Sprintf("main-%d", i)
	}
	spec := c.overviewSpec(OverviewRequest{Paths: ps, RootRefs: refs, Me: "egor", Phase1: true, Phase2: true})
	q, vars := BuildQuery(spec, ps)
	if len(q) >= 10000 {
		t.Fatalf("query of 50 projects is %d chars", len(q))
	}
	for i := range ps {
		if !strings.Contains(q, fmt.Sprintf("$r%d: String!", i)) {
			t.Fatalf("$r%d is not declared", i)
		}
		if vars[fmt.Sprintf("r%d", i)] != refs[ps[i]] {
			t.Fatalf("r%d = %v", i, vars[fmt.Sprintf("r%d", i)])
		}
	}
	// Every alias uses its own variable only.
	re := regexp.MustCompile(`p(\d+): project\(fullPath: \$p\d+\) \{[^\n]*pipelines\(ref: \$r(\d+)`)
	m := re.FindAllStringSubmatch(q, -1)
	if len(m) != 50 {
		t.Fatalf("aliases with pipelines = %d", len(m))
	}
	for _, x := range m {
		if x[1] != x[2] {
			t.Fatalf("alias p%s uses $r%s", x[1], x[2])
		}
	}
	if vars["me"] != "egor" || strings.Contains(q, "main-1") {
		t.Fatal("refs must be passed as variables")
	}
}

const overviewResponse = `{"data":{
 "p0":{"id":"gid://gitlab/Project/42","name":"route-service","webUrl":"https://gitlab.example.com/platform/route-service",
   "lastActivityAt":"2026-10-19T11:00:00Z","archived":false,"repository":{"rootRef":"main"},
   "opened":{"count":6},"review":{"count":2},
   "dp":{"nodes":[{"id":"gid://gitlab/Ci::Pipeline/91790","iid":"790","status":"MANUAL","path":"/platform/route-service/-/pipelines/91790","createdAt":"2026-10-19T07:00:00Z"}]}},
 "p1":null,
 "p2":{"id":"gid://gitlab/Project/7","name":"empty","webUrl":"u","lastActivityAt":"2026-10-01T00:00:00Z","archived":true,
   "repository":{"rootRef":null},"opened":{"count":0},"review":{"count":0},"dp":{"nodes":[]}},
 "p3":null},
 "errors":[{"message":"boom","path":["p3","opened"]}]}`

func TestOverviewMapping(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string { return overviewResponse })
	ps := []string{"platform/route-service", "gone/project", "qa/empty", "qa/broken"}
	refs := map[string]string{ps[0]: "main", ps[1]: "main", ps[2]: "main", ps[3]: "main"}
	groups := collect(c.Overview(context.Background(), OverviewRequest{Paths: ps, RootRefs: refs, Me: "egor", Phase1: true, Phase2: true}))
	SortGroups(groups, ps)
	if rec.count() != 1 {
		t.Fatalf("requests = %d", rec.count())
	}
	if !strings.Contains(rec.queries[0], "dp: pipelines(ref: $r0") || !strings.Contains(rec.queries[0], "reviewerUsername: $me") {
		t.Fatalf("query:\n%s", rec.queries[0])
	}
	ov := groups[0].Items[0]
	if ov.ID != 42 || ov.OpenMRs != 6 || ov.ReviewMRs != 2 || ov.DefaultBranch != "main" || ov.Phase != 2 {
		t.Fatalf("overview = %+v", ov)
	}
	if p := ov.DefaultPipeline; p == nil || p.ID != 91790 || p.Status != gitlab.StatusManual || !strings.HasSuffix(p.WebURL, "/pipelines/91790") || p.CreatedAt.IsZero() {
		t.Fatalf("pipeline = %+v", ov.DefaultPipeline)
	}
	if !errors.Is(groups[1].Err, gitlab.ErrNotFound) {
		t.Fatalf("null project: %v", groups[1].Err)
	}
	if e := groups[2].Items[0]; !e.Archived || e.DefaultBranch != "" || e.DefaultPipeline != nil {
		t.Fatalf("empty repo = %+v", e)
	}
	if groups[3].Err == nil || !strings.Contains(groups[3].Err.Error(), "boom") {
		t.Fatalf("alias error = %v", groups[3].Err)
	}
}

func TestOverviewPhases(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string {
		return `{"data":{"p0":{"dp":{"nodes":[{"id":"gid://gitlab/Ci::Pipeline/5","status":"SUCCESS"}]}}}}`
	})
	g := collect(c.Overview(context.Background(), OverviewRequest{Paths: []string{"a/b"}, RootRefs: map[string]string{"a/b": "develop"}, Phase2: true}))
	q := rec.queries[0]
	if strings.Contains(q, "WsOverview") || strings.Contains(q, "$me") {
		t.Fatalf("phase 2 query must not ask phase 1:\n%s", q)
	}
	ov := g[0].Items[0]
	if ov.Phase != 2 || ov.DefaultBranch != "develop" || ov.DefaultPipeline == nil || ov.DefaultPipeline.Status != gitlab.StatusSuccess {
		t.Fatalf("p2 = %+v", ov)
	}

	c1, rec1 := newRecorder(t, func(int, string, map[string]any) string {
		return `{"data":{"p0":{"name":"b","repository":{"rootRef":"main"},"opened":{"count":1},"review":{"count":0}}}}`
	})
	g = collect(c1.Overview(context.Background(), OverviewRequest{Paths: []string{"a/b"}, Me: "egor", Phase1: true}))
	if strings.Contains(rec1.queries[0], "pipelines(") {
		t.Fatal("phase 1 must not ask pipelines")
	}
	if ov := g[0].Items[0]; ov.Phase != 1 || ov.DefaultBranch != "main" || ov.OpenMRs != 1 {
		t.Fatalf("p1 = %+v", ov)
	}
}

func mrNode(iid int, updated string) string {
	return fmt.Sprintf(`{"iid":"%d","title":"MR %d","state":"opened","updatedAt":"%s","author":{"username":"anna.k"}}`, iid, iid, updated)
}

func TestWorkspaceMRsMine(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string {
		return `{"data":{"p0":{
		 "a":{"count":2,"nodes":[` + mrNode(479, "2026-10-19T10:00:00Z") + `,` + mrNode(470, "2026-10-10T10:00:00Z") + `]},
		 "s":{"count":0,"nodes":[]},
		 "r":{"count":1,"nodes":[` + mrNode(479, "2026-10-19T10:00:00Z") + `]}}}}`
	})
	g := collect(c.WorkspaceMRs(context.Background(), []string{"platform/route-service"}, gitlab.MRFilter{State: gitlab.MRStateOpened, Mine: true}, "egor", 3))
	q := rec.queries[0]
	for _, want := range []string{"a: mergeRequests(", "authorUsername: $me", "assigneeUsername: $me", "reviewerUsername: $me", "...MRRow"} {
		if !strings.Contains(q, want) {
			t.Fatalf("query lacks %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "#@opt") {
		t.Fatal("optional markers must be rendered")
	}
	if len(g[0].Items) != 2 || g[0].Items[0].IID != 479 || g[0].Total != 2 || g[0].Items[0].Project.FullPath != "platform/route-service" {
		t.Fatalf("group = %+v", g[0])
	}
}

func TestWorkspaceMRsPlainAndPipelines(t *testing.T) {
	c, rec := newRecorder(t, func(n int, q string, _ map[string]any) string {
		if strings.Contains(q, "WsPipeGroup") {
			return `{"data":{"p0":{"pipelines":{"count":14,"nodes":[{"id":"gid://gitlab/Ci::Pipeline/9","iid":"3","status":"RUNNING","ref":"main"}]}}}}`
		}
		return `{"data":{"p0":{"m":{"count":14,"nodes":[` + mrNode(1, "2026-10-19T10:00:00Z") + `]}}}}`
	})
	g := collect(c.WorkspaceMRs(context.Background(), []string{"a/b"}, gitlab.MRFilter{State: gitlab.MRStateMerged, Search: "fix"}, "egor", 3))
	if g[0].Total != 14 || len(g[0].Items) != 1 {
		t.Fatalf("mrs = %+v", g[0])
	}
	if q := rec.queries[0]; !strings.Contains(q, "state: $state, search: $search") || strings.Contains(q, "$me") {
		t.Fatalf("query:\n%s", q)
	}
	if rec.vars[0]["state"] != "merged" || rec.vars[0]["search"] != "fix" || rec.vars[0]["first"] != float64(3) {
		t.Fatalf("vars = %v", rec.vars[0])
	}
	pg := collect(c.WorkspacePipelines(context.Background(), []string{"a/b"}, gitlab.PipelineFilter{Status: "running", Username: "egor"}, 3))
	if pg[0].Total != 14 || pg[0].Items[0].ID != 9 || pg[0].Items[0].Status != gitlab.StatusRunning {
		t.Fatalf("pipelines = %+v", pg[0])
	}
	if q := rec.queries[1]; !strings.Contains(q, "pipelines(status: $status, username: $username, first: $first)") {
		t.Fatalf("query:\n%s", q)
	}
	if rec.vars[1]["status"] != "RUNNING" {
		t.Fatalf("vars = %v", rec.vars[1])
	}
}

func TestWorkspaceMRKindsLearnSeparately(t *testing.T) {
	f := &fakeServer{complexityPer: 40}
	c := newClient(t, f, 10)
	spec := c.mrGroupSpec(gitlab.MRFilter{Mine: true}, "egor", 3)
	spec.Decode = func(string, json.RawMessage) ([]gitlab.MergeRequest, int, error) { return nil, 0, nil }
	collect(Batch(context.Background(), c, spec, paths(10)))
	if c.batchSize(KindMRsMine) != 5 {
		t.Fatalf("learned mine = %d", c.batchSize(KindMRsMine))
	}
	if c.batchSize(KindMRs) != 10 {
		t.Fatalf("plain MR batches must keep their own size, got %d", c.batchSize(KindMRs))
	}
}

func TestSearch(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string {
		return `{"data":{"projects":{"nodes":[{"id":"gid://gitlab/Project/42","fullPath":"platform/route-service","name":"route-service","lastActivityAt":"2026-10-19T10:00:00Z"}]},
		"groups":{"nodes":[{"id":"gid://gitlab/Group/9","fullPath":"platform","name":"platform"}]}}}`
	})
	ps, gs, err := c.Search(context.Background(), "route")
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].ID != 42 || ps[0].FullPath != "platform/route-service" || len(gs) != 1 || gs[0].ID != 9 {
		t.Fatalf("ps = %+v gs = %+v", ps, gs)
	}
	if rec.vars[0]["q"] != "route" || !strings.Contains(rec.queries[0], "searchNamespaces: true") {
		t.Fatal("query")
	}

	c2, _ := newRecorder(t, func(int, string, map[string]any) string {
		return `{"errors":[{"message":"Field 'projects' doesn't accept argument 'searchNamespaces'"}]}`
	})
	_, _, err = c2.Search(context.Background(), "route")
	var se *SchemaError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v", err)
	}
}

func TestProjectMRsBySourceBranch(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string {
		return `{"data":{"project":{"m":{"count":1,"pageInfo":{"hasNextPage":false},"nodes":[` + mrNode(479, "2026-10-19T10:00:00Z") + `]}}}}`
	})
	page, err := c.ProjectMRs(context.Background(), gitlab.ProjectRef{FullPath: "a/b"}, gitlab.MRFilter{State: gitlab.MRStateOpened, SourceBranch: "fix/slot-race"}, "egor", "", 1)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("page = %+v %v", page, err)
	}
	if !strings.Contains(rec.queries[0], "sourceBranches: $sourceBranches") {
		t.Fatalf("query:\n%s", rec.queries[0])
	}
	if got := rec.vars[0]["sourceBranches"]; fmt.Sprint(got) != "[fix/slot-race]" {
		t.Fatalf("vars = %v", got)
	}
}
