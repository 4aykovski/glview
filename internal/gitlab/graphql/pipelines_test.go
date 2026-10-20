package graphql

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestProjectPipelines(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string { return fixture(t, "project_pipelines.json") })
	page, err := c.ProjectPipelines(context.Background(), testProject, gitlab.PipelineFilter{Ref: "main", Status: "failed", Username: "egor"}, "CUR", 20)
	if err != nil {
		t.Fatal(err)
	}
	q, v := rec.queries[0], rec.vars[0]
	for _, want := range []string{"ref: $ref", "status: $status", "username: $username", "after: $after", "first: $first"} {
		if !strings.Contains(q, want) {
			t.Errorf("query lacks %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "source: $source") {
		t.Error("unset filters must not be sent")
	}
	if v["status"] != "FAILED" || v["after"] != "CUR" {
		t.Errorf("vars = %v", v)
	}
	if page.Total != 812 || page.NextCursor != "PC1" || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	p := page.Items[0]
	if p.ID != 91840 || p.IID != 512 || p.Status != gitlab.StatusRunning || p.Source != "merge_request_event" || p.CommitTitle != "surge: add pricing" || p.Duration != 0 {
		t.Errorf("pipeline = %+v", p)
	}
	f := page.Items[1]
	if f.Status != gitlab.StatusFailed || f.Duration != 340*time.Second || f.FinishedAt == nil || f.User != nil {
		t.Errorf("failed pipeline = %+v", f)
	}
}

func TestMRPipelines(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string { return fixture(t, "mr_pipelines.json") })
	ps, err := c.MRPipelines(context.Background(), testProject, 479, 20)
	if err != nil {
		t.Fatal(err)
	}
	if v := rec.vars[0]; v["iid"] != "479" || v["first"] != float64(20) {
		t.Errorf("vars = %v", v)
	}
	if len(ps) != 2 {
		t.Fatalf("pipelines = %+v", ps)
	}
	p := ps[0]
	if p.ID != 91840 || p.Status != gitlab.StatusRunning || p.User == nil || p.User.Username != "anna.k" || p.CommitTitle != "surge: add pricing" {
		t.Errorf("pipeline = %+v", p)
	}
	if f := ps[1]; f.Status != gitlab.StatusFailed || f.Duration != 215*time.Second || f.CommitTitle != "surge: draft" {
		t.Errorf("failed pipeline = %+v", f)
	}
}

func TestMRPipelinesNotFound(t *testing.T) {
	c, _ := newRecorder(t, func(int, string, map[string]any) string {
		return `{"data":{"project":{"mergeRequest":null}}}`
	})
	if _, err := c.MRPipelines(context.Background(), testProject, 1, 20); !errors.Is(err, gitlab.ErrNotFound) {
		t.Errorf("err = %v, want not found", err)
	}
}

func TestPipelineStages(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string { return fixture(t, "pipeline_stages.json") })
	st, err := c.PipelineStages(context.Background(), testProject, 512)
	if err != nil {
		t.Fatal(err)
	}
	if rec.vars[0]["iid"] != "512" {
		t.Errorf("vars = %v", rec.vars[0])
	}
	want := []gitlab.StageSummary{
		{Name: "build", Status: gitlab.StatusSuccess, Done: 2, Total: 2, Duration: 110 * time.Second},
		{Name: "test", Status: gitlab.StatusRunning, Done: 2, Total: 3},
		{Name: "deploy", Status: gitlab.StatusCreated, Done: 0, Total: 1},
	}
	if len(st) != 3 {
		t.Fatalf("stages = %+v", st)
	}
	for i := range want {
		if st[i] != want[i] {
			t.Errorf("stage %d = %+v, want %+v", i, st[i], want[i])
		}
	}
}

func TestCIConfigVariables(t *testing.T) {
	calls := 0
	c, _ := newRecorder(t, func(int, string, map[string]any) string {
		calls++
		if calls == 1 {
			return `{"data":{"project":{"ciConfigVariables":null}}}`
		}
		return `{"data":{"project":{"ciConfigVariables":[
			{"key":"DEPLOY_ENV","value":"staging","valueOptions":["staging","prod-canary"],"description":"where"},
			{"key":"SKIP_E2E","value":"","valueOptions":null,"description":null}]}}}`
	})
	_, err := c.CIConfigVariables(context.Background(), testProject, "main")
	if !errors.Is(err, gitlab.ErrNotReady) {
		t.Fatalf("null must mean not ready, got %v", err)
	}
	vars, err := c.CIConfigVariables(context.Background(), testProject, "main")
	if err != nil || len(vars) != 2 {
		t.Fatalf("%+v %v", vars, err)
	}
	if vars[0].Key != "DEPLOY_ENV" || len(vars[0].ValueOptions) != 2 || vars[0].Description != "where" || vars[1].ValueOptions != nil {
		t.Errorf("vars = %+v", vars)
	}
}
