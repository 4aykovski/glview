package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Batch kinds of the workspace queries; each one learns its own batch size.
const (
	KindOverview     = "ws.overview"      // phase 1
	KindOverviewFull = "ws.overview.full" // phases 1 and 2 in one request
	KindOverviewP2   = "ws.overview.p2"   // phase 2 only (default branch pipeline)
	KindMRs          = "ws.mrs"
	KindMRsMine      = "ws.mrs.mine"
	KindPipelines    = "ws.pipelines"
)

// overviewFragment is phase 1 of the Projects tab (SPEC §5.2).
const overviewFragment = `
fragment WsOverview on Project {
  id name webUrl lastActivityAt archived
  repository { rootRef }
  opened: mergeRequests(state: opened) { count }
  review: mergeRequests(state: opened, reviewerUsername: $me) { count }
}
`

// defaultPipelineField is phase 2: the latest pipeline of the default
// branch, whose name is the alias's own variable $rN.
const defaultPipelineField = `dp: pipelines(ref: $r%d, first: 1) { nodes { id iid status path } }`

type gqlOverview struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	WebURL         string `json:"webUrl"`
	LastActivityAt string `json:"lastActivityAt"`
	Archived       bool   `json:"archived"`
	Repository     *struct {
		RootRef *string `json:"rootRef"`
	} `json:"repository"`
	Opened *struct {
		Count int `json:"count"`
	} `json:"opened"`
	Review *struct {
		Count int `json:"count"`
	} `json:"review"`
	DP *struct {
		Nodes []gqlPipeline `json:"nodes"`
	} `json:"dp"`
}

// OverviewRequest is one overview batch: Phase 1 (metadata), Phase 2
// (default branch pipelines for the known RootRefs) or both (Full).
type OverviewRequest struct {
	Paths    []string
	RootRefs map[string]string // required for phase 2 and full requests
	Me       string
	Phase1   bool
	Phase2   bool
}

// Overview runs an overview batch. Projects without a default branch
// (empty repositories) get no phase 2 and are reported with Phase 2 at once.
func (c *Client) Overview(ctx context.Context, r OverviewRequest) <-chan gitlab.BatchResult[gitlab.ProjectOverview] {
	return Batch(ctx, c, c.overviewSpec(r), r.Paths)
}

func (c *Client) overviewSpec(r OverviewRequest) BatchSpec[gitlab.ProjectOverview] {
	origin := c.origin()
	spec := BatchSpec[gitlab.ProjectOverview]{
		Decode: func(path string, raw json.RawMessage) ([]gitlab.ProjectOverview, int, error) {
			var o gqlOverview
			if err := json.Unmarshal(raw, &o); err != nil {
				return nil, 0, err
			}
			ov := o.domain(path, origin)
			if r.Phase2 {
				ov.Phase = 2
			}
			if !r.Phase1 {
				ov.DefaultBranch = r.RootRefs[path]
			}
			return []gitlab.ProjectOverview{ov}, 1, nil
		},
	}
	switch {
	case r.Phase1 && r.Phase2:
		spec.Kind = KindOverviewFull
	case r.Phase1:
		spec.Kind = KindOverview
	default:
		spec.Kind = KindOverviewP2
	}
	if r.Phase1 {
		spec.Fragment, spec.FragmentName = overviewFragment, "WsOverview"
		spec.VarDecls = "$me: String!"
		spec.Vars = map[string]any{"me": r.Me}
	}
	if r.Phase2 {
		spec.PerProject = func(i int, path string) (string, map[string]any) {
			return fmt.Sprintf("$r%d: String!", i), map[string]any{fmt.Sprintf("r%d", i): r.RootRefs[path]}
		}
		spec.Body = func(i int) string {
			body := fmt.Sprintf(defaultPipelineField, i)
			if r.Phase1 {
				body = "...WsOverview " + body
			}
			return body
		}
	}
	return spec
}

func (o gqlOverview) domain(path, origin string) gitlab.ProjectOverview {
	ov := gitlab.ProjectOverview{
		Project: gitlab.Project{
			ProjectRef:     gitlab.ProjectRef{ID: gidOrZero(o.ID), FullPath: path},
			Name:           o.Name,
			WebURL:         o.WebURL,
			LastActivityAt: parseTime(o.LastActivityAt),
			Archived:       o.Archived,
		},
		Phase: 1,
	}
	if o.Repository != nil && o.Repository.RootRef != nil {
		ov.DefaultBranch = *o.Repository.RootRef
	}
	if o.Opened != nil {
		ov.OpenMRs = o.Opened.Count
	}
	if o.Review != nil {
		ov.ReviewMRs = o.Review.Count
	}
	if o.DP != nil && len(o.DP.Nodes) > 0 {
		p := o.DP.Nodes[0].summary(ov.ProjectRef, origin)
		ov.DefaultPipeline = &p
	}
	return ov
}

// WorkspaceMRs lists the first perProject merge requests of every project
// (state and title search of f). With the "me" filter every project gets
// three connections (author, assignee, reviewer) merged by IID; Total is
// then a lower bound.
func (c *Client) WorkspaceMRs(ctx context.Context, paths []string, f gitlab.MRFilter, me string, perProject int) <-chan gitlab.BatchResult[gitlab.MergeRequest] {
	return Batch(ctx, c, c.mrGroupSpec(f, me, perProject), paths)
}

func (c *Client) mrGroupSpec(f gitlab.MRFilter, me string, perProject int) BatchSpec[gitlab.MergeRequest] {
	if perProject <= 0 {
		perProject = 3
	}
	decls := []string{"$first: Int!"}
	vars := map[string]any{"first": perProject}
	args := []string{}
	if f.State != "" {
		decls = append(decls, "$state: MergeRequestState")
		vars["state"] = string(f.State)
		args = append(args, "state: $state")
	}
	if f.Search != "" {
		decls = append(decls, "$search: String")
		vars["search"] = f.Search
		args = append(args, "search: $search")
	}
	conn := func(alias, extra string) string {
		a := append([]string(nil), args...)
		if extra != "" {
			a = append(a, extra)
		}
		a = append(a, "first: $first", "sort: UPDATED_DESC")
		return fmt.Sprintf("  %s: mergeRequests(%s) { count nodes { ...MRRow } }\n", alias, strings.Join(a, ", "))
	}
	kind, body := KindMRs, conn("m", "")
	aliases := []string{"m"}
	if f.Mine {
		decls = append(decls, "$me: String!")
		vars["me"] = me
		kind = KindMRsMine
		body = conn("a", "authorUsername: $me") + conn("s", "assigneeUsername: $me") + conn("r", "reviewerUsername: $me")
		aliases = []string{"a", "s", "r"}
	}
	origin := c.origin()
	return BatchSpec[gitlab.MergeRequest]{
		Kind:         kind,
		Fragment:     "fragment WsMRGroup on Project {\n" + body + "}\n" + mrRowFragment,
		FragmentName: "WsMRGroup",
		VarDecls:     strings.Join(decls, ", "),
		Vars:         vars,
		Decode: func(path string, raw json.RawMessage) ([]gitlab.MergeRequest, int, error) {
			var conns map[string]*struct {
				Count int     `json:"count"`
				Nodes []gqlMR `json:"nodes"`
			}
			if err := json.Unmarshal(raw, &conns); err != nil {
				return nil, 0, err
			}
			ref := gitlab.ProjectRef{FullPath: path}
			seen := map[int64]bool{}
			var items []gitlab.MergeRequest
			total := 0
			for _, a := range aliases {
				cn := conns[a]
				if cn == nil {
					continue
				}
				total = max(total, cn.Count)
				for _, n := range cn.Nodes {
					mr := n.domain(ref, me, origin)
					if !seen[mr.IID] {
						seen[mr.IID] = true
						items = append(items, mr)
					}
				}
			}
			sortByUpdated(items)
			total = max(total, len(items))
			if len(items) > perProject {
				items = items[:perProject]
			}
			return items, total, nil
		},
	}
}

// WorkspacePipelines lists the first perProject pipelines of every project
// (status, ref and username of f).
func (c *Client) WorkspacePipelines(ctx context.Context, paths []string, f gitlab.PipelineFilter, perProject int) <-chan gitlab.BatchResult[gitlab.PipelineSummary] {
	return Batch(ctx, c, c.pipelineGroupSpec(f, perProject), paths)
}

func (c *Client) pipelineGroupSpec(f gitlab.PipelineFilter, perProject int) BatchSpec[gitlab.PipelineSummary] {
	if perProject <= 0 {
		perProject = 3
	}
	decls := []string{"$first: Int!"}
	vars := map[string]any{"first": perProject}
	var args []string
	add := func(name, typ string, v any) {
		decls = append(decls, "$"+name+": "+typ)
		vars[name] = v
		args = append(args, name+": $"+name)
	}
	if f.Status != "" {
		add("status", "PipelineStatusEnum", strings.ToUpper(f.Status))
	}
	if f.Ref != "" {
		add("ref", "String", f.Ref)
	}
	if f.Username != "" {
		add("username", "String", f.Username)
	}
	args = append(args, "first: $first")
	origin := c.origin()
	return BatchSpec[gitlab.PipelineSummary]{
		Kind: KindPipelines,
		Fragment: fmt.Sprintf("fragment WsPipeGroup on Project {\n  pipelines(%s) { count nodes { ...PipelineRow } }\n}\n%s",
			strings.Join(args, ", "), pipelineRowFragment),
		FragmentName: "WsPipeGroup",
		VarDecls:     strings.Join(decls, ", "),
		Vars:         vars,
		Decode: func(path string, raw json.RawMessage) ([]gitlab.PipelineSummary, int, error) {
			var p struct {
				Pipelines struct {
					Count int           `json:"count"`
					Nodes []gqlPipeline `json:"nodes"`
				} `json:"pipelines"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, 0, err
			}
			ref := gitlab.ProjectRef{FullPath: path}
			items := make([]gitlab.PipelineSummary, len(p.Pipelines.Nodes))
			for i, n := range p.Pipelines.Nodes {
				items[i] = n.summary(ref, origin)
			}
			return items, p.Pipelines.Count, nil
		},
	}
}

const searchQuery = `query Search($q: String!) {
  projects(search: $q, searchNamespaces: true, first: 10, sort: "latest_activity_desc") {
    nodes { id fullPath name webUrl lastActivityAt archived }
  }
  groups(search: $q, first: 5) { nodes { id fullPath name webUrl } }
}`

// SchemaError is a GraphQL error in the response of a query the instance
// may not support (an unknown argument on an old instance): the caller
// falls back to REST.
type SchemaError struct{ Message string }

func (e *SchemaError) Error() string { return "graphql: " + e.Message }

// Search finds up to 10 projects (matching the namespace too, most active
// first) and 5 groups in one request.
func (c *Client) Search(ctx context.Context, q string) ([]gitlab.Project, []gitlab.Group, error) {
	resp, err := c.Raw(ctx, searchQuery, map[string]any{"q": q})
	if err != nil {
		return nil, nil, err
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return nil, nil, &SchemaError{Message: strings.Join(msgs, "; ")}
	}
	var data struct {
		Projects struct {
			Nodes []struct {
				ID             string `json:"id"`
				FullPath       string `json:"fullPath"`
				Name           string `json:"name"`
				WebURL         string `json:"webUrl"`
				LastActivityAt string `json:"lastActivityAt"`
				Archived       bool   `json:"archived"`
			} `json:"nodes"`
		} `json:"projects"`
		Groups struct {
			Nodes []struct {
				ID       string `json:"id"`
				FullPath string `json:"fullPath"`
				Name     string `json:"name"`
				WebURL   string `json:"webUrl"`
			} `json:"nodes"`
		} `json:"groups"`
	}
	if err := decodeData(resp, &data); err != nil {
		return nil, nil, err
	}
	projects := make([]gitlab.Project, len(data.Projects.Nodes))
	for i, n := range data.Projects.Nodes {
		projects[i] = gitlab.Project{
			ProjectRef: gitlab.ProjectRef{ID: gidOrZero(n.ID), FullPath: n.FullPath},
			Name:       n.Name, WebURL: n.WebURL, LastActivityAt: parseTime(n.LastActivityAt), Archived: n.Archived,
		}
	}
	groups := make([]gitlab.Group, len(data.Groups.Nodes))
	for i, n := range data.Groups.Nodes {
		groups[i] = gitlab.Group{ID: gidOrZero(n.ID), FullPath: n.FullPath, Name: n.Name, WebURL: n.WebURL}
	}
	return projects, groups, nil
}
