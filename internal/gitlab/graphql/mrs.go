package graphql

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/4aykovski/glview/internal/gitlab"
)

// pipelineRowFragment is the shared pipeline row (MR head pipelines and the
// pipelines list).
const pipelineRowFragment = `
fragment PipelineRow on Pipeline {
  id iid status ref sha duration createdAt finishedAt path
  #@opt source
  source
  #@end
  user { username name }
  #@opt commit
  commit { title }
  #@end
}
`

// mrRowFragment is the merge request row used by the project MR list (and,
// in M2, by the workspace batches).
const mrRowFragment = `
fragment MRRow on MergeRequest {
  iid title description draft state createdAt updatedAt webUrl
  sourceBranch targetBranch diffHeadSha detailedMergeStatus conflicts
  squash forceRemoveSourceBranch
  author { username name webUrl }
  reviewers { nodes { username name } }
  assignees { nodes { username name } }
  labels { nodes { title } }
  approvedBy { nodes { username name } }
  approvalsRequired approvalsLeft
  #@opt resolvableDiscussionsCount,resolvedDiscussionsCount
  resolvableDiscussionsCount resolvedDiscussionsCount
  #@end
  #@opt diffStatsSummary
  diffStatsSummary { additions deletions fileCount }
  #@end
  headPipeline { ...PipelineRow }
}
` + pipelineRowFragment

type gqlMR struct {
	IID                     string `json:"iid"`
	Title                   string `json:"title"`
	Description             string `json:"description"`
	Draft                   bool   `json:"draft"`
	State                   string `json:"state"`
	CreatedAt               string `json:"createdAt"`
	UpdatedAt               string `json:"updatedAt"`
	WebURL                  string `json:"webUrl"`
	SourceBranch            string `json:"sourceBranch"`
	TargetBranch            string `json:"targetBranch"`
	DiffHeadSHA             string `json:"diffHeadSha"`
	DetailedMergeStatus     string `json:"detailedMergeStatus"`
	Conflicts               bool   `json:"conflicts"`
	Squash                  bool   `json:"squash"`
	ForceRemoveSourceBranch bool   `json:"forceRemoveSourceBranch"`
	Author                  *gqlUser
	Reviewers               gqlUsers `json:"reviewers"`
	Assignees               gqlUsers `json:"assignees"`
	Labels                  struct {
		Nodes []struct {
			Title string `json:"title"`
		} `json:"nodes"`
	} `json:"labels"`
	ApprovedBy                 gqlUsers `json:"approvedBy"`
	ApprovalsRequired          *int     `json:"approvalsRequired"`
	ApprovalsLeft              *int     `json:"approvalsLeft"`
	ResolvableDiscussionsCount *int     `json:"resolvableDiscussionsCount"`
	ResolvedDiscussionsCount   *int     `json:"resolvedDiscussionsCount"`
	DiffStatsSummary           *struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
		FileCount int `json:"fileCount"`
	} `json:"diffStatsSummary"`
	HeadPipeline *gqlPipeline `json:"headPipeline"`

	// Detail-only fields.
	RebaseInProgress bool     `json:"rebaseInProgress"`
	MergeError       string   `json:"mergeError"`
	MergedAt         string   `json:"mergedAt"`
	ClosedAt         string   `json:"closedAt"`
	MergeUser        *gqlUser `json:"mergeUser"`
	CommitCount      *int     `json:"commitCount"`
}

// domain maps a node to a MergeRequest; me is the current user's name.
func (m gqlMR) domain(project gitlab.ProjectRef, me, origin string) gitlab.MergeRequest {
	mr := gitlab.MergeRequest{
		Project:                 project,
		IID:                     parseInt(m.IID),
		Title:                   m.Title,
		Description:             m.Description,
		State:                   gitlab.MRState(lower(m.State)),
		Draft:                   m.Draft,
		Assignees:               m.Assignees.domain(),
		Reviewers:               m.Reviewers.domain(),
		SourceBranch:            m.SourceBranch,
		TargetBranch:            m.TargetBranch,
		SHA:                     m.DiffHeadSHA,
		MergeStatus:             lower(m.DetailedMergeStatus),
		HasConflicts:            m.Conflicts,
		WebURL:                  m.WebURL,
		CreatedAt:               parseTime(m.CreatedAt),
		UpdatedAt:               parseTime(m.UpdatedAt),
		RebaseInProgress:        m.RebaseInProgress,
		MergeError:              m.MergeError,
		MergedAt:                parseTime(m.MergedAt),
		ClosedAt:                parseTime(m.ClosedAt),
		Squash:                  m.Squash,
		ForceRemoveSourceBranch: m.ForceRemoveSourceBranch,
	}
	if m.Author != nil {
		mr.Author = m.Author.domain()
	}
	if m.MergeUser != nil {
		u := m.MergeUser.domain()
		mr.MergedBy = &u
	}
	for _, l := range m.Labels.Nodes {
		mr.Labels = append(mr.Labels, l.Title)
	}
	mr.Approvals.ApprovedBy = m.ApprovedBy.domain()
	for _, u := range mr.Approvals.ApprovedBy {
		if me != "" && u.Username == me {
			mr.Approvals.ApprovedByMe = true
		}
	}
	if m.ApprovalsRequired != nil {
		mr.Approvals.Required = *m.ApprovalsRequired
	}
	if m.ApprovalsLeft != nil {
		mr.Approvals.Left = *m.ApprovalsLeft
	}
	// Unknown thread counts (field missing on the instance) are -1.
	mr.Discussions = gitlab.DiscussionStats{Total: -1, Unresolved: -1}
	if m.ResolvableDiscussionsCount != nil && m.ResolvedDiscussionsCount != nil {
		mr.Discussions = gitlab.DiscussionStats{
			Total:      *m.ResolvableDiscussionsCount,
			Unresolved: *m.ResolvableDiscussionsCount - *m.ResolvedDiscussionsCount,
		}
	}
	if d := m.DiffStatsSummary; d != nil {
		mr.Diff = &gitlab.DiffStats{Additions: d.Additions, Deletions: d.Deletions, Files: d.FileCount}
	}
	if m.CommitCount != nil {
		mr.CommitCount = *m.CommitCount
	}
	if m.HeadPipeline != nil {
		ps := m.HeadPipeline.summary(project, origin)
		mr.HeadPipeline = &ps
	}
	return mr
}

// origin returns scheme://host of the instance, the base for relative paths
// such as a pipeline's path.
func (c *Client) origin() string {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

type gqlMRConn struct {
	Count    int `json:"count"`
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []gqlMR `json:"nodes"`
}

// mrAlias is one connection of the list query. Plain lists use a single
// alias "m"; the "me" filter uses a (author), s (assignee) and r (reviewer).
type mrAlias struct {
	name  string
	extra string // extra argument fixing the user, e.g. "authorUsername: $me"
}

// ProjectMRs lists merge requests of the project. me is the current user's
// username (for "@me", the "me" filter and ApprovedByMe). For the "me"
// filter NextCursor packs the three connection cursors as "a|s|r" (an
// empty part means that connection is exhausted) and Total is a lower
// bound (the largest connection count).
func (c *Client) ProjectMRs(ctx context.Context, project gitlab.ProjectRef, f gitlab.MRFilter, me, cursor string, limit int) (gitlab.Page[gitlab.MergeRequest], error) {
	if limit <= 0 {
		limit = 20
	}
	aliases := []mrAlias{{name: "m"}}
	cursors := map[string]string{"m": cursor}
	if f.Mine {
		aliases = []mrAlias{
			{"a", "authorUsername: $me"},
			{"s", "assigneeUsername: $me"},
		}
		if f.Reviewer == "" || f.Reviewer == "@me" {
			aliases = append(aliases, mrAlias{"r", "reviewerUsername: $me"})
		} else {
			aliases = append(aliases, mrAlias{"r", ""}) // contradicts the reviewer filter: skipped below
		}
		parts := strings.SplitN(cursor, "|", 3)
		for i, a := range aliases {
			cursors[a.name] = ""
			if cursor != "" {
				if i < len(parts) {
					cursors[a.name] = parts[i]
				}
				if cursors[a.name] == "" {
					cursors[a.name] = "-" // exhausted
				}
			}
		}
	}

	var active []mrAlias
	for _, a := range aliases {
		if cursors[a.name] == "-" {
			continue
		}
		if f.Mine && a.name == "r" && a.extra == "" {
			continue
		}
		active = append(active, a)
	}
	if len(active) == 0 {
		return gitlab.Page[gitlab.MergeRequest]{Total: -1}, nil
	}

	conns, err := c.fetchMRAliases(ctx, project, f, me, active, cursors, limit)
	if err != nil {
		return gitlab.Page[gitlab.MergeRequest]{}, err
	}

	origin := c.origin()
	seen := map[int64]bool{}
	var items []gitlab.MergeRequest
	next := make([]string, len(aliases))
	more, total := false, 0
	for i, a := range aliases {
		conn, ok := conns[a.name]
		if !ok {
			continue // skipped alias keeps an empty (exhausted) cursor
		}
		total = max(total, conn.Count)
		if conn.PageInfo.HasNextPage {
			next[i] = conn.PageInfo.EndCursor
			more = true
		}
		for _, n := range conn.Nodes {
			mr := n.domain(project, me, origin)
			if seen[mr.IID] {
				continue
			}
			seen[mr.IID] = true
			items = append(items, mr)
		}
	}
	sortByUpdated(items)

	page := gitlab.Page[gitlab.MergeRequest]{Items: items, Total: total}
	if more {
		if f.Mine {
			page.NextCursor = strings.Join(next, "|")
		} else {
			page.NextCursor = next[0]
		}
	}
	return page, nil
}

// fetchMRAliases runs one request per alias, in parallel. Three full MR
// connections in one query exceed GitLab's complexity limit (about 300 of
// 250 with 20 rows each), one connection costs about a third of it.
func (c *Client) fetchMRAliases(ctx context.Context, project gitlab.ProjectRef, f gitlab.MRFilter, me string, aliases []mrAlias, cursors map[string]string, limit int) (map[string]gqlMRConn, error) {
	var mu sync.Mutex
	out := make(map[string]gqlMRConn, len(aliases))
	g, gctx := errgroup.WithContext(ctx)
	for _, a := range aliases {
		g.Go(func() error {
			query, vars := buildProjectMRs(project, f, me, []mrAlias{a}, cursors, limit)
			var data struct {
				Project map[string]gqlMRConn `json:"project"`
			}
			if err := c.DoOpt(gctx, query, vars, &data); err != nil {
				return err
			}
			if data.Project == nil {
				return &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404, Message: "project " + project.FullPath}
			}
			mu.Lock()
			maps.Copy(out, data.Project)
			mu.Unlock()
			return nil
		})
	}
	return out, g.Wait()
}

func sortByUpdated(items []gitlab.MergeRequest) {
	// Stable insertion sort keeps equal timestamps in connection order.
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].UpdatedAt.After(items[j-1].UpdatedAt); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// buildProjectMRs assembles the list query for the given aliases. Only the
// filters that are set become arguments, so unset ones are never sent as null.
func buildProjectMRs(project gitlab.ProjectRef, f gitlab.MRFilter, me string, aliases []mrAlias, cursors map[string]string, limit int) (string, map[string]any) {
	vars := map[string]any{"path": project.FullPath, "first": limit}
	decls := []string{"$path: ID!", "$first: Int!"}
	var common []string

	if f.State != "" {
		decls = append(decls, "$state: MergeRequestState")
		vars["state"] = string(f.State)
		common = append(common, "state: $state")
	}
	user := func(argName, v string) {
		if v == "@me" {
			v = me
		}
		decls = append(decls, "$"+argName+": String")
		vars[argName] = v
	}
	scopeArg := ""
	switch f.Scope {
	case gitlab.MRScopeCreatedByMe:
		scopeArg = "authorUsername: $me"
	case gitlab.MRScopeAssignedToMe:
		scopeArg = "assigneeUsername: $me"
	}
	needMe := scopeArg != "" || f.Mine
	if needMe {
		decls = append(decls, "$me: String")
		vars["me"] = me
	}
	if f.Reviewer != "" {
		user("reviewer", f.Reviewer)
		common = append(common, "reviewerUsername: $reviewer")
	}
	if f.Draft != nil {
		decls = append(decls, "$draft: Boolean")
		vars["draft"] = *f.Draft
		common = append(common, "draft: $draft")
	}
	if len(f.Labels) > 0 {
		decls = append(decls, "$labels: [String!]")
		vars["labels"] = f.Labels
		common = append(common, "labels: $labels")
	}
	if f.Search != "" {
		decls = append(decls, "$search: String")
		vars["search"] = f.Search
		common = append(common, "search: $search")
	}
	if f.SourceBranch != "" {
		decls = append(decls, "$sourceBranches: [String!]")
		vars["sourceBranches"] = []string{f.SourceBranch}
		common = append(common, "sourceBranches: $sourceBranches")
	}

	var b strings.Builder
	var proj []string
	for _, a := range aliases {
		args := append([]string(nil), common...)
		if scopeArg != "" && scopeArg != a.extra {
			args = append(args, scopeArg)
		}
		if a.extra != "" {
			// The alias's own user argument replaces a clashing filter argument
			// (reviewer:@me with the "me" filter is the same user anyway).
			key := a.extra[:strings.Index(a.extra, ":")]
			args = removeArg(args, key)
			args = append(args, a.extra)
		}
		if cur := cursors[a.name]; cur != "" && cur != "-" {
			v := "after_" + a.name
			decls = append(decls, "$"+v+": String")
			vars[v] = cur
			args = append(args, "after: $"+v)
		}
		args = append(args, "first: $first", "sort: UPDATED_DESC")
		proj = append(proj, fmt.Sprintf("    %s: mergeRequests(%s) { count pageInfo { hasNextPage endCursor } nodes { ...MRRow } }",
			a.name, strings.Join(args, ", ")))
	}
	fmt.Fprintf(&b, "query ProjectMRs(%s) {\n  project(fullPath: $path) {\n%s\n  }\n}\n", strings.Join(decls, ", "), strings.Join(proj, "\n"))
	b.WriteString(mrRowFragment)
	return b.String(), vars
}

func removeArg(args []string, key string) []string {
	out := args[:0:0]
	for _, a := range args {
		if !strings.HasPrefix(a, key+":") {
			out = append(out, a)
		}
	}
	return out
}

const mrDetailQuery = `query MR($path: ID!, $iid: String!) {
  project(fullPath: $path) {
    mergeRequest(iid: $iid) { ...MRDetail }
  }
}
fragment MRDetail on MergeRequest {
  ...MRRow
  rebaseInProgress
  mergeError
  mergedAt closedAt mergeUser { username name }
  #@opt commitCount
  commitCount
  #@end
  headPipeline {
    stages { nodes { name detailedStatus { group } jobs { nodes { status duration } } } }
  }
}
` + mrRowFragment

// MR returns one merge request with description, approvals, head pipeline
// stages and rebase state. me is the current user's username.
func (c *Client) MR(ctx context.Context, project gitlab.ProjectRef, iid int64, me string) (gitlab.MergeRequest, error) {
	var data struct {
		Project *struct {
			MergeRequest *gqlMR `json:"mergeRequest"`
		} `json:"project"`
	}
	vars := map[string]any{"path": project.FullPath, "iid": fmt.Sprint(iid)}
	if err := c.DoOpt(ctx, mrDetailQuery, vars, &data); err != nil {
		return gitlab.MergeRequest{}, err
	}
	if data.Project == nil || data.Project.MergeRequest == nil {
		return gitlab.MergeRequest{}, &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404, Message: fmt.Sprintf("merge request !%d", iid)}
	}
	return data.Project.MergeRequest.domain(project, me, c.origin()), nil
}
