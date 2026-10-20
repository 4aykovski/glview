package rest

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
)

// commitTitleTTL is how long a commit title stays cached; commits never
// change, the limit only bounds memory.
const commitTitleTTL = 10 * time.Minute

type titleEntry struct {
	title string
	at    time.Time
}

// Pipelines: lists and stage previews come from GraphQL (package hybrid);
// this file holds the REST part — the pipeline with its jobs, and mutations.
type pipelines struct{ c *Client }

func (pipelines) List(context.Context, gitlab.ProjectRef, gitlab.PipelineFilter, string, int) (gitlab.Page[gitlab.PipelineSummary], error) {
	return gitlab.Page[gitlab.PipelineSummary]{}, errGraphQLOnly
}

func (pipelines) Stages(context.Context, gitlab.ProjectRef, int64) ([]gitlab.StageSummary, error) {
	return nil, errGraphQLOnly
}

// Get returns the pipeline with its jobs (retries collapsed into
// RetryCount), bridges and the stages in pipeline order.
func (s pipelines) Get(ctx context.Context, p gitlab.ProjectRef, id int64) (gitlab.Pipeline, error) {
	api := s.c.api
	pl, _, err := api.Pipelines.GetPipeline(pid(p), id, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Pipeline{}, mapError(err)
	}
	out := gitlab.Pipeline{PipelineSummary: s.c.mapPipeline(p, pl), YAMLErrors: pl.YamlErrors}

	yes := true
	opt := &gl.ListJobsOptions{ListOptions: gl.ListOptions{PerPage: 100, Page: 1}, IncludeRetried: &yes}
	var all []gitlab.Job
	for {
		page, resp, err := api.Jobs.ListPipelineJobs(pid(p), id, opt, gl.WithContext(ctx))
		if err != nil {
			return gitlab.Pipeline{}, mapError(err)
		}
		for _, j := range page {
			all = append(all, mapJob(j))
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	out.Jobs = collapseRetries(all)

	bopt := &gl.ListJobsOptions{ListOptions: gl.ListOptions{PerPage: 100, Page: 1}}
	for {
		page, resp, err := api.Jobs.ListPipelineBridges(pid(p), id, bopt, gl.WithContext(ctx))
		if err != nil {
			return gitlab.Pipeline{}, mapError(err)
		}
		for _, b := range page {
			out.Bridges = append(out.Bridges, s.c.mapBridge(b))
		}
		if resp.NextPage == 0 {
			break
		}
		bopt.Page = resp.NextPage
	}
	sort.SliceStable(out.Bridges, func(i, j int) bool { return out.Bridges[i].ID < out.Bridges[j].ID })
	out.Stages = orderedStages(out.Jobs, out.Bridges)

	out.CommitTitle = s.c.commitTitle(ctx, p, pl.SHA)
	return out, nil
}

// collapseRetries keeps the newest job per name and counts the older ones.
// The result is ordered by ID, which is the order GitLab created them in
// (and therefore the stage order).
func collapseRetries(jobs []gitlab.Job) []gitlab.Job {
	slices.SortStableFunc(jobs, func(a, b gitlab.Job) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	idx := map[string]int{}
	var out []gitlab.Job
	for _, j := range jobs {
		key := j.Stage + "\x00" + j.Name
		if i, ok := idx[key]; ok {
			j.RetryCount = out[i].RetryCount + 1
			out[i] = j
			continue
		}
		idx[key] = len(out)
		out = append(out, j)
	}
	return out
}

// orderedStages lists the stages in pipeline order (first appearance by
// job ID, bridges included) with their progress.
func orderedStages(jobs []gitlab.Job, bridges []gitlab.Bridge) []gitlab.StageSummary {
	type item struct {
		id     int64
		stage  string
		status gitlab.PipelineStatus
		dur    time.Duration
	}
	var items []item
	for _, j := range jobs {
		items = append(items, item{j.ID, j.Stage, j.Status, j.Duration})
	}
	for _, b := range bridges {
		items = append(items, item{b.ID, b.Stage, b.Status, b.Duration})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].id < items[j].id })
	var order []string
	by := map[string][]gitlab.PipelineStatus{}
	dur := map[string]time.Duration{}
	for _, it := range items {
		if _, ok := by[it.stage]; !ok {
			order = append(order, it.stage)
		}
		by[it.stage] = append(by[it.stage], it.status)
		dur[it.stage] += it.dur
	}
	out := make([]gitlab.StageSummary, len(order))
	for i, name := range order {
		out[i] = gitlab.NewStage(name, by[name])
		out[i].Duration = dur[name]
	}
	return out
}

func (c *Client) mapPipeline(p gitlab.ProjectRef, pl *gl.Pipeline) gitlab.PipelineSummary {
	s := gitlab.PipelineSummary{
		Project: p, ID: pl.ID, IID: pl.IID, Status: gitlab.PipelineStatus(pl.Status), Ref: pl.Ref, SHA: pl.SHA,
		Source: string(pl.Source), Duration: time.Duration(pl.Duration) * time.Second,
		CreatedAt: deref(pl.CreatedAt), WebURL: pl.WebURL,
	}
	if pl.FinishedAt != nil {
		s.FinishedAt = pl.FinishedAt
	}
	if pl.User != nil {
		s.User = &gitlab.User{ID: pl.User.ID, Username: pl.User.Username, Name: pl.User.Name, WebURL: pl.User.WebURL}
	}
	return s
}

func mapJob(j *gl.Job) gitlab.Job {
	out := gitlab.Job{
		ID: j.ID, Name: j.Name, Stage: j.Stage, Status: gitlab.PipelineStatus(j.Status), AllowFailure: j.AllowFailure,
		Duration: time.Duration(j.Duration * float64(time.Second)), Runner: j.Runner.Description, WebURL: j.WebURL,
		StartedAt: j.StartedAt,
	}
	if out.Runner == "" {
		out.Runner = j.Runner.Name
	}
	return out
}

func (c *Client) mapBridge(b *gl.Bridge) gitlab.Bridge {
	out := gitlab.Bridge{Job: gitlab.Job{
		ID: b.ID, Name: b.Name, Stage: b.Stage, Status: gitlab.PipelineStatus(b.Status), AllowFailure: b.AllowFailure,
		Duration: time.Duration(b.Duration * float64(time.Second)), WebURL: b.WebURL, StartedAt: b.StartedAt,
	}}
	if d := b.DownstreamPipeline; d != nil {
		path := c.projectPathFromURL(d.WebURL)
		out.DownstreamProject = path
		out.Downstream = &gitlab.PipelineSummary{
			Project: gitlab.ProjectRef{ID: d.ProjectID, FullPath: path}, ID: d.ID, IID: d.IID,
			Status: gitlab.PipelineStatus(d.Status), Ref: d.Ref, SHA: d.SHA, Source: d.Source,
			CreatedAt: deref(d.CreatedAt), WebURL: d.WebURL,
		}
	}
	return out
}

// projectPathFromURL extracts "group/project" from a pipeline web URL
// such as https://host/prefix/group/sub/project/-/pipelines/5521.
func (c *Client) projectPathFromURL(webURL string) string {
	rest, ok := strings.CutPrefix(webURL, c.baseURL+"/")
	if !ok {
		return ""
	}
	path, _, found := strings.Cut(rest, "/-/")
	if !found {
		return ""
	}
	return path
}

// commitTitle returns the title of a commit, best effort: errors give "".
func (c *Client) commitTitle(ctx context.Context, p gitlab.ProjectRef, sha string) string {
	if sha == "" {
		return ""
	}
	key := p.Key() + "@" + sha
	c.titleMu.Lock()
	e, ok := c.titles[key]
	c.titleMu.Unlock()
	if ok && time.Since(e.at) < commitTitleTTL {
		return e.title
	}
	commit, _, err := c.api.Commits.GetCommit(pid(p), sha, nil, gl.WithContext(ctx))
	if err != nil || commit == nil {
		return ""
	}
	c.titleMu.Lock()
	c.titles[key] = titleEntry{title: commit.Title, at: time.Now()}
	c.titleMu.Unlock()
	return commit.Title
}

func (s pipelines) Create(ctx context.Context, p gitlab.ProjectRef, ref string, vars []gitlab.CIVariable) (gitlab.PipelineSummary, error) {
	opt := &gl.CreatePipelineOptions{Ref: &ref}
	if len(vars) > 0 {
		list := make([]*gl.PipelineVariableOptions, len(vars))
		for i, v := range vars {
			list[i] = &gl.PipelineVariableOptions{Key: gl.Ptr(v.Key), Value: gl.Ptr(v.Value), VariableType: variableType(v.Type)}
		}
		opt.Variables = &list
	}
	pl, _, err := s.c.api.Pipelines.CreatePipeline(pid(p), opt, gl.WithContext(ctx))
	if err != nil {
		return gitlab.PipelineSummary{}, mapError(err)
	}
	return s.c.mapPipeline(p, pl), nil
}

func variableType(t gitlab.VarType) *gl.VariableTypeValue {
	if t == gitlab.VarFile {
		return gl.Ptr(gl.FileVariableType)
	}
	return gl.Ptr(gl.EnvVariableType)
}

func (s pipelines) Retry(ctx context.Context, p gitlab.ProjectRef, id int64) error {
	_, _, err := s.c.api.Pipelines.RetryPipelineBuild(pid(p), id, gl.WithContext(ctx))
	return mapError(err)
}

func (s pipelines) Cancel(ctx context.Context, p gitlab.ProjectRef, id int64) error {
	_, _, err := s.c.api.Pipelines.CancelPipelineBuild(pid(p), id, gl.WithContext(ctx))
	return mapError(err)
}
