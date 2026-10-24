package rest

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Create opens an MR in p. The response is the minimal mapped MR (mapMR);
// the caller reads details through Get.
func (s mrs) Create(ctx context.Context, p gitlab.ProjectRef, n gitlab.NewMR) (gitlab.MergeRequest, error) {
	opt := &gl.CreateMergeRequestOptions{
		SourceBranch:       &n.SourceBranch,
		TargetBranch:       &n.TargetBranch,
		Title:              &n.Title,
		RemoveSourceBranch: &n.RemoveSourceBranch,
		Squash:             &n.Squash,
	}
	if n.Description != "" {
		opt.Description = &n.Description
	}
	if len(n.AssigneeIDs) > 0 {
		opt.AssigneeIDs = &n.AssigneeIDs
	}
	if len(n.ReviewerIDs) > 0 {
		opt.ReviewerIDs = &n.ReviewerIDs
	}
	if len(n.Labels) > 0 {
		labels := gl.LabelOptions(n.Labels)
		opt.Labels = &labels
	}
	if n.MilestoneID != 0 {
		opt.MilestoneID = &n.MilestoneID
	}
	mr, _, err := s.c.api.MergeRequests.CreateMergeRequest(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return gitlab.MergeRequest{}, mapError(err)
	}
	return mapMR(p, mr), nil
}

// MRTemplates lists the names of the project's MR description templates.
func (s projects) MRTemplates(ctx context.Context, p gitlab.ProjectRef) ([]string, error) {
	opt := &gl.ListProjectTemplatesOptions{ListOptions: gl.ListOptions{PerPage: maxPerPage}}
	list, _, err := s.c.api.ProjectTemplates.ListTemplates(pid(p), "merge_requests", opt, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]string, len(list))
	for i, t := range list {
		out[i] = t.Name
	}
	return out, nil
}

// MRTemplate returns the content of the named MR description template.
func (s projects) MRTemplate(ctx context.Context, p gitlab.ProjectRef, name string) (string, error) {
	t, _, err := s.c.api.ProjectTemplates.GetProjectTemplate(pid(p), "merge_requests", name, gl.WithContext(ctx))
	if err != nil {
		return "", mapError(err)
	}
	return t.Content, nil
}
