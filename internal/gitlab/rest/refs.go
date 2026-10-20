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

func (s projects) CIConfigVariables(context.Context, gitlab.ProjectRef, string) ([]gitlab.CIConfigVariable, error) {
	return nil, errGraphQLOnly
}

type tags struct{ c *Client }

func (s tags) List(ctx context.Context, p gitlab.ProjectRef, search, cursor string, limit int) (gitlab.Page[gitlab.Tag], error) {
	if limit <= 0 {
		limit = refListSize
	}
	opt := &gl.ListTagsOptions{ListOptions: gl.ListOptions{PerPage: int64(limit)}}
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
	page := gitlab.Page[gitlab.Tag]{Total: -1}
	if resp.TotalItems > 0 {
		page.Total = int(resp.TotalItems)
	}
	if resp.NextPage > 0 {
		page.NextCursor = strconv.FormatInt(resp.NextPage, 10)
	}
	for _, t := range list {
		tag := gitlab.Tag{Project: p, Name: t.Name, Message: t.Message, Protected: t.Protected}
		if t.Commit != nil {
			tag.Commit = mapCommit(t.Commit)
		}
		page.Items = append(page.Items, tag)
	}
	return page, nil
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
