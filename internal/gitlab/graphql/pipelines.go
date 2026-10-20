package graphql

import (
	"context"
	"fmt"
	"strings"

	"github.com/4aykovski/glview/internal/gitlab"
)

// ProjectPipelines lists pipelines of the project, newest first.
func (c *Client) ProjectPipelines(ctx context.Context, project gitlab.ProjectRef, f gitlab.PipelineFilter, cursor string, limit int) (gitlab.Page[gitlab.PipelineSummary], error) {
	if limit <= 0 {
		limit = 20
	}
	vars := map[string]any{"path": project.FullPath, "first": limit}
	decls := []string{"$path: ID!", "$first: Int!"}
	var args []string
	add := func(name, typ, gqlArg string, v any) {
		decls = append(decls, "$"+name+": "+typ)
		vars[name] = v
		args = append(args, gqlArg+": $"+name)
	}
	if f.Ref != "" {
		add("ref", "String", "ref", f.Ref)
	}
	if f.Status != "" {
		add("status", "PipelineStatusEnum", "status", strings.ToUpper(f.Status))
	}
	if f.Source != "" {
		add("source", "String", "source", f.Source)
	}
	if f.Username != "" {
		add("username", "String", "username", f.Username)
	}
	switch f.Scope {
	case "tags":
		add("scope", "PipelineScopeEnum", "scope", "TAGS")
	case "branches":
		add("scope", "PipelineScopeEnum", "scope", "BRANCHES")
	}
	if cursor != "" {
		add("after", "String", "after", cursor)
	}
	args = append(args, "first: $first")

	query := fmt.Sprintf(`query ProjectPipelines(%s) {
  project(fullPath: $path) {
    pipelines(%s) { count pageInfo { hasNextPage endCursor } nodes { ...PipelineRow } }
  }
}
%s`, strings.Join(decls, ", "), strings.Join(args, ", "), pipelineRowFragment)

	var data struct {
		Project *struct {
			Pipelines struct {
				Count    int `json:"count"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []gqlPipeline `json:"nodes"`
			} `json:"pipelines"`
		} `json:"project"`
	}
	if err := c.DoOpt(ctx, query, vars, &data); err != nil {
		return gitlab.Page[gitlab.PipelineSummary]{}, err
	}
	if data.Project == nil {
		return gitlab.Page[gitlab.PipelineSummary]{}, projectNotFound(project)
	}
	conn := data.Project.Pipelines
	page := gitlab.Page[gitlab.PipelineSummary]{Total: conn.Count}
	origin := c.origin()
	for _, n := range conn.Nodes {
		page.Items = append(page.Items, n.summary(project, origin))
	}
	if conn.PageInfo.HasNextPage {
		page.NextCursor = conn.PageInfo.EndCursor
	}
	return page, nil
}

const mrPipelinesQuery = `query MRPipelines($path: ID!, $iid: String!, $first: Int!) {
  project(fullPath: $path) {
    mergeRequest(iid: $iid) {
      pipelines(first: $first) { nodes { ...PipelineRow } }
    }
  }
}
` + pipelineRowFragment

// MRPipelines returns the latest pipelines of a merge request, newest first.
// Unlike the REST endpoint it carries user, duration and commit title.
func (c *Client) MRPipelines(ctx context.Context, project gitlab.ProjectRef, iid int64, limit int) ([]gitlab.PipelineSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	var data struct {
		Project *struct {
			MergeRequest *struct {
				Pipelines struct {
					Nodes []gqlPipeline `json:"nodes"`
				} `json:"pipelines"`
			} `json:"mergeRequest"`
		} `json:"project"`
	}
	vars := map[string]any{"path": project.FullPath, "iid": fmt.Sprint(iid), "first": limit}
	if err := c.DoOpt(ctx, mrPipelinesQuery, vars, &data); err != nil {
		return nil, err
	}
	if data.Project == nil {
		return nil, projectNotFound(project)
	}
	if data.Project.MergeRequest == nil {
		return nil, &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404, Message: fmt.Sprintf("merge request !%d", iid)}
	}
	origin := c.origin()
	nodes := data.Project.MergeRequest.Pipelines.Nodes
	out := make([]gitlab.PipelineSummary, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.summary(project, origin))
	}
	return out, nil
}

func projectNotFound(project gitlab.ProjectRef) error {
	return &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404, Message: "project " + project.FullPath}
}

const pipelineStagesQuery = `query PipelineStages($path: ID!, $iid: ID!) {
  project(fullPath: $path) {
    pipeline(iid: $iid) {
      stages { nodes { name detailedStatus { group } jobs { nodes { status duration } } } }
    }
  }
}`

// PipelineStages returns per-stage progress of the pipeline with the given
// project-level IID (list preview).
func (c *Client) PipelineStages(ctx context.Context, project gitlab.ProjectRef, iid int64) ([]gitlab.StageSummary, error) {
	var data struct {
		Project *struct {
			Pipeline *struct {
				Stages struct {
					Nodes []gqlStage `json:"nodes"`
				} `json:"stages"`
			} `json:"pipeline"`
		} `json:"project"`
	}
	vars := map[string]any{"path": project.FullPath, "iid": fmt.Sprint(iid)}
	if err := c.Do(ctx, pipelineStagesQuery, vars, &data); err != nil {
		return nil, err
	}
	if data.Project == nil || data.Project.Pipeline == nil {
		return nil, &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404, Message: fmt.Sprintf("pipeline iid %d", iid)}
	}
	return stagesOf(data.Project.Pipeline.Stages.Nodes), nil
}

const ciConfigVariablesQuery = `query CIConfigVariables($path: ID!, $ref: String!) {
  project(fullPath: $path) {
    ciConfigVariables(ref: $ref) { key value valueOptions description }
  }
}`

// CIConfigVariables returns the variable hints from .gitlab-ci.yml for ref.
// GitLab computes them asynchronously and answers null until they are
// ready: that is reported as gitlab.ErrNotReady.
func (c *Client) CIConfigVariables(ctx context.Context, project gitlab.ProjectRef, gitRef string) ([]gitlab.CIConfigVariable, error) {
	var data struct {
		Project *struct {
			CIConfigVariables []struct {
				Key          string   `json:"key"`
				Value        string   `json:"value"`
				ValueOptions []string `json:"valueOptions"`
				Description  string   `json:"description"`
			} `json:"ciConfigVariables"`
		} `json:"project"`
	}
	vars := map[string]any{"path": project.FullPath, "ref": gitRef}
	if err := c.Do(ctx, ciConfigVariablesQuery, vars, &data); err != nil {
		return nil, err
	}
	if data.Project == nil {
		return nil, projectNotFound(project)
	}
	if data.Project.CIConfigVariables == nil {
		return nil, gitlab.ErrNotReady
	}
	out := make([]gitlab.CIConfigVariable, len(data.Project.CIConfigVariables))
	for i, v := range data.Project.CIConfigVariables {
		out[i] = gitlab.CIConfigVariable{Key: v.Key, Value: v.Value, ValueOptions: v.ValueOptions, Description: v.Description}
	}
	return out, nil
}
