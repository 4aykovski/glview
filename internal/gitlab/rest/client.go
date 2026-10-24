// Package rest adapts gitlab.com/gitlab-org/api/client-go to the domain
// types and interfaces of internal/gitlab.
package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/httpx"
)

// Client implements gitlab.Client over the REST API.
type Client struct {
	api     *gl.Client
	baseURL string

	verMu sync.Mutex
	ver   *gitlab.Version // cached for the session

	titleMu sync.Mutex
	titles  map[string]titleEntry // commit titles by SHA (commits are immutable)
}

var _ gitlab.Client = (*Client)(nil)

// New creates a REST client for the instance at baseURL (e.g.
// https://gitlab.example.com or https://example.com/gitlab) using the shared
// http client from httpx.
func New(baseURL, token string, hc *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	api, err := gl.NewClient(token,
		gl.WithBaseURL(baseURL+"/api/v4"),
		gl.WithHTTPClient(hc),
		gl.WithoutRetries(), // 429 is handled by httpx.Transport
	)
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}
	return &Client{api: api, baseURL: baseURL, titles: map[string]titleEntry{}}, nil
}

func (c *Client) MergeRequests() gitlab.MergeRequestService { return mrs{c} }
func (c *Client) Pipelines() gitlab.PipelineService         { return pipelines{c} }
func (c *Client) Jobs() gitlab.JobService                   { return jobs{c} }
func (c *Client) Tags() gitlab.TagService                   { return tags{c} }
func (c *Client) Users() gitlab.UserService                 { return users{c} }
func (c *Client) Projects() gitlab.ProjectService           { return projects{c} }
func (c *Client) Instance() gitlab.InstanceService {
	return instance{c}
}

type users struct{ c *Client }

func (s users) Current(ctx context.Context) (gitlab.User, error) {
	u, _, err := s.c.api.Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		return gitlab.User{}, mapError(err)
	}
	return mapUser(u), nil
}

type projects struct{ c *Client }

func (s projects) Get(ctx context.Context, ref gitlab.ProjectRef) (gitlab.Project, error) {
	p, _, err := s.c.api.Projects.GetProject(pid(ref), &gl.GetProjectOptions{}, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Project{}, mapError(err)
	}
	return mapProject(p), nil
}

type instance struct{ c *Client }

func (s instance) URL() string { return s.c.baseURL }

func (s instance) Version(ctx context.Context) (gitlab.Version, error) {
	s.c.verMu.Lock()
	defer s.c.verMu.Unlock()
	if s.c.ver != nil {
		return *s.c.ver, nil
	}
	v, _, err := s.c.api.Version.GetVersion(gl.WithContext(ctx))
	if err != nil {
		return gitlab.Version{}, mapError(err)
	}
	ver := gitlab.Version{Version: v.Version, Revision: v.Revision}
	s.c.ver = &ver
	return ver, nil
}

// pid is the project identifier for client-go: the numeric ID when known,
// otherwise the full path.
func pid(ref gitlab.ProjectRef) any {
	if ref.ID != 0 {
		return ref.ID
	}
	return ref.FullPath
}

// totalOf is the X-Total of a list response: 0 is a real zero, -1 means
// GitLab did not send the header (it does not for big result sets).
func totalOf(resp *gl.Response) int {
	if resp == nil || resp.Response == nil || resp.Header.Get("X-Total") == "" {
		return -1
	}
	return int(resp.TotalItems)
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	var er *gl.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		return httpx.FromStatus(er.Response.StatusCode, er.Response.Header, er.Body)
	}
	if errors.Is(err, gl.ErrNotFound) {
		return &gitlab.Error{Kind: gitlab.ErrNotFound, Status: http.StatusNotFound}
	}
	return httpx.FromTransport(err)
}

func mapUser(u *gl.User) gitlab.User {
	if u == nil {
		return gitlab.User{}
	}
	return gitlab.User{ID: u.ID, Username: u.Username, Name: u.Name, WebURL: u.WebURL}
}

func mapProject(p *gl.Project) gitlab.Project {
	return gitlab.Project{
		ProjectRef:     gitlab.ProjectRef{ID: p.ID, FullPath: p.PathWithNamespace},
		Name:           p.Name,
		WebURL:         p.WebURL,
		DefaultBranch:  p.DefaultBranch,
		LastActivityAt: deref(p.LastActivityAt),
		Archived:       p.Archived,
	}
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// errGraphQLOnly is returned by methods that the hybrid client serves
// through GraphQL (lists, details, stage previews, CI variable hints); the
// REST adapter alone does not implement them.
var errGraphQLOnly = errors.New("served by the GraphQL client only")
