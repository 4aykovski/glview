package rest

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestPipelineGetCollapsesRetriesAndOrdersStages(t *testing.T) {
	var commitCalls atomic.Int32
	var includeRetried string
	c, srv := newServer(t, "/gitlab", map[string]func(http.ResponseWriter, *http.Request){
		"GET /gitlab/api/v4/projects/42/pipelines/91840": jsonReply(200,
			`{"id":91840,"iid":512,"status":"running","source":"merge_request_event","ref":"feat/surge","sha":"a91f2c0d","duration":220,
			  "created_at":"2026-10-19T10:00:00Z","web_url":"https://x/p","user":{"username":"anna.k","name":"Anna"},"yaml_errors":""}`),
		"GET /gitlab/api/v4/projects/42/pipelines/91840/jobs": func(w http.ResponseWriter, r *http.Request) {
			includeRetried = r.URL.Query().Get("include_retried")
			w.Header().Set("Content-Type", "application/json")
			// newest first, as GitLab returns them; page 1 of 2
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("X-Next-Page", "2")
				w.Write([]byte(`[
				  {"id":305,"name":"test:unit","stage":"test","status":"running","duration":12.5,"runner":{"description":"runner-1"}},
				  {"id":304,"name":"build","stage":"build","status":"success","duration":60}]`))
				return
			}
			w.Write([]byte(`[
			  {"id":303,"name":"test:unit","stage":"test","status":"failed","allow_failure":true},
			  {"id":302,"name":"test:unit","stage":"test","status":"failed"},
			  {"id":301,"name":"build","stage":"build","status":"failed"},
			  {"id":306,"name":"deploy","stage":"deploy","status":"manual"}]`))
		},
		"GET /gitlab/api/v4/projects/42/pipelines/91840/bridges": func(w http.ResponseWriter, r *http.Request) {
			jsonReply(200, `[
			  {"id":307,"name":"e2e","stage":"deploy","status":"running",
			   "downstream_pipeline":{"id":5521,"iid":9,"project_id":77,"status":"running","ref":"main","sha":"s","web_url":"`+"http://"+r.Host+`/gitlab/qa/e2e-routes/-/pipelines/5521"}},
			  {"id":308,"name":"waiting","stage":"deploy","status":"created","downstream_pipeline":null}]`)(w, r)
		},
		"GET /gitlab/api/v4/projects/42/repository/commits/a91f2c0d": func(w http.ResponseWriter, r *http.Request) {
			commitCalls.Add(1)
			jsonReply(200, `{"id":"a91f2c0d","title":"surge: add pricing"}`)(w, r)
		},
	})
	_ = srv
	ctx := context.Background()
	pl, err := c.Pipelines().Get(ctx, proj, 91840)
	if err != nil {
		t.Fatal(err)
	}
	if includeRetried != "true" {
		t.Errorf("include_retried = %q", includeRetried)
	}
	if pl.ID != 91840 || pl.Status != gitlab.StatusRunning || pl.User.Username != "anna.k" || pl.CommitTitle != "surge: add pricing" {
		t.Errorf("pipeline = %+v", pl.PipelineSummary)
	}
	// build(301 failed, 304 success) -> 304 wins with RetryCount 1;
	// test:unit(302,303,305) -> 305 wins with RetryCount 2.
	byName := map[string]gitlab.Job{}
	for _, j := range pl.Jobs {
		byName[j.Name] = j
	}
	if len(pl.Jobs) != 3 || byName["build"].ID != 304 || byName["build"].RetryCount != 1 || byName["test:unit"].ID != 305 || byName["test:unit"].RetryCount != 2 {
		t.Errorf("jobs = %+v", pl.Jobs)
	}
	if byName["test:unit"].Runner != "runner-1" || byName["test:unit"].Duration.Seconds() != 12.5 {
		t.Errorf("job fields = %+v", byName["test:unit"])
	}
	if len(pl.Bridges) != 2 {
		t.Fatalf("bridges = %+v", pl.Bridges)
	}
	e2e := pl.Bridges[0]
	if e2e.Downstream == nil || e2e.Downstream.ID != 5521 || e2e.Downstream.Project.ID != 77 || e2e.DownstreamProject != "qa/e2e-routes" ||
		e2e.Downstream.Project.FullPath != "qa/e2e-routes" {
		t.Errorf("downstream = %+v", e2e)
	}
	if pl.Bridges[1].Downstream != nil {
		t.Errorf("waiting bridge must have no downstream: %+v", pl.Bridges[1])
	}
	var names []string
	for _, s := range pl.Stages {
		names = append(names, s.Name+":"+string(s.Status))
	}
	if got := strings.Join(names, ","); got != "build:success,test:running,deploy:running" {
		// deploy has manual job 306, running bridge 307 and created bridge 308
		t.Errorf("stages = %s", got)
	}
	// Second call: the commit title is cached.
	if _, err := c.Pipelines().Get(ctx, proj, 91840); err != nil {
		t.Fatal(err)
	}
	if commitCalls.Load() != 1 {
		t.Errorf("commit requested %d times, want 1", commitCalls.Load())
	}
}

func TestPipelineCreateRetryCancel(t *testing.T) {
	var created map[string]any
	var retried, canceled bool
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/42/pipeline": func(w http.ResponseWriter, r *http.Request) {
			created = body(t, r)
			jsonReply(201, `{"id":9001,"iid":3,"status":"created","ref":"feat/surge"}`)(w, r)
		},
		"POST /api/v4/projects/42/pipelines/9001/retry":  func(w http.ResponseWriter, r *http.Request) { retried = true; jsonReply(201, `{"id":9001}`)(w, r) },
		"POST /api/v4/projects/42/pipelines/9001/cancel": func(w http.ResponseWriter, r *http.Request) { canceled = true; jsonReply(200, `{"id":9001}`)(w, r) },
	})
	ctx := context.Background()
	ps, err := c.Pipelines().Create(ctx, proj, "feat/surge", []gitlab.CIVariable{
		{Key: "DEPLOY_ENV", Value: "staging", Type: gitlab.VarEnv}, {Key: "CFG", Value: "x", Type: gitlab.VarFile},
	})
	if err != nil || ps.ID != 9001 {
		t.Fatalf("%+v %v", ps, err)
	}
	vars, _ := created["variables"].([]any)
	if created["ref"] != "feat/surge" || len(vars) != 2 {
		t.Fatalf("create body = %v", created)
	}
	v0 := vars[0].(map[string]any)
	v1 := vars[1].(map[string]any)
	if v0["key"] != "DEPLOY_ENV" || v0["value"] != "staging" || v0["variable_type"] != "env_var" || v1["variable_type"] != "file" {
		t.Errorf("variables = %v", vars)
	}
	if err := c.Pipelines().Retry(ctx, proj, 9001); err != nil || !retried {
		t.Fatalf("retry: %v %v", retried, err)
	}
	if err := c.Pipelines().Cancel(ctx, proj, 9001); err != nil || !canceled {
		t.Fatalf("cancel: %v %v", canceled, err)
	}
}

func TestJobsPlayTraceRetryCancel(t *testing.T) {
	var played map[string]any
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/42/jobs/7/play": func(w http.ResponseWriter, r *http.Request) {
			played = body(t, r)
			jsonReply(200, `{"id":7,"name":"deploy","stage":"deploy","status":"pending"}`)(w, r)
		},
		"GET /api/v4/projects/42/jobs/7/trace": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("line 1\nline 2\n"))
		},
		"GET /api/v4/projects/42/jobs/7":         jsonReply(200, `{"id":7,"name":"deploy","stage":"deploy","status":"running","allow_failure":true,"runner":{"description":"r"}}`),
		"POST /api/v4/projects/42/jobs/7/retry":  jsonReply(201, `{"id":8,"name":"deploy","status":"pending"}`),
		"POST /api/v4/projects/42/jobs/7/cancel": jsonReply(201, `{"id":7,"status":"canceled"}`),
	})
	ctx := context.Background()
	j, err := c.Jobs().Play(ctx, proj, 7, []gitlab.CIVariable{{Key: "TARGET", Value: "canary", Type: gitlab.VarEnv}})
	if err != nil || j.Status != gitlab.StatusPending {
		t.Fatalf("%+v %v", j, err)
	}
	attrs, _ := played["job_variables_attributes"].([]any)
	if len(attrs) != 1 || attrs[0].(map[string]any)["key"] != "TARGET" || attrs[0].(map[string]any)["value"] != "canary" {
		t.Errorf("play body = %v", played)
	}
	tr, err := c.Jobs().Trace(ctx, proj, 7)
	if err != nil || string(tr) != "line 1\nline 2\n" {
		t.Fatalf("trace %q %v", tr, err)
	}
	got, err := c.Jobs().Get(ctx, proj, 7)
	if err != nil || got.Status != gitlab.StatusRunning || !got.AllowFailure || got.Runner != "r" {
		t.Fatalf("get %+v %v", got, err)
	}
	if nj, err := c.Jobs().Retry(ctx, proj, 7); err != nil || nj.ID != 8 {
		t.Fatalf("retry %+v %v", nj, err)
	}
	if err := c.Jobs().Cancel(ctx, proj, 7); err != nil {
		t.Fatal(err)
	}
}

func TestPlayWithoutVariables(t *testing.T) {
	var played map[string]any
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/42/jobs/7/play": func(w http.ResponseWriter, r *http.Request) {
			played = body(t, r)
			jsonReply(200, `{"id":7}`)(w, r)
		},
	})
	if _, err := c.Jobs().Play(context.Background(), proj, 7, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := played["job_variables_attributes"]; ok {
		t.Errorf("empty form must play without variables: %v", played)
	}
}

func TestBranchesAndTags(t *testing.T) {
	var branchQ, tagQ string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/repository/branches": func(w http.ResponseWriter, r *http.Request) {
			branchQ = r.URL.RawQuery
			jsonReply(200, `[{"name":"feat/surge"},{"name":"feat/slots"}]`)(w, r)
		},
		"GET /api/v4/projects/42/repository/tags": func(w http.ResponseWriter, r *http.Request) {
			tagQ = r.URL.RawQuery
			w.Header().Set("X-Total", "3")
			w.Header().Set("X-Next-Page", "2")
			jsonReply(200, `[{"name":"v1.14.2","message":"m","protected":true,"commit":{"id":"abcdef1234","short_id":"abcdef1","title":"t"}}]`)(w, r)
		},
	})
	ctx := context.Background()
	bs, err := c.Projects().Branches(ctx, proj, "feat/s")
	if err != nil || len(bs) != 2 || bs[0] != "feat/surge" || !strings.Contains(branchQ, "search=feat%2Fs") {
		t.Fatalf("%v %v %s", bs, err, branchQ)
	}
	page, err := c.Tags().List(ctx, proj, "v1", "", 5)
	if err != nil || len(page.Items) != 1 || page.Total != 3 || page.NextCursor != "2" || page.Items[0].Commit.ShortSHA != "abcdef1" || !page.Items[0].Protected {
		t.Fatalf("%+v %v", page, err)
	}
	if !strings.Contains(tagQ, "search=v1") || !strings.Contains(tagQ, "per_page=5") {
		t.Errorf("tag query = %s", tagQ)
	}
}

func TestMembersAndLabels(t *testing.T) {
	var memberQ, labelQ string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/members/all": func(w http.ResponseWriter, r *http.Request) {
			memberQ = r.URL.RawQuery
			jsonReply(200, `[{"id":3,"username":"anna.k","name":"Anna K"},{"id":4,"username":"andrey","name":"Andrey"}]`)(w, r)
		},
		"GET /api/v4/projects/42/labels": func(w http.ResponseWriter, r *http.Request) {
			labelQ = r.URL.RawQuery
			jsonReply(200, `[{"id":1,"name":"backend"},{"id":2,"name":"bug"}]`)(w, r)
		},
	})
	ctx := context.Background()
	us, err := c.Projects().Members(ctx, proj, "an")
	if err != nil || len(us) != 2 || us[0].Username != "anna.k" || us[0].Name != "Anna K" || us[0].ID != 3 {
		t.Fatalf("%+v %v", us, err)
	}
	if !strings.Contains(memberQ, "query=an") || !strings.Contains(memberQ, "per_page=20") {
		t.Errorf("member query = %s", memberQ)
	}
	ls, err := c.Projects().Labels(ctx, proj, "b")
	if err != nil || len(ls) != 2 || ls[0] != "backend" {
		t.Fatalf("%v %v", ls, err)
	}
	if !strings.Contains(labelQ, "search=b") || !strings.Contains(labelQ, "per_page=20") {
		t.Errorf("label query = %s", labelQ)
	}
}
