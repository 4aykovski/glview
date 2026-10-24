package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestMRCreateBody(t *testing.T) {
	var body map[string]any
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/platform%2Froute-service/merge_requests": func(w http.ResponseWriter, r *http.Request) {
			_ = json.Unmarshal([]byte(readBody(r)), &body)
			jsonReply(201, `{"iid":483,"title":"Draft: Fix slot race","state":"opened","source_branch":"feat/surge",
			 "target_branch":"main","web_url":"https://gitlab.example.com/platform/route-service/-/merge_requests/483","draft":true}`)(w, r)
		},
	})
	mr, err := c.MergeRequests().Create(context.Background(), issueProject, gitlab.NewMR{
		SourceBranch: "feat/surge", TargetBranch: "main", Title: "Draft: Fix slot race", Description: "Body",
		AssigneeIDs: []int64{7}, ReviewerIDs: []int64{3, 9}, Labels: []string{"bug", "pricing"},
		MilestoneID: 31, RemoveSourceBranch: true, Squash: false,
	})
	if err != nil || mr.IID != 483 || mr.Title != "Draft: Fix slot race" || mr.SourceBranch != "feat/surge" {
		t.Fatalf("mr = %+v, err = %v", mr, err)
	}
	want := map[string]any{
		"source_branch": "feat/surge", "target_branch": "main", "title": "Draft: Fix slot race", "description": "Body",
		"assignee_ids": []any{float64(7)}, "reviewer_ids": []any{float64(3), float64(9)},
		"labels": "bug,pricing", "milestone_id": float64(31), "remove_source_branch": true, "squash": false,
	}
	for k, v := range want {
		got, _ := json.Marshal(body[k])
		exp, _ := json.Marshal(v)
		if string(got) != string(exp) {
			t.Errorf("%s = %s, want %s", k, got, exp)
		}
	}
}

func TestMRCreateOmitsEmptyOptionals(t *testing.T) {
	var raw string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/platform%2Froute-service/merge_requests": func(w http.ResponseWriter, r *http.Request) {
			raw = readBody(r)
			jsonReply(201, `{"iid":1}`)(w, r)
		},
	})
	_, err := c.MergeRequests().Create(context.Background(), issueProject, gitlab.NewMR{SourceBranch: "a", TargetBranch: "main", Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"assignee_ids", "reviewer_ids", "labels", "milestone_id", "description"} {
		if strings.Contains(raw, k) {
			t.Errorf("body %s carries %s", raw, k)
		}
	}
}

func TestMRCreateServerErrors(t *testing.T) {
	for _, tt := range []struct {
		status int
		kind   error
	}{{409, gitlab.ErrConflict}, {422, gitlab.ErrNotPossible}, {400, gitlab.ErrNotPossible}} {
		c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
			"POST /api/v4/projects/platform%2Froute-service/merge_requests": jsonReply(tt.status, `{"message":["Source branch does not exist"]}`),
		})
		_, err := c.MergeRequests().Create(context.Background(), issueProject, gitlab.NewMR{SourceBranch: "a", TargetBranch: "main", Title: "T"})
		var ge *gitlab.Error
		if !errors.Is(err, tt.kind) || !errors.As(err, &ge) || !strings.Contains(ge.Message, "Source branch does not exist") {
			t.Errorf("status %d: err = %v", tt.status, err)
		}
	}
}

func TestMRTemplates(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/platform%2Froute-service/templates/merge_requests":     jsonReply(200, `[{"key":"Default","name":"Default"},{"key":"Bug","name":"Bug"}]`),
		"GET /api/v4/projects/platform%2Froute-service/templates/merge_requests/Bug": jsonReply(200, `{"name":"Bug","content":"## Steps\n"}`),
	})
	names, err := c.Projects().MRTemplates(context.Background(), issueProject)
	if err != nil || len(names) != 2 || names[0] != "Default" || names[1] != "Bug" {
		t.Fatalf("names = %v, err = %v", names, err)
	}
	content, err := c.Projects().MRTemplate(context.Background(), issueProject, "Bug")
	if err != nil || content != "## Steps\n" {
		t.Fatalf("content = %q, err = %v", content, err)
	}
}

func TestProjectGetCreateDefaults(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/platform%2Froute-service": jsonReply(200, `{"id":42,"path_with_namespace":"platform/route-service",
		 "default_branch":"main","remove_source_branch_after_merge":true,"squash_option":"default_on"}`),
	})
	p, err := c.Projects().Get(context.Background(), issueProject)
	if err != nil || !p.RemoveSourceBranchAfterMerge || p.SquashOption != "default_on" {
		t.Fatalf("project = %+v, err = %v", p, err)
	}
}

func TestCompareFiles(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/repository/compare": jsonReply(200, `{"commits":[{"id":"a1","short_id":"a1","title":"x"}],
		 "diffs":[{"new_path":"a.go"},{"new_path":"b.go"},{"new_path":"c.go"}]}`),
	})
	cmp, err := c.Tags().Compare(context.Background(), gitlab.ProjectRef{ID: 42}, "main", "feat/surge")
	if err != nil || cmp.Files != 3 {
		t.Fatalf("cmp = %+v, err = %v", cmp, err)
	}
}
