package rest

import (
	"context"
	"sync"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
)

// maxPerPage is GitLab's page size limit.
const maxPerPage = 100

// Starred returns up to 100 starred projects of the current user.
func (s projects) Starred(ctx context.Context) ([]gitlab.Project, error) {
	opt := &gl.ListProjectsOptions{
		ListOptions: gl.ListOptions{PerPage: maxPerPage},
		Starred:     gl.Ptr(true),
		Simple:      gl.Ptr(true),
	}
	list, _, err := s.c.api.Projects.ListProjects(opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]gitlab.Project, len(list))
	for i, p := range list {
		out[i] = mapProject(p)
		out[i].Starred = true
	}
	return out, nil
}

// ListGroupProjects returns up to limit non-archived projects of the group
// and its subgroups, most active first, and the total count (X-Total);
// -1 when GitLab sent no X-Total and there are more projects than taken.
func (s projects) ListGroupProjects(ctx context.Context, groupID int64, limit int) ([]gitlab.Project, int, error) {
	if limit <= 0 {
		limit = maxPerPage
	}
	opt := &gl.ListGroupProjectsOptions{
		ListOptions:      gl.ListOptions{PerPage: int64(min(limit, maxPerPage)), Page: 1},
		IncludeSubGroups: gl.Ptr(true),
		Archived:         gl.Ptr(false),
		OrderBy:          gl.Ptr("last_activity_at"),
		Sort:             gl.Ptr("desc"),
		Simple:           gl.Ptr(true),
	}
	var out []gitlab.Project
	total := -1
	more := false // projects beyond what was taken
	for len(out) < limit {
		list, resp, err := s.c.api.Groups.ListGroupProjects(groupID, opt, gl.WithContext(ctx))
		if err != nil {
			return nil, 0, mapError(err)
		}
		if resp.TotalItems > 0 {
			total = int(resp.TotalItems)
		}
		for _, p := range list {
			if len(out) == limit {
				more = true
				break
			}
			out = append(out, mapProject(p))
		}
		if resp.NextPage == 0 || len(list) == 0 {
			break
		}
		more = true
		opt.Page = resp.NextPage
	}
	switch {
	case total < 0 && more:
		// GitLab omits X-Total above 10 000 rows: the total is unknown.
	case total < len(out):
		total = len(out)
	}
	return out, total, nil
}

// LatestTag returns the most recently updated tag of the project, "" when
// it has none (the workspace Tag column).
func (c *Client) LatestTag(ctx context.Context, p gitlab.ProjectRef) (string, error) {
	opt := &gl.ListTagsOptions{ListOptions: gl.ListOptions{PerPage: 1}, OrderBy: gl.Ptr("updated")}
	list, _, err := c.api.Tags.ListTags(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return "", mapError(err)
	}
	if len(list) == 0 {
		return "", nil
	}
	return list[0].Name, nil
}

// Search is the REST fallback of the global search: projects matching q
// (namespace included, most active first, up to 10) and groups (up to 5).
func (c *Client) Search() gitlab.SearchService { return search{c} }

type search struct{ c *Client }

func (s search) Search(ctx context.Context, q string) ([]gitlab.Project, []gitlab.Group, error) {
	popt := &gl.ListProjectsOptions{
		ListOptions:      gl.ListOptions{PerPage: 10},
		Search:           gl.Ptr(q),
		SearchNamespaces: gl.Ptr(true),
		OrderBy:          gl.Ptr("last_activity_at"),
		Simple:           gl.Ptr(true),
	}
	list, _, err := s.c.api.Projects.ListProjects(popt, gl.WithContext(ctx))
	if err != nil {
		return nil, nil, mapError(err)
	}
	projects := make([]gitlab.Project, len(list))
	for i, p := range list {
		projects[i] = mapProject(p)
	}
	gopt := &gl.ListGroupsOptions{ListOptions: gl.ListOptions{PerPage: 5}, Search: gl.Ptr(q)}
	glist, _, err := s.c.api.Groups.ListGroups(gopt, gl.WithContext(ctx))
	if err != nil {
		return nil, nil, mapError(err)
	}
	groups := make([]gitlab.Group, len(glist))
	for i, g := range glist {
		groups[i] = gitlab.Group{ID: g.ID, FullPath: g.FullPath, Name: g.Name, WebURL: g.WebURL}
	}
	return projects, groups, nil
}

// Workspace batches need GraphQL; the REST adapter alone reports every
// project with errGraphQLOnly.
func (c *Client) Workspace() gitlab.WorkspaceService { return workspace{c} }

type workspace struct{ c *Client }

func failAll[T any](ps []gitlab.ProjectRef) <-chan gitlab.BatchResult[T] {
	ch := make(chan gitlab.BatchResult[T], 1)
	res := gitlab.BatchResult[T]{Err: errGraphQLOnly}
	for _, p := range ps {
		res.Groups = append(res.Groups, gitlab.ProjectGroup[T]{Project: p, Total: -1, Err: errGraphQLOnly})
	}
	ch <- res
	close(ch)
	return ch
}

func (workspace) ProjectsOverview(_ context.Context, ps []gitlab.ProjectRef, _ gitlab.OverviewOptions) <-chan gitlab.BatchResult[gitlab.ProjectOverview] {
	return failAll[gitlab.ProjectOverview](ps)
}

func (workspace) MergeRequests(_ context.Context, ps []gitlab.ProjectRef, _ gitlab.MRFilter, _ int) <-chan gitlab.BatchResult[gitlab.MergeRequest] {
	return failAll[gitlab.MergeRequest](ps)
}

func (workspace) Pipelines(_ context.Context, ps []gitlab.ProjectRef, _ gitlab.PipelineFilter, _ int) <-chan gitlab.BatchResult[gitlab.PipelineSummary] {
	return failAll[gitlab.PipelineSummary](ps)
}

// tagParallel bounds the projects whose tags are requested at once.
const tagParallel = 4

// tagPipelineWindow is how many of the newest tag pipelines are matched to
// the tags of a workspace group (SPEC §5.2).
const tagPipelineWindow = 50

// Tags lists the tags of every project over REST, at most four projects at
// a time; every project is reported as soon as it is ready.
func (w workspace) Tags(ctx context.Context, ps []gitlab.ProjectRef, search string, perProject int) <-chan gitlab.BatchResult[gitlab.Tag] {
	out := make(chan gitlab.BatchResult[gitlab.Tag])
	sem := make(chan struct{}, tagParallel)
	var wg sync.WaitGroup
	for _, p := range ps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			g := w.projectTags(ctx, p, search, perProject)
			<-sem
			select {
			case out <- gitlab.BatchResult[gitlab.Tag]{Groups: []gitlab.ProjectGroup[gitlab.Tag]{g}}:
			case <-ctx.Done():
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func (w workspace) projectTags(ctx context.Context, p gitlab.ProjectRef, search string, perProject int) gitlab.ProjectGroup[gitlab.Tag] {
	svc := tags(w)
	page, err := svc.List(ctx, p, search, "", perProject)
	if err != nil {
		return gitlab.ProjectGroup[gitlab.Tag]{Project: p, Total: -1, Err: err}
	}
	g := gitlab.ProjectGroup[gitlab.Tag]{Project: p, Items: page.Items, Total: page.Total}
	if g.Total >= 0 && g.Total < len(g.Items) {
		g.Total = len(g.Items)
	}
	if len(g.Items) > 0 {
		pipes, err := svc.Pipelines(ctx, p, tagPipelineWindow)
		if err != nil {
			debuglog.L().Debug("workspace tags: pipelines failed", "project", p.FullPath, "err", err)
		}
		gitlab.MatchTagPipelines(g.Items, pipes)
	}
	return g
}
