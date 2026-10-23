package rest

import (
	"context"
	"slices"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
)

type issues struct{ c *Client }

func (c *Client) Issues() gitlab.IssueService { return issues{c} }

// List returns one page of project issues, most recently updated first.
func (s issues) List(ctx context.Context, p gitlab.ProjectRef, f gitlab.IssueFilter, cursor string, limit int) (gitlab.Page[gitlab.Issue], error) {
	if limit <= 0 {
		limit = refListSize
	}
	opt := &gl.ListProjectIssuesOptions{
		ListOptions: gl.ListOptions{PerPage: int64(limit)},
		OrderBy:     gl.Ptr("updated_at"),
		Sort:        gl.Ptr("desc"),
	}
	switch f.State {
	case "", "opened":
		opt.State = gl.Ptr("opened")
	case "all":
		// GitLab's default is all states: the parameter is left out.
	default:
		opt.State = gl.Ptr(f.State)
	}
	switch f.Assignee {
	case "":
	case "@me":
		opt.Scope = gl.Ptr("assigned_to_me")
	case "none":
		opt.AssigneeID = gl.AssigneeID(gl.UserIDNone)
	default:
		opt.AssigneeUsername = gl.Ptr(f.Assignee)
	}
	switch f.Milestone {
	case "":
	case "none":
		opt.Milestone = gl.Ptr("None")
	default:
		opt.Milestone = gl.Ptr(f.Milestone)
	}
	if len(f.Labels) > 0 {
		lo := gl.LabelOptions(f.Labels)
		opt.Labels = &lo
	}
	if f.Search != "" {
		opt.Search = gl.Ptr(f.Search)
	}
	if n, err := strconv.ParseInt(cursor, 10, 64); err == nil {
		opt.Page = n
	}
	list, resp, err := s.c.api.Issues.ListProjectIssues(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Page[gitlab.Issue]{}, mapError(err)
	}
	page := gitlab.Page[gitlab.Issue]{Total: -1}
	if resp.TotalItems > 0 {
		page.Total = int(resp.TotalItems)
	}
	if resp.NextPage > 0 {
		page.NextCursor = strconv.FormatInt(resp.NextPage, 10)
	}
	for _, i := range list {
		page.Items = append(page.Items, mapIssue(p, i))
	}
	return page, nil
}

func (s issues) Get(ctx context.Context, p gitlab.ProjectRef, iid int64) (gitlab.Issue, error) {
	i, _, err := s.c.api.Issues.GetIssue(pid(p), iid, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Issue{}, mapError(err)
	}
	return mapIssue(p, i), nil
}

// Notes returns the last `last` notes, oldest first.
func (s issues) Notes(ctx context.Context, p gitlab.ProjectRef, iid int64, last int) ([]gitlab.Note, error) {
	if last <= 0 {
		last = 50
	}
	opt := &gl.ListIssueNotesOptions{
		ListOptions: gl.ListOptions{PerPage: int64(min(last, 100))},
		OrderBy:     gl.Ptr("created_at"),
		Sort:        gl.Ptr("desc"),
	}
	list, _, err := s.c.api.Notes.ListIssueNotes(pid(p), iid, opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]gitlab.Note, 0, len(list))
	for _, n := range list {
		out = append(out, mapNote(n))
	}
	slices.Reverse(out)
	return out, nil
}

func (s issues) Discussions(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.Discussion, error) {
	var out []gitlab.Discussion
	opt := &gl.ListIssueDiscussionsOptions{ListOptions: gl.ListOptions{PerPage: 100, Page: 1}}
	for {
		page, resp, err := s.c.api.Discussions.ListIssueDiscussions(pid(p), iid, opt, gl.WithContext(ctx))
		if err != nil {
			return nil, mapError(err)
		}
		for _, d := range page {
			out = append(out, mapDiscussion(d))
		}
		if resp.NextPage == 0 {
			return out, nil
		}
		opt.Page = resp.NextPage
	}
}

func (s issues) RelatedMRs(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.RelatedMR, error) {
	opt := &gl.ListMergeRequestsRelatedToIssueOptions{ListOptions: gl.ListOptions{PerPage: 20}}
	list, _, err := s.c.api.Issues.ListMergeRequestsRelatedToIssue(pid(p), iid, opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]gitlab.RelatedMR, len(list))
	for i, m := range list {
		out[i] = gitlab.RelatedMR{IID: m.IID, Title: m.Title, State: gitlab.MRState(m.State), WebURL: m.WebURL}
	}
	return out, nil
}

func (s issues) Comment(ctx context.Context, p gitlab.ProjectRef, iid int64, body string) (gitlab.Note, error) {
	n, _, err := s.c.api.Notes.CreateIssueNote(pid(p), iid, &gl.CreateIssueNoteOptions{Body: &body}, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Note{}, mapError(err)
	}
	return mapNote(n), nil
}

func (s issues) SetState(ctx context.Context, p gitlab.ProjectRef, iid int64, closed bool) (gitlab.Issue, error) {
	event := "reopen"
	if closed {
		event = "close"
	}
	i, _, err := s.c.api.Issues.UpdateIssue(pid(p), iid, &gl.UpdateIssueOptions{StateEvent: &event}, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Issue{}, mapError(err)
	}
	return mapIssue(p, i), nil
}

func mapNote(n *gl.Note) gitlab.Note {
	return gitlab.Note{
		ID: n.ID, Body: n.Body, System: n.System, CreatedAt: deref(n.CreatedAt),
		Author: gitlab.User{ID: n.Author.ID, Username: n.Author.Username, Name: n.Author.Name, WebURL: n.Author.WebURL},
	}
}

func mapIssue(p gitlab.ProjectRef, i *gl.Issue) gitlab.Issue {
	out := gitlab.Issue{
		Project: p, IID: i.IID, Title: i.Title, Description: i.Description, State: i.State,
		Labels: []string(i.Labels), UserNotesCount: int(i.UserNotesCount), MergeRequestsCount: int(i.MergeRequestCount),
		WebURL: i.WebURL, CreatedAt: deref(i.CreatedAt), UpdatedAt: deref(i.UpdatedAt),
	}
	if i.Author != nil {
		out.Author = gitlab.User{ID: i.Author.ID, Username: i.Author.Username, Name: i.Author.Name, WebURL: i.Author.WebURL}
	}
	for _, a := range i.Assignees {
		out.Assignees = append(out.Assignees, gitlab.User{ID: a.ID, Username: a.Username, Name: a.Name, WebURL: a.WebURL})
	}
	if i.Milestone != nil {
		out.Milestone = i.Milestone.Title
	}
	return out
}
