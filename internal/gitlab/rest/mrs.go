package rest

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Merge requests: list, details and pipelines come from GraphQL (see package
// hybrid); this file holds the REST part — discussions, commits and
// mutations.
type mrs struct{ c *Client }

func (mrs) List(context.Context, gitlab.ProjectRef, gitlab.MRFilter, string, int) (gitlab.Page[gitlab.MergeRequest], error) {
	return gitlab.Page[gitlab.MergeRequest]{}, errGraphQLOnly
}

func (mrs) Get(context.Context, gitlab.ProjectRef, int64) (gitlab.MergeRequest, error) {
	return gitlab.MergeRequest{}, errGraphQLOnly
}

// Discussions returns every discussion of the MR (all pages).
func (s mrs) Discussions(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.Discussion, error) {
	var out []gitlab.Discussion
	opt := &gl.ListMergeRequestDiscussionsOptions{ListOptions: gl.ListOptions{PerPage: 100, Page: 1}}
	for {
		page, resp, err := s.c.api.Discussions.ListMergeRequestDiscussions(pid(p), iid, opt, gl.WithContext(ctx))
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

func mapDiscussion(d *gl.Discussion) gitlab.Discussion {
	out := gitlab.Discussion{ID: d.ID}
	for i, n := range d.Notes {
		note := gitlab.Note{
			ID: n.ID, Body: n.Body, System: n.System, CreatedAt: deref(n.CreatedAt),
			Author: gitlab.User{ID: n.Author.ID, Username: n.Author.Username, Name: n.Author.Name, WebURL: n.Author.WebURL},
		}
		out.Notes = append(out.Notes, note)
		if i == 0 {
			out.Resolvable, out.Resolved = n.Resolvable, n.Resolved
			if n.Position != nil {
				path, line := n.Position.NewPath, n.Position.NewLine
				if path == "" {
					path, line = n.Position.OldPath, n.Position.OldLine
				}
				if path != "" {
					out.Position = &gitlab.NotePosition{Path: path, Line: int(line)}
				}
			}
		}
	}
	return out
}

func (mrs) Pipelines(context.Context, gitlab.ProjectRef, int64) ([]gitlab.PipelineSummary, error) {
	return nil, errGraphQLOnly
}

// Commits returns every commit of the MR (all pages) in API order, newest
// first.
func (s mrs) Commits(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.CommitSummary, error) {
	var out []gitlab.CommitSummary
	opt := &gl.GetMergeRequestCommitsOptions{ListOptions: gl.ListOptions{PerPage: 100, Page: 1}}
	for {
		page, resp, err := s.c.api.MergeRequests.GetMergeRequestCommits(pid(p), iid, opt, gl.WithContext(ctx))
		if err != nil {
			return nil, mapError(err)
		}
		for _, c := range page {
			out = append(out, mapCommit(c))
		}
		if resp.NextPage == 0 {
			return out, nil
		}
		opt.Page = resp.NextPage
	}
}

func (s mrs) Approve(ctx context.Context, p gitlab.ProjectRef, iid int64, sha string) error {
	opt := &gl.ApproveMergeRequestOptions{}
	if sha != "" {
		opt.SHA = &sha
	}
	_, _, err := s.c.api.MergeRequestApprovals.ApproveMergeRequest(pid(p), iid, opt, gl.WithContext(ctx))
	return mapError(err)
}

func (s mrs) Unapprove(ctx context.Context, p gitlab.ProjectRef, iid int64) error {
	_, err := s.c.api.MergeRequestApprovals.UnapproveMergeRequest(pid(p), iid, gl.WithContext(ctx))
	return mapError(err)
}

func mapCommit(c *gl.Commit) gitlab.CommitSummary {
	return gitlab.CommitSummary{SHA: c.ID, ShortSHA: c.ShortID, Title: c.Title, AuthorName: c.AuthorName, WebURL: c.WebURL, CreatedAt: deref(c.CreatedAt)}
}

// Merge merges the MR, or sets it to merge when the pipeline succeeds. The
// flag for the latter depends on the instance version: auto_merge from
// 17.11, merge_when_pipeline_succeeds before that.
func (s mrs) Merge(ctx context.Context, p gitlab.ProjectRef, iid int64, o gitlab.MergeOptions) (gitlab.MergeRequest, error) {
	opt := &gl.AcceptMergeRequestOptions{Squash: &o.Squash, ShouldRemoveSourceBranch: &o.RemoveSourceBranch}
	if o.SHA != "" {
		opt.SHA = &o.SHA
	}
	if o.AutoMerge {
		v, err := s.c.Instance().Version(ctx)
		if err != nil {
			return gitlab.MergeRequest{}, err
		}
		yes := true
		if versionAtLeast(v.Version, 17, 11) {
			opt.AutoMerge = &yes
		} else {
			opt.MergeWhenPipelineSucceeds = &yes //nolint:staticcheck // instances before 17.11 only know this flag
		}
	}
	mr, _, err := s.c.api.MergeRequests.AcceptMergeRequest(pid(p), iid, opt, gl.WithContext(ctx))
	if err != nil {
		return gitlab.MergeRequest{}, mapError(err)
	}
	return mapMR(p, mr), nil
}

func (s mrs) Rebase(ctx context.Context, p gitlab.ProjectRef, iid int64) error {
	_, err := s.c.api.MergeRequests.RebaseMergeRequest(pid(p), iid, nil, gl.WithContext(ctx))
	return mapError(err)
}

// RecheckMergeStatus triggers GitLab's lazy merge status computation and
// returns the detailed_merge_status of the response. It is the only place MR
// details are read over REST; plain Get stays on GraphQL.
func (s mrs) RecheckMergeStatus(ctx context.Context, p gitlab.ProjectRef, iid int64) (string, error) {
	// GetMergeRequestsOptions has no recheck field in this client-go version,
	// so the query parameter is added by hand.
	recheck := func(r *retryablehttp.Request) error {
		q := r.URL.Query()
		q.Set("with_merge_status_recheck", "true")
		r.URL.RawQuery = q.Encode()
		return nil
	}
	mr, _, err := s.c.api.MergeRequests.GetMergeRequest(pid(p), iid, nil, gl.WithContext(ctx), recheck)
	if err != nil {
		return "", mapError(err)
	}
	return mr.DetailedMergeStatus, nil
}

// mapMR maps the subset of fields a mutation response is used for.
func mapMR(p gitlab.ProjectRef, m *gl.MergeRequest) gitlab.MergeRequest {
	if m == nil {
		return gitlab.MergeRequest{}
	}
	return gitlab.MergeRequest{
		Project: p, IID: m.IID, Title: m.Title, State: gitlab.MRState(m.State), SHA: m.SHA,
		SourceBranch: m.SourceBranch, TargetBranch: m.TargetBranch, MergeStatus: m.DetailedMergeStatus,
		WebURL: m.WebURL, Draft: m.Draft,
	}
}

// versionAtLeast reports whether a GitLab version string such as
// "17.11.2-ee" or "18.0.0-pre" is at least major.minor. Unparseable
// versions count as old.
func versionAtLeast(version string, major, minor int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	maj, err1 := strconv.Atoi(parts[0])
	minor2, err2 := strconv.Atoi(leadingDigits(parts[1]))
	if err1 != nil || err2 != nil {
		return false
	}
	return maj > major || (maj == major && minor2 >= minor)
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}
