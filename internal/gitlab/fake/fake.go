// Package fake is an in-memory gitlab.Client for tests and demos. Every call
// is recorded; per-method delays and errors can be scripted.
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Call is one recorded method invocation.
type Call struct {
	Method string // "Users.Current", "Projects.Get", ...
	Args   []any
}

// Client is a scriptable fake. The zero value is not usable; call New.
type Client struct {
	mu sync.Mutex

	BaseURL        string
	User           gitlab.User
	Ver            gitlab.Version
	ProjectsByPath map[string]gitlab.Project

	delays map[string]time.Duration
	errs   map[string][]error // queued errors per method, consumed in order
	calls  []Call
	hooks  map[string][]func(n int)
	data   data

	// Workspace and search data (workspace.go).
	projDelay     map[string]time.Duration
	batchSize     int
	starred       []string
	groups        []gitlab.Group
	groupProjects map[int64][]string
}

var _ gitlab.Client = (*Client)(nil)

// New returns a fake with a default user and instance version.
func New() *Client {
	return &Client{
		BaseURL:        "https://gitlab.example.com",
		User:           gitlab.User{ID: 1, Username: "egor", Name: "Egor"},
		Ver:            gitlab.Version{Version: "17.11.0"},
		ProjectsByPath: map[string]gitlab.Project{},
		delays:         map[string]time.Duration{},
		errs:           map[string][]error{},
		hooks:          map[string][]func(n int){},
		data:           newData(),
		projDelay:      map[string]time.Duration{},
		groupProjects:  map[int64][]string{},
	}
}

// AddProject registers a project by its full path.
func (c *Client) AddProject(p gitlab.Project) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ProjectsByPath[p.FullPath] = p
}

// SetDelay makes every call to method sleep for d (respecting ctx).
func (c *Client) SetDelay(method string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delays[method] = d
}

// FailNext queues err for the next call to method.
func (c *Client) FailNext(method string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errs[method] = append(c.errs[method], err)
}

// OnCall registers fn to run on every call of method, before it is served;
// n is the 1-based number of the call. Scenario tests use it to change the
// fake's data between polls (a rebase that finishes on the third Get…).
func (c *Client) OnCall(method string, fn func(n int)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hooks[method] = append(c.hooks[method], fn)
}

// Calls returns a copy of the recorded calls.
func (c *Client) Calls() []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Call(nil), c.calls...)
}

// CallCount returns how many times method was called.
func (c *Client) CallCount(method string) int {
	n := 0
	for _, call := range c.Calls() {
		if call.Method == method {
			n++
		}
	}
	return n
}

// ResetCalls clears the call log.
func (c *Client) ResetCalls() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = nil
}

// enter records the call, applies the scripted delay and returns a queued error.
func (c *Client) enter(ctx context.Context, method string, args ...any) error {
	c.mu.Lock()
	c.calls = append(c.calls, Call{Method: method, Args: args})
	n := 0
	for _, call := range c.calls {
		if call.Method == method {
			n++
		}
	}
	hooks := c.hooks[method]
	delay := c.delays[method]
	var err error
	if q := c.errs[method]; len(q) > 0 {
		err, c.errs[method] = q[0], q[1:]
	}
	c.mu.Unlock()

	// Hooks run outside the lock so they may call the mutators.
	for _, h := range hooks {
		h(n)
	}
	if delay > 0 {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (c *Client) Users() gitlab.UserService                 { return users{c} }
func (c *Client) Projects() gitlab.ProjectService           { return projects{c} }
func (c *Client) Instance() gitlab.InstanceService          { return instance{c} }
func (c *Client) MergeRequests() gitlab.MergeRequestService { return mrService{c} }
func (c *Client) Pipelines() gitlab.PipelineService         { return pipelineService{c} }
func (c *Client) Jobs() gitlab.JobService                   { return jobService{c} }
func (c *Client) Tags() gitlab.TagService                   { return tagService{c} }

type users struct{ c *Client }

func (s users) Current(ctx context.Context) (gitlab.User, error) {
	if err := s.c.enter(ctx, "Users.Current"); err != nil {
		return gitlab.User{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.c.User, nil
}

type projects struct{ c *Client }

func (s projects) Get(ctx context.Context, ref gitlab.ProjectRef) (gitlab.Project, error) {
	if err := s.c.enter(ctx, "Projects.Get", ref.FullPath); err != nil {
		return gitlab.Project{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	p, ok := s.c.ProjectsByPath[ref.FullPath]
	if !ok {
		return gitlab.Project{}, &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404, Message: fmt.Sprintf("project %s", ref.FullPath)}
	}
	return p, nil
}

type instance struct{ c *Client }

func (s instance) URL() string { return s.c.BaseURL }

func (s instance) Version(ctx context.Context) (gitlab.Version, error) {
	if err := s.c.enter(ctx, "Instance.Version"); err != nil {
		return gitlab.Version{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.c.Ver, nil
}
