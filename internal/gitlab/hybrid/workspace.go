package hybrid

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/graphql"
)

const (
	rootRefTTL  = 24 * time.Hour   // default branches rarely change (SPEC §5.8)
	lastTagTTL  = 10 * time.Minute // latest tag of a workspace project
	tagParallel = 4                // REST tag requests in flight
	tagTimeout  = 15 * time.Second
)

type workspace struct{ c *Client }

func (c *Client) Workspace() gitlab.WorkspaceService { return workspace{c} }

func paths(ps []gitlab.ProjectRef) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.FullPath
	}
	return out
}

func (w workspace) MergeRequests(ctx context.Context, ps []gitlab.ProjectRef, f gitlab.MRFilter, perProject int) <-chan gitlab.BatchResult[gitlab.MergeRequest] {
	return w.c.gql.WorkspaceMRs(ctx, paths(ps), f, w.c.me, perProject)
}

func (w workspace) Pipelines(ctx context.Context, ps []gitlab.ProjectRef, f gitlab.PipelineFilter, perProject int) <-chan gitlab.BatchResult[gitlab.PipelineSummary] {
	return w.c.gql.WorkspacePipelines(ctx, paths(ps), f, perProject)
}

// overviewRun is one ProjectsOverview call: it merges the phases and tags
// of every project and reports the merged row after each change.
type overviewRun struct {
	c      *Client
	ctx    context.Context
	out    chan gitlab.BatchResult[gitlab.ProjectOverview]
	force  bool
	noTags bool // OverviewOptions.SkipTags

	mu     sync.Mutex
	state  map[string]gitlab.ProjectOverview
	tagged map[string]bool
	tagSem chan struct{}
	wg     sync.WaitGroup
}

// ProjectsOverview runs phase 1 and phase 2 (SPEC §5.2). Projects whose
// default branch is known (o.RootRefs or the 24-hour cache) get both phases
// in one request per batch; the others get phase 1 first and then phase 2
// for those with a default branch. The latest tag is fetched over REST per
// project, at most four at a time, cached for 10 minutes.
func (w workspace) ProjectsOverview(ctx context.Context, ps []gitlab.ProjectRef, o gitlab.OverviewOptions) <-chan gitlab.BatchResult[gitlab.ProjectOverview] {
	run := &overviewRun{
		c: w.c, ctx: ctx, force: o.ForceTags, noTags: o.SkipTags,
		out:    make(chan gitlab.BatchResult[gitlab.ProjectOverview]),
		state:  map[string]gitlab.ProjectOverview{},
		tagged: map[string]bool{},
		tagSem: make(chan struct{}, tagParallel),
	}
	refs := map[string]string{}
	var known, unknown []string
	for _, p := range ps {
		ref := o.RootRefs[p.FullPath]
		if ref == "" {
			ref, _ = cache.Lookup[string](w.c.cache, rootRefKey(p.FullPath), rootRefTTL)
		}
		if ref != "" {
			refs[p.FullPath] = ref
			known = append(known, p.FullPath)
		} else {
			unknown = append(unknown, p.FullPath)
		}
	}
	go func() {
		defer close(run.out)
		if len(known) > 0 {
			run.wg.Add(1)
			go func() {
				defer run.wg.Done()
				req := graphql.OverviewRequest{Paths: known, RootRefs: refs, Me: w.c.me, Phase1: true, Phase2: true}
				for res := range w.c.gql.Overview(ctx, req) {
					run.later(run.phase1(res, refs))
				}
			}()
		}
		if len(unknown) > 0 {
			run.wg.Add(1)
			go func() {
				defer run.wg.Done()
				req := graphql.OverviewRequest{Paths: unknown, Me: w.c.me, Phase1: true}
				for res := range w.c.gql.Overview(ctx, req) {
					run.later(run.phase1(res, nil))
				}
			}()
		}
		run.wg.Wait()
	}()
	return run.out
}

func rootRefKey(path string) string { return "rootref:" + path }

func (r *overviewRun) send(res gitlab.BatchResult[gitlab.ProjectOverview]) {
	select {
	case r.out <- res:
	case <-r.ctx.Done():
	}
}

// phase1 records a batch that carried phase 1 (and maybe phase 2 with the
// default branches used), reports it and returns the default branches of
// the projects that still need phase 2.
func (r *overviewRun) phase1(res gitlab.BatchResult[gitlab.ProjectOverview], used map[string]string) map[string]string {
	need := map[string]string{}
	r.mu.Lock()
	for i, g := range res.Groups {
		if len(g.Items) == 0 {
			continue // project: null or an error without data
		}
		ov := g.Items[0]
		path := g.Project.FullPath
		if g.Err != nil {
			// A field error (no CI access…) with the project's data: the
			// row stays, the error goes to the debug log (SPEC §5.2).
			debuglog.L().Debug("workspace overview: partial error", "project", path, "err", g.Err)
			res.Groups[i].Err = nil
		}
		if ov.DefaultBranch != "" {
			r.c.cache.Set(rootRefKey(path), ov.DefaultBranch)
		}
		switch {
		case ov.DefaultBranch == "":
			ov.Phase = 2 // an empty repository has no default branch pipeline
			ov.DefaultPipeline = nil
		case used != nil && used[path] != ov.DefaultBranch:
			ov.Phase, ov.DefaultPipeline = 1, nil // the default branch changed
			need[path] = ov.DefaultBranch
		case used == nil:
			need[path] = ov.DefaultBranch
		}
		if old, ok := r.state[path]; ok && old.TagKnown {
			ov.LastTag, ov.TagKnown = old.LastTag, true
		}
		r.state[path] = ov
		res.Groups[i].Project = ov.ProjectRef
		res.Groups[i].Items = []gitlab.ProjectOverview{ov}
		r.startTag(ov.ProjectRef)
	}
	r.mu.Unlock()
	r.send(res)
	return need
}

// later runs phase 2 for refs without holding up the next batch.
func (r *overviewRun) later(refs map[string]string) {
	if len(refs) == 0 {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.phase2(refs)
	}()
}

// phase2 asks the default branch pipelines of the given projects.
func (r *overviewRun) phase2(refs map[string]string) {
	if len(refs) == 0 || r.ctx.Err() != nil {
		return
	}
	ps := make([]string, 0, len(refs))
	for p := range refs {
		ps = append(ps, p)
	}
	req := graphql.OverviewRequest{Paths: ps, RootRefs: refs, Phase2: true}
	for res := range r.c.gql.Overview(r.ctx, req) {
		var groups []gitlab.ProjectGroup[gitlab.ProjectOverview]
		r.mu.Lock()
		for _, g := range res.Groups {
			path := g.Project.FullPath
			ov, ok := r.state[path]
			if !ok {
				continue
			}
			if g.Err != nil {
				debuglog.L().Debug("workspace overview: phase 2 failed", "project", path, "err", g.Err)
			}
			if len(g.Items) > 0 {
				ov.DefaultPipeline = g.Items[0].DefaultPipeline
			}
			ov.Phase = 2
			r.state[path] = ov
			groups = append(groups, gitlab.ProjectGroup[gitlab.ProjectOverview]{Project: ov.ProjectRef, Items: []gitlab.ProjectOverview{ov}, Total: 1})
		}
		r.mu.Unlock()
		r.send(gitlab.BatchResult[gitlab.ProjectOverview]{Groups: groups})
	}
}

// startTag fetches the latest tag of p once per run (the caller holds mu).
func (r *overviewRun) startTag(p gitlab.ProjectRef) {
	if r.noTags || r.tagged[p.FullPath] {
		return
	}
	r.tagged[p.FullPath] = true
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		select {
		case r.tagSem <- struct{}{}:
		case <-r.ctx.Done():
			return
		}
		tag, err := r.c.latestTag(r.ctx, p, r.force)
		<-r.tagSem
		if err != nil {
			debuglog.L().Debug("workspace overview: latest tag failed", "project", p.FullPath, "err", err)
			return
		}
		r.mu.Lock()
		ov := r.state[p.FullPath]
		ov.LastTag, ov.TagKnown = tag, true
		r.state[p.FullPath] = ov
		r.mu.Unlock()
		r.send(gitlab.BatchResult[gitlab.ProjectOverview]{Groups: []gitlab.ProjectGroup[gitlab.ProjectOverview]{
			{Project: ov.ProjectRef, Items: []gitlab.ProjectOverview{ov}, Total: 1},
		}})
	}()
}

// latestTag reads the latest tag through the 10-minute cache; force (r)
// always asks GitLab.
func (c *Client) latestTag(ctx context.Context, p gitlab.ProjectRef, force bool) (string, error) {
	key := "lasttag:" + p.FullPath
	if !force {
		if v, ok := cache.Lookup[string](c.cache, key, lastTagTTL); ok {
			return v, nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, tagTimeout)
	defer cancel()
	tag, err := c.rest.LatestTag(ctx, p)
	if err != nil {
		return "", err
	}
	c.cache.Set(key, tag)
	return tag, nil
}

type search struct{ c *Client }

func (c *Client) Search() gitlab.SearchService { return search{c} }

// Search asks GraphQL and falls back to REST when GraphQL fails (an old
// instance without searchNamespaces, SPEC §5.2).
func (s search) Search(ctx context.Context, q string) ([]gitlab.Project, []gitlab.Group, error) {
	ps, gs, err := s.c.gql.Search(ctx, q)
	if err == nil || ctx.Err() != nil || errors.Is(err, gitlab.ErrUnauthorized) {
		return ps, gs, err
	}
	debuglog.L().Info("search: GraphQL failed, using REST", "err", err)
	return s.c.rest.Search().Search(ctx, q)
}
