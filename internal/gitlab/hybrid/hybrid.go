// Package hybrid assembles one gitlab.Client from the REST adapter and the
// GraphQL client: lists, details and stage previews come from GraphQL
// (SPEC §13.1), everything else — including all mutations — from REST.
package hybrid

import (
	"context"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/graphql"
	"github.com/4aykovski/glview/internal/gitlab/rest"
)

// Client implements gitlab.Client over both transports.
type Client struct {
	rest  *rest.Client
	gql   *graphql.Client
	me    string
	cache *cache.Cache // default branches and latest tags of workspace projects
}

var _ gitlab.Client = (*Client)(nil)

// New returns a client for the current user me (username): it is needed for
// "@me", the "me" filter and the "approved by me" flag.
func New(r *rest.Client, g *graphql.Client, me string) *Client {
	return &Client{rest: r, gql: g, me: me, cache: cache.New()}
}

// WithCache makes the client keep its workspace caches (default branches,
// latest tags) in c, shared with the UI.
func (c *Client) WithCache(cc *cache.Cache) *Client {
	if cc != nil {
		c.cache = cc
	}
	return c
}

func (c *Client) Users() gitlab.UserService        { return c.rest.Users() }
func (c *Client) Instance() gitlab.InstanceService { return c.rest.Instance() }
func (c *Client) Jobs() gitlab.JobService          { return c.rest.Jobs() }
func (c *Client) Tags() gitlab.TagService          { return c.rest.Tags() }

func (c *Client) Projects() gitlab.ProjectService {
	return projects{ProjectService: c.rest.Projects(), gql: c.gql}
}

func (c *Client) MergeRequests() gitlab.MergeRequestService {
	return mrs{MergeRequestService: c.rest.MergeRequests(), gql: c.gql, me: c.me}
}

func (c *Client) Pipelines() gitlab.PipelineService {
	return pipelines{PipelineService: c.rest.Pipelines(), gql: c.gql}
}

// Each wrapper embeds the REST service and overrides the GraphQL methods.

type projects struct {
	gitlab.ProjectService
	gql *graphql.Client
}

func (s projects) CIConfigVariables(ctx context.Context, p gitlab.ProjectRef, gitRef string) ([]gitlab.CIConfigVariable, error) {
	return s.gql.CIConfigVariables(ctx, p, gitRef)
}

type mrs struct {
	gitlab.MergeRequestService
	gql *graphql.Client
	me  string
}

func (s mrs) List(ctx context.Context, p gitlab.ProjectRef, f gitlab.MRFilter, cursor string, limit int) (gitlab.Page[gitlab.MergeRequest], error) {
	return s.gql.ProjectMRs(ctx, p, f, s.me, cursor, limit)
}

func (s mrs) Get(ctx context.Context, p gitlab.ProjectRef, iid int64) (gitlab.MergeRequest, error) {
	return s.gql.MR(ctx, p, iid, s.me)
}

func (s mrs) Pipelines(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.PipelineSummary, error) {
	return s.gql.MRPipelines(ctx, p, iid, 20)
}

type pipelines struct {
	gitlab.PipelineService
	gql *graphql.Client
}

func (s pipelines) List(ctx context.Context, p gitlab.ProjectRef, f gitlab.PipelineFilter, cursor string, limit int) (gitlab.Page[gitlab.PipelineSummary], error) {
	return s.gql.ProjectPipelines(ctx, p, f, cursor, limit)
}

func (s pipelines) Stages(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.StageSummary, error) {
	return s.gql.PipelineStages(ctx, p, iid)
}
