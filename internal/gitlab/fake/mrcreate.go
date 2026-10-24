package fake

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/4aykovski/glview/internal/gitlab"
)

type mrTemplate struct{ name, content string }

// SetMRTemplates registers the MR description templates of a project as
// name, content pairs.
func (c *Client) SetMRTemplates(path string, pairs ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mrTemplates == nil {
		c.mrTemplates = map[string][]mrTemplate{}
	}
	var list []mrTemplate
	for i := 0; i+1 < len(pairs); i += 2 {
		list = append(list, mrTemplate{name: pairs[i], content: pairs[i+1]})
	}
	c.mrTemplates[path] = list
}

func (s projects) MRTemplates(ctx context.Context, p gitlab.ProjectRef) ([]string, error) {
	if err := s.c.enter(ctx, "Projects.MRTemplates", p.FullPath); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	var out []string
	for _, t := range s.c.mrTemplates[s.c.pathOf(p)] {
		out = append(out, t.name)
	}
	return out, nil
}

func (s projects) MRTemplate(ctx context.Context, p gitlab.ProjectRef, name string) (string, error) {
	if err := s.c.enter(ctx, "Projects.MRTemplate", p.FullPath, name); err != nil {
		return "", err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	for _, t := range s.c.mrTemplates[s.c.pathOf(p)] {
		if t.name == name {
			return t.content, nil
		}
	}
	return "", notFound("template " + name)
}

// Create adds an opened MR by the fake user to the project's list, so List
// and Get see it; the next free IID follows the highest one registered.
// Members, labels and milestones of the project resolve the IDs and names.
func (s mrService) Create(ctx context.Context, p gitlab.ProjectRef, n gitlab.NewMR) (gitlab.MergeRequest, error) {
	if err := s.c.enter(ctx, "MergeRequests.Create", p.FullPath, n); err != nil {
		return gitlab.MergeRequest{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	var iid int64
	for _, mr := range s.c.data.mrs[path] {
		iid = max(iid, mr.IID)
	}
	iid++
	people := func(ids []int64) []gitlab.User {
		var out []gitlab.User
		for _, id := range ids {
			if s.c.User.ID == id {
				out = append(out, s.c.User)
				continue
			}
			for _, u := range s.c.data.members[path] {
				if u.ID == id {
					out = append(out, u)
				}
			}
		}
		return out
	}
	mr := gitlab.MergeRequest{
		Project: gitlab.ProjectRef{ID: p.ID, FullPath: path}, IID: iid, Title: n.Title, Description: n.Description,
		State: gitlab.MRStateOpened, Draft: strings.HasPrefix(strings.ToLower(n.Title), "draft:"), Author: s.c.User,
		Assignees: people(n.AssigneeIDs), Reviewers: people(n.ReviewerIDs),
		SourceBranch: n.SourceBranch, TargetBranch: n.TargetBranch, Labels: slices.Clone(n.Labels),
		MergeStatus: "checking", Squash: n.Squash, ForceRemoveSourceBranch: n.RemoveSourceBranch,
		WebURL:    s.c.BaseURL + "/" + path + "/-/merge_requests/" + strconv.FormatInt(iid, 10),
		CreatedAt: s.c.now(), UpdatedAt: s.c.now(),
	}
	s.c.data.mrs[path] = append(s.c.data.mrs[path], mr)
	return mr, nil
}
