package fake

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/4aykovski/glview/internal/gitlab"
)

func (c *Client) Issues() gitlab.IssueService { return issueService{c} }

func issueKey(path string, iid int64) string { return path + "#" + strconv.FormatInt(iid, 10) }

// AddIssue registers an issue of project path.
func (c *Client) AddIssue(path string, i gitlab.Issue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i.Project.FullPath == "" {
		i.Project.FullPath = path
	}
	if i.State == "" {
		i.State = "opened"
	}
	c.data.issues[path] = append(c.data.issues[path], i)
}

// UpdateIssue changes a registered issue in place.
func (c *Client) UpdateIssue(path string, iid int64, fn func(*gitlab.Issue)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.data.issues[path] {
		if c.data.issues[path][i].IID == iid {
			fn(&c.data.issues[path][i])
		}
	}
}

// SetIssueNotes replaces the notes of an issue (oldest first) and rebuilds
// its discussions as one-note threads.
func (c *Client) SetIssueNotes(path string, iid int64, notes ...gitlab.Note) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := issueKey(path, iid)
	c.data.inotes[k] = notes
	c.data.idiscs[k] = nil
	for _, n := range notes {
		c.data.idiscs[k] = append(c.data.idiscs[k], gitlab.Discussion{ID: fmt.Sprintf("n%d", n.ID), Notes: []gitlab.Note{n}})
	}
}

// SetIssueDiscussions replaces the discussions of an issue and flattens
// them into its notes.
func (c *Client) SetIssueDiscussions(path string, iid int64, ds []gitlab.Discussion) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := issueKey(path, iid)
	c.data.idiscs[k] = ds
	c.data.inotes[k] = nil
	for _, d := range ds {
		c.data.inotes[k] = append(c.data.inotes[k], d.Notes...)
	}
	sort.SliceStable(c.data.inotes[k], func(i, j int) bool { return c.data.inotes[k][i].CreatedAt.Before(c.data.inotes[k][j].CreatedAt) })
}

// SetRelatedMRs replaces the merge requests related to an issue; it does
// not touch Issue.MergeRequestsCount.
func (c *Client) SetRelatedMRs(path string, iid int64, mrs ...gitlab.RelatedMR) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.irelated[issueKey(path, iid)] = mrs
}

func (c *Client) findIssue(path string, iid int64) (*gitlab.Issue, error) {
	for i := range c.data.issues[path] {
		if c.data.issues[path][i].IID == iid {
			return &c.data.issues[path][i], nil
		}
	}
	return nil, notFound(fmt.Sprintf("issue #%d", iid))
}

type issueService struct{ c *Client }

func (s issueService) List(ctx context.Context, p gitlab.ProjectRef, f gitlab.IssueFilter, cursor string, limit int) (gitlab.Page[gitlab.Issue], error) {
	if err := s.c.enter(ctx, "Issues.List", p.FullPath, f, cursor, limit); err != nil {
		return gitlab.Page[gitlab.Issue]{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	if _, ok := s.c.ProjectsByPath[path]; !ok && len(s.c.data.issues[path]) == 0 {
		return gitlab.Page[gitlab.Issue]{}, notFound("project " + path)
	}
	var all []gitlab.Issue
	for _, i := range s.c.data.issues[path] {
		if matchIssue(i, f, s.c.User.Username) {
			all = append(all, i)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	return paginate(all, cursor, limit), nil
}

func matchIssue(i gitlab.Issue, f gitlab.IssueFilter, me string) bool {
	if f.State != "all" {
		want := f.State
		if want == "" {
			want = "opened"
		}
		if i.State != want {
			return false
		}
	}
	hasAssignee := func(name string) bool {
		return slices.ContainsFunc(i.Assignees, func(u gitlab.User) bool { return u.Username == name })
	}
	switch f.Assignee {
	case "":
	case "@me":
		if !hasAssignee(me) {
			return false
		}
	case "none":
		if len(i.Assignees) > 0 {
			return false
		}
	default:
		if !hasAssignee(f.Assignee) {
			return false
		}
	}
	switch f.Milestone {
	case "":
	case "none":
		if i.Milestone != "" {
			return false
		}
	default:
		if i.Milestone != f.Milestone {
			return false
		}
	}
	for _, l := range f.Labels {
		if !slices.Contains(i.Labels, l) {
			return false
		}
	}
	if f.Search != "" {
		q := strings.ToLower(f.Search)
		if !strings.Contains(strings.ToLower(i.Title), q) && !strings.Contains(strings.ToLower(i.Description), q) {
			return false
		}
	}
	return true
}

func (s issueService) Get(ctx context.Context, p gitlab.ProjectRef, iid int64) (gitlab.Issue, error) {
	if err := s.c.enter(ctx, "Issues.Get", p.FullPath, iid); err != nil {
		return gitlab.Issue{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	i, err := s.c.findIssue(s.c.pathOf(p), iid)
	if err != nil {
		return gitlab.Issue{}, err
	}
	return *i, nil
}

func (s issueService) Notes(ctx context.Context, p gitlab.ProjectRef, iid int64, last int) ([]gitlab.Note, error) {
	if err := s.c.enter(ctx, "Issues.Notes", p.FullPath, iid, last); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	if _, err := s.c.findIssue(path, iid); err != nil {
		return nil, err
	}
	notes := s.c.data.inotes[issueKey(path, iid)]
	if last > 0 && len(notes) > last {
		notes = notes[len(notes)-last:]
	}
	return slices.Clone(notes), nil
}

func (s issueService) Discussions(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.Discussion, error) {
	if err := s.c.enter(ctx, "Issues.Discussions", p.FullPath, iid); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	if _, err := s.c.findIssue(path, iid); err != nil {
		return nil, err
	}
	return slices.Clone(s.c.data.idiscs[issueKey(path, iid)]), nil
}

func (s issueService) RelatedMRs(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.RelatedMR, error) {
	if err := s.c.enter(ctx, "Issues.RelatedMRs", p.FullPath, iid); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return slices.Clone(s.c.data.irelated[issueKey(s.c.pathOf(p), iid)]), nil
}

func (s issueService) Comment(ctx context.Context, p gitlab.ProjectRef, iid int64, body string) (gitlab.Note, error) {
	if err := s.c.enter(ctx, "Issues.Comment", p.FullPath, iid, body); err != nil {
		return gitlab.Note{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	i, err := s.c.findIssue(path, iid)
	if err != nil {
		return gitlab.Note{}, err
	}
	n := s.c.newNote(body)
	k := issueKey(path, iid)
	s.c.data.inotes[k] = append(s.c.data.inotes[k], n)
	s.c.data.idiscs[k] = append(s.c.data.idiscs[k], gitlab.Discussion{ID: fmt.Sprintf("n%d", n.ID), Notes: []gitlab.Note{n}})
	i.UserNotesCount++
	i.UpdatedAt = s.c.now()
	return n, nil
}

func (s issueService) SetState(ctx context.Context, p gitlab.ProjectRef, iid int64, closed bool) (gitlab.Issue, error) {
	if err := s.c.enter(ctx, "Issues.SetState", p.FullPath, iid, closed); err != nil {
		return gitlab.Issue{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	i, err := s.c.findIssue(s.c.pathOf(p), iid)
	if err != nil {
		return gitlab.Issue{}, err
	}
	i.State = "opened"
	if closed {
		i.State = "closed"
	}
	i.UpdatedAt = s.c.now()
	return *i, nil
}
