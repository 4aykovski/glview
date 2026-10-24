package rest

import (
	"context"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
)

// refListSize bounds ref autocompletion results.
const refListSize = 20

func (s projects) Branches(ctx context.Context, p gitlab.ProjectRef, search string) ([]string, error) {
	opt := &gl.ListBranchesOptions{ListOptions: gl.ListOptions{PerPage: refListSize}}
	if search != "" {
		opt.Search = &search
	}
	list, _, err := s.c.api.Branches.ListBranches(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]string, len(list))
	for i, b := range list {
		out[i] = b.Name
	}
	return out, nil
}

// Milestones lists titles of the project's active milestones for filter
// suggestions.
func (s projects) Milestones(ctx context.Context, p gitlab.ProjectRef, search string) ([]string, error) {
	opt := &gl.ListMilestonesOptions{ListOptions: gl.ListOptions{PerPage: refListSize}, State: gl.Ptr("active")}
	if search != "" {
		opt.Search = &search
	}
	list, _, err := s.c.api.Milestones.ListMilestones(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]string, len(list))
	for i, m := range list {
		out[i] = m.Title
	}
	return out, nil
}

func (s projects) CIConfigVariables(context.Context, gitlab.ProjectRef, string) ([]gitlab.CIConfigVariable, error) {
	return nil, errGraphQLOnly
}

type tags struct{ c *Client }

func (s tags) List(ctx context.Context, p gitlab.ProjectRef, search, cursor string, limit int) (gitlab.Page[gitlab.Tag], error) {
	if limit <= 0 {
		limit = refListSize
	}
	opt := &gl.ListTagsOptions{
		ListOptions: gl.ListOptions{PerPage: int64(limit)},
		OrderBy:     gl.Ptr("version"),
		Sort:        gl.Ptr("desc"),
	}
	if search != "" {
		opt.Search = &search
	}
	if n, err := strconv.ParseInt(cursor, 10, 64); err == nil {
		opt.Page = n
	}
	list, resp, err := s.c.api.Tags.ListTags(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Page[gitlab.Tag]{}, mapError(err)
	}
	page := gitlab.Page[gitlab.Tag]{Total: totalOf(resp)}
	if resp.NextPage > 0 {
		page.NextCursor = strconv.FormatInt(resp.NextPage, 10)
	}
	for _, t := range list {
		page.Items = append(page.Items, mapTag(p, t))
	}
	return page, nil
}

func mapTag(p gitlab.ProjectRef, t *gl.Tag) gitlab.Tag {
	tag := gitlab.Tag{Project: p, Name: t.Name, Message: t.Message, Protected: t.Protected}
	if t.Commit != nil {
		tag.Commit = mapCommit(t.Commit)
	}
	if t.Release != nil {
		tag.Release = &gitlab.ReleaseSummary{Name: t.Release.TagName, Description: t.Release.Description}
	}
	return tag
}

// Pipelines returns the newest pipelines that ran for tags.
func (s tags) Pipelines(ctx context.Context, p gitlab.ProjectRef, limit int) ([]gitlab.PipelineSummary, error) {
	if limit <= 0 {
		limit = maxPerPage
	}
	opt := &gl.ListProjectPipelinesOptions{
		ListOptions: gl.ListOptions{PerPage: int64(min(limit, maxPerPage))},
		Scope:       gl.Ptr("tags"),
		OrderBy:     gl.Ptr("id"),
		Sort:        gl.Ptr("desc"),
	}
	list, _, err := s.c.api.Pipelines.ListProjectPipelines(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]gitlab.PipelineSummary, len(list))
	for i, pl := range list {
		out[i] = gitlab.PipelineSummary{
			Project: p, ID: pl.ID, IID: pl.IID, Status: gitlab.PipelineStatus(pl.Status), Ref: pl.Ref, SHA: pl.SHA,
			Source: pl.Source, CreatedAt: deref(pl.CreatedAt), WebURL: pl.WebURL,
		}
	}
	return out, nil
}

func (s tags) Create(ctx context.Context, p gitlab.ProjectRef, name, ref, message string) (gitlab.Tag, error) {
	opt := &gl.CreateTagOptions{TagName: &name, Ref: &ref}
	if message != "" {
		opt.Message = &message
	}
	t, _, err := s.c.api.Tags.CreateTag(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Tag{}, mapError(err)
	}
	return mapTag(p, t), nil
}

func (s tags) Delete(ctx context.Context, p gitlab.ProjectRef, name string) error {
	_, err := s.c.api.Tags.DeleteTag(pid(p), name, gl.WithContext(ctx))
	return mapError(err)
}

// Compare lists the commits from..to, newest first (GitLab sends them
// oldest first); Head is the last commit of "to".
func (s tags) Compare(ctx context.Context, p gitlab.ProjectRef, from, to string) (gitlab.Comparison, error) {
	cmp, _, err := s.c.api.Repositories.Compare(pid(p), &gl.CompareOptions{From: &from, To: &to}, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Comparison{}, mapError(err)
	}
	var out gitlab.Comparison
	if cmp.Commit != nil {
		head := mapCommit(cmp.Commit)
		out.Head = &head
	}
	for i := len(cmp.Commits) - 1; i >= 0; i-- {
		out.Commits = append(out.Commits, mapCommit(cmp.Commits[i]))
	}
	return out, nil
}

// Members lists project members for filter suggestions.
func (s projects) Members(ctx context.Context, p gitlab.ProjectRef, search string) ([]gitlab.User, error) {
	opt := &gl.ListProjectMembersOptions{ListOptions: gl.ListOptions{PerPage: refListSize}}
	if search != "" {
		opt.Query = &search
	}
	list, _, err := s.c.api.ProjectMembers.ListAllProjectMembers(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]gitlab.User, len(list))
	for i, m := range list {
		out[i] = gitlab.User{ID: m.ID, Username: m.Username, Name: m.Name}
	}
	return out, nil
}

// Labels lists project label names for filter suggestions.
func (s projects) Labels(ctx context.Context, p gitlab.ProjectRef, search string) ([]string, error) {
	opt := &gl.ListLabelsOptions{ListOptions: gl.ListOptions{PerPage: refListSize}}
	if search != "" {
		opt.Search = &search
	}
	list, _, err := s.c.api.Labels.ListLabels(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]string, len(list))
	for i, l := range list {
		out[i] = l.Name
	}
	return out, nil
}
