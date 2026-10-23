package fake

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

// DefaultBatchSize is how many projects one fake workspace batch holds.
const DefaultBatchSize = 10

// SetProjectDelay delays every workspace batch that contains path by d, so
// tests can make batches finish in any order.
func (c *Client) SetProjectDelay(path string, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projDelay[path] = d
}

// SetBatchSize changes the number of projects per workspace batch.
func (c *Client) SetBatchSize(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.batchSize = n
}

// SetStarred marks the registered projects with the given paths as starred.
func (c *Client) SetStarred(paths ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.starred = paths
}

// AddGroup registers a group and the paths of its projects (most active
// first, as GitLab returns them).
func (c *Client) AddGroup(g gitlab.Group, projects ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.groups = append(c.groups, g)
	c.groupProjects[g.ID] = projects
}

// SetUnknownTotal makes ListGroupProjects of the group report total -1
// when it has more projects than the limit (GitLab without X-Total).
func (c *Client) SetUnknownTotal(groupID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unknownTotal[groupID] = true
}

func (c *Client) Workspace() gitlab.WorkspaceService { return workspace{c} }
func (c *Client) Search() gitlab.SearchService       { return search{c} }

type workspace struct{ c *Client }

// batches splits ps like the GraphQL batcher and runs fn per batch in
// parallel, after the largest delay of the batch's projects.
func batches[T any](ctx context.Context, c *Client, ps []gitlab.ProjectRef, fn func(p gitlab.ProjectRef) gitlab.ProjectGroup[T]) <-chan gitlab.BatchResult[T] {
	out := make(chan gitlab.BatchResult[T])
	c.mu.Lock()
	size := c.batchSize
	if size <= 0 {
		size = DefaultBatchSize
	}
	c.mu.Unlock()
	var wg sync.WaitGroup
	for start := 0; start < len(ps); start += size {
		chunk := ps[start:min(start+size, len(ps))]
		wg.Add(1)
		go func() {
			defer wg.Done()
			var d time.Duration
			c.mu.Lock()
			for _, p := range chunk {
				d = max(d, c.projDelay[p.FullPath])
			}
			c.mu.Unlock()
			if d > 0 {
				t := time.NewTimer(d)
				defer t.Stop()
				select {
				case <-t.C:
				case <-ctx.Done():
					return
				}
			}
			var res gitlab.BatchResult[T]
			for _, p := range chunk {
				res.Groups = append(res.Groups, fn(p))
			}
			select {
			case out <- res:
			case <-ctx.Done():
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func failed[T any](ps []gitlab.ProjectRef, err error) <-chan gitlab.BatchResult[T] {
	ch := make(chan gitlab.BatchResult[T], 1)
	res := gitlab.BatchResult[T]{Err: err}
	for _, p := range ps {
		res.Groups = append(res.Groups, gitlab.ProjectGroup[T]{Project: p, Total: -1, Err: err})
	}
	ch <- res
	close(ch)
	return ch
}

func (w workspace) ProjectsOverview(ctx context.Context, ps []gitlab.ProjectRef, o gitlab.OverviewOptions) <-chan gitlab.BatchResult[gitlab.ProjectOverview] {
	c := w.c
	if err := c.enter(ctx, "Workspace.ProjectsOverview", paths(ps), o); err != nil {
		return failed[gitlab.ProjectOverview](ps, err)
	}
	return batches(ctx, c, ps, func(p gitlab.ProjectRef) gitlab.ProjectGroup[gitlab.ProjectOverview] {
		c.mu.Lock()
		defer c.mu.Unlock()
		pr, ok := c.ProjectsByPath[p.FullPath]
		if !ok {
			return gitlab.ProjectGroup[gitlab.ProjectOverview]{Project: p, Total: -1, Err: notFound("project " + p.FullPath)}
		}
		ov := gitlab.ProjectOverview{Project: pr, Phase: 2, TagKnown: true}
		me := c.User.Username
		for _, mr := range c.data.mrs[p.FullPath] {
			if mr.State != gitlab.MRStateOpened {
				continue
			}
			ov.OpenMRs++
			if slices.ContainsFunc(mr.Reviewers, func(u gitlab.User) bool { return u.Username == me }) {
				ov.ReviewMRs++
			}
		}
		var best *gitlab.PipelineSummary
		for _, pl := range c.data.pipes[p.FullPath] {
			if pl.Ref == pr.DefaultBranch && (best == nil || pl.ID > best.ID) {
				s := pl.PipelineSummary
				best = &s
			}
		}
		ov.DefaultPipeline = best
		if o.SkipTags {
			ov.TagKnown = false
		} else if tags := c.data.tags[p.FullPath]; len(tags) > 0 {
			ov.LastTag = tags[0].Name
		}
		return gitlab.ProjectGroup[gitlab.ProjectOverview]{Project: pr.ProjectRef, Items: []gitlab.ProjectOverview{ov}, Total: 1}
	})
}

func (w workspace) MergeRequests(ctx context.Context, ps []gitlab.ProjectRef, f gitlab.MRFilter, perProject int) <-chan gitlab.BatchResult[gitlab.MergeRequest] {
	c := w.c
	if err := c.enter(ctx, "Workspace.MergeRequests", paths(ps), f, perProject); err != nil {
		return failed[gitlab.MergeRequest](ps, err)
	}
	return batches(ctx, c, ps, func(p gitlab.ProjectRef) gitlab.ProjectGroup[gitlab.MergeRequest] {
		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.ProjectsByPath[p.FullPath]; !ok {
			return gitlab.ProjectGroup[gitlab.MergeRequest]{Project: p, Total: -1, Err: notFound("project " + p.FullPath)}
		}
		var all []gitlab.MergeRequest
		for _, mr := range c.data.mrs[p.FullPath] {
			if matchMR(mr, f, c.User.Username) {
				all = append(all, row(c.withMe(mr)))
			}
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
		g := gitlab.ProjectGroup[gitlab.MergeRequest]{Project: p, Total: len(all)}
		g.Items = all[:min(len(all), perProject)]
		return g
	})
}

func (w workspace) Pipelines(ctx context.Context, ps []gitlab.ProjectRef, f gitlab.PipelineFilter, perProject int) <-chan gitlab.BatchResult[gitlab.PipelineSummary] {
	c := w.c
	if err := c.enter(ctx, "Workspace.Pipelines", paths(ps), f, perProject); err != nil {
		return failed[gitlab.PipelineSummary](ps, err)
	}
	return batches(ctx, c, ps, func(p gitlab.ProjectRef) gitlab.ProjectGroup[gitlab.PipelineSummary] {
		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.ProjectsByPath[p.FullPath]; !ok {
			return gitlab.ProjectGroup[gitlab.PipelineSummary]{Project: p, Total: -1, Err: notFound("project " + p.FullPath)}
		}
		var all []gitlab.PipelineSummary
		for _, pl := range c.data.pipes[p.FullPath] {
			if (f.Ref == "" || pl.Ref == f.Ref) && (f.Status == "" || string(pl.Status) == f.Status) &&
				(f.Username == "" || (pl.User != nil && pl.User.Username == f.Username)) {
				all = append(all, pl.PipelineSummary)
			}
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].ID > all[j].ID })
		g := gitlab.ProjectGroup[gitlab.PipelineSummary]{Project: p, Total: len(all)}
		g.Items = all[:min(len(all), perProject)]
		return g
	})
}

func paths(ps []gitlab.ProjectRef) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.FullPath
	}
	return out
}

func (s projects) Starred(ctx context.Context) ([]gitlab.Project, error) {
	if err := s.c.enter(ctx, "Projects.Starred"); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	var out []gitlab.Project
	for _, path := range s.c.starred {
		p, ok := s.c.ProjectsByPath[path]
		if !ok {
			p = gitlab.Project{ProjectRef: gitlab.ProjectRef{FullPath: path}}
		}
		p.Starred = true
		out = append(out, p)
	}
	return out, nil
}

func (s projects) ListGroupProjects(ctx context.Context, groupID int64, limit int) ([]gitlab.Project, int, error) {
	if err := s.c.enter(ctx, "Projects.ListGroupProjects", groupID, limit); err != nil {
		return nil, 0, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	all, ok := s.c.groupProjects[groupID]
	if !ok {
		return nil, 0, notFound("group")
	}
	var out []gitlab.Project
	for _, path := range all[:min(len(all), limit)] {
		p, ok := s.c.ProjectsByPath[path]
		if !ok {
			p = gitlab.Project{ProjectRef: gitlab.ProjectRef{FullPath: path}}
		}
		out = append(out, p)
	}
	if c := s.c; c.unknownTotal[groupID] && len(all) > limit {
		return out, -1, nil
	}
	return out, len(all), nil
}

type search struct{ c *Client }

// Search matches q as a case-insensitive substring of project and group
// paths: up to 10 projects (most active first) and 5 groups.
func (s search) Search(ctx context.Context, q string) ([]gitlab.Project, []gitlab.Group, error) {
	if err := s.c.enter(ctx, "Search.Search", q); err != nil {
		return nil, nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	q = strings.ToLower(q)
	var ps []gitlab.Project
	for path, p := range s.c.ProjectsByPath {
		if strings.Contains(strings.ToLower(path), q) {
			ps = append(ps, p)
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if !ps[i].LastActivityAt.Equal(ps[j].LastActivityAt) {
			return ps[i].LastActivityAt.After(ps[j].LastActivityAt)
		}
		return ps[i].FullPath < ps[j].FullPath
	})
	var gs []gitlab.Group
	for _, g := range s.c.groups {
		if strings.Contains(strings.ToLower(g.FullPath), q) && len(gs) < 5 {
			gs = append(gs, g)
		}
	}
	return ps[:min(len(ps), 10)], gs, nil
}
