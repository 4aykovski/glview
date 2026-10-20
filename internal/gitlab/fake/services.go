package fake

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

// data holds the fake's in-memory GitLab state, guarded by Client.mu.
type data struct {
	mrs        map[string][]gitlab.MergeRequest // by project path
	discs      map[string][]gitlab.Discussion   // key: path!iid
	mrPipes    map[string][]gitlab.PipelineSummary
	pipes      map[string][]gitlab.Pipeline // by project path
	traces     map[int64][]byte
	branches   map[string][]string
	tags       map[string][]gitlab.Tag
	ciVars     map[string][]gitlab.CIConfigVariable // key: path@ref
	members    map[string][]gitlab.User
	labels     map[string][]string
	recheck    map[string]string // key: path!iid -> final status after a recheck
	rechecking map[string]bool
	nextJobID  int64
	nextPipeID int64
}

func newData() data {
	return data{
		mrs:        map[string][]gitlab.MergeRequest{},
		discs:      map[string][]gitlab.Discussion{},
		mrPipes:    map[string][]gitlab.PipelineSummary{},
		pipes:      map[string][]gitlab.Pipeline{},
		traces:     map[int64][]byte{},
		branches:   map[string][]string{},
		tags:       map[string][]gitlab.Tag{},
		ciVars:     map[string][]gitlab.CIConfigVariable{},
		members:    map[string][]gitlab.User{},
		labels:     map[string][]string{},
		recheck:    map[string]string{},
		rechecking: map[string]bool{},
		nextJobID:  900000,
		nextPipeID: 900000,
	}
}

func mrKey(path string, iid int64) string { return path + "!" + strconv.FormatInt(iid, 10) }

// AddMR registers a merge request of project path.
func (c *Client) AddMR(path string, mr gitlab.MergeRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mr.Project.FullPath == "" {
		mr.Project.FullPath = path
	}
	c.data.mrs[path] = append(c.data.mrs[path], mr)
}

// UpdateMR changes a registered merge request in place.
func (c *Client) UpdateMR(path string, iid int64, fn func(*gitlab.MergeRequest)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.data.mrs[path] {
		if c.data.mrs[path][i].IID == iid {
			fn(&c.data.mrs[path][i])
		}
	}
}

// SetDiscussions replaces the discussions of an MR.
func (c *Client) SetDiscussions(path string, iid int64, d []gitlab.Discussion) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.discs[mrKey(path, iid)] = d
}

// SetMRPipelines replaces the pipelines listed for an MR.
func (c *Client) SetMRPipelines(path string, iid int64, p []gitlab.PipelineSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.mrPipes[mrKey(path, iid)] = p
}

// AddPipeline registers a pipeline (newest first by ID order of insertion).
func (c *Client) AddPipeline(path string, p gitlab.Pipeline) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p.Project.FullPath == "" {
		p.Project.FullPath = path
	}
	c.data.pipes[path] = append(c.data.pipes[path], p)
}

// UpdatePipeline changes a registered pipeline in place.
func (c *Client) UpdatePipeline(path string, id int64, fn func(*gitlab.Pipeline)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.data.pipes[path] {
		if c.data.pipes[path][i].ID == id {
			fn(&c.data.pipes[path][i])
		}
	}
}

// SetTrace replaces the log of a job.
func (c *Client) SetTrace(jobID int64, s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.traces[jobID] = []byte(s)
}

// AppendTrace appends to the log of a job (a running job writing output).
func (c *Client) AppendTrace(jobID int64, s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.traces[jobID] = append(c.data.traces[jobID], s...)
}

// SetBranches registers the branch names of a project.
func (c *Client) SetBranches(path string, names ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.branches[path] = names
}

// SetTags registers the tags of a project.
func (c *Client) SetTags(path string, tags ...gitlab.Tag) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.tags[path] = tags
}

// SetCIVariables registers the .gitlab-ci.yml variable hints for ref.
func (c *Client) SetCIVariables(path, ref string, vars ...gitlab.CIConfigVariable) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.ciVars[path+"@"+ref] = vars
}

func notFound(what string) error {
	return &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404, Message: what}
}

// pathOf resolves a project path from a ref that may carry only the ID.
// The caller holds c.mu.
func (c *Client) pathOf(ref gitlab.ProjectRef) string {
	if ref.FullPath != "" {
		return ref.FullPath
	}
	for path, p := range c.ProjectsByPath {
		if p.ID == ref.ID {
			return path
		}
	}
	return ""
}

func (c *Client) findMR(path string, iid int64) (*gitlab.MergeRequest, error) {
	for i := range c.data.mrs[path] {
		if c.data.mrs[path][i].IID == iid {
			return &c.data.mrs[path][i], nil
		}
	}
	return nil, notFound(fmt.Sprintf("merge request !%d", iid))
}

// withMe returns a copy of mr with ApprovedByMe computed for the fake user.
func (c *Client) withMe(mr gitlab.MergeRequest) gitlab.MergeRequest {
	mr.Approvals.ApprovedBy = slices.Clone(mr.Approvals.ApprovedBy)
	mr.Approvals.ApprovedByMe = slices.ContainsFunc(mr.Approvals.ApprovedBy, func(u gitlab.User) bool {
		return u.Username == c.User.Username
	})
	return mr
}

type mrService struct{ c *Client }

func (s mrService) List(ctx context.Context, p gitlab.ProjectRef, f gitlab.MRFilter, cursor string, limit int) (gitlab.Page[gitlab.MergeRequest], error) {
	if err := s.c.enter(ctx, "MergeRequests.List", p.FullPath, f, cursor, limit); err != nil {
		return gitlab.Page[gitlab.MergeRequest]{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	if _, ok := s.c.ProjectsByPath[path]; !ok && len(s.c.data.mrs[path]) == 0 {
		return gitlab.Page[gitlab.MergeRequest]{}, notFound("project " + path)
	}
	me := s.c.User.Username
	var all []gitlab.MergeRequest
	for _, mr := range s.c.data.mrs[path] {
		if matchMR(mr, f, me) {
			all = append(all, s.c.withMe(mr))
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	return paginate(all, cursor, limit), nil
}

func matchMR(mr gitlab.MergeRequest, f gitlab.MRFilter, me string) bool {
	if f.State != "" && f.State != gitlab.MRStateAll && mr.State != f.State {
		return false
	}
	isAuthor := mr.Author.Username == me
	isAssignee := slices.ContainsFunc(mr.Assignees, func(u gitlab.User) bool { return u.Username == me })
	isReviewer := slices.ContainsFunc(mr.Reviewers, func(u gitlab.User) bool { return u.Username == me })
	if f.Mine && !isAuthor && !isAssignee && !isReviewer {
		return false
	}
	switch f.Scope {
	case gitlab.MRScopeCreatedByMe:
		if !isAuthor {
			return false
		}
	case gitlab.MRScopeAssignedToMe:
		if !isAssignee {
			return false
		}
	}
	switch f.Reviewer {
	case "":
	case "@me":
		if !isReviewer {
			return false
		}
	default:
		if !slices.ContainsFunc(mr.Reviewers, func(u gitlab.User) bool { return u.Username == f.Reviewer }) {
			return false
		}
	}
	if f.Draft != nil && mr.Draft != *f.Draft {
		return false
	}
	for _, l := range f.Labels {
		if !slices.Contains(mr.Labels, l) {
			return false
		}
	}
	if f.Search != "" && !strings.Contains(strings.ToLower(mr.Title), strings.ToLower(f.Search)) {
		return false
	}
	return true
}

// paginate cuts a page out of all; the cursor is the offset of the first item.
func paginate[T any](all []T, cursor string, limit int) gitlab.Page[T] {
	start, _ := strconv.Atoi(cursor)
	start = min(max(start, 0), len(all))
	if limit <= 0 {
		limit = len(all)
	}
	end := min(start+limit, len(all))
	page := gitlab.Page[T]{Items: slices.Clone(all[start:end]), Total: len(all)}
	if end < len(all) {
		page.NextCursor = strconv.Itoa(end)
	}
	return page
}

func (s mrService) Get(ctx context.Context, p gitlab.ProjectRef, iid int64) (gitlab.MergeRequest, error) {
	if err := s.c.enter(ctx, "MergeRequests.Get", p.FullPath, iid); err != nil {
		return gitlab.MergeRequest{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	mr, err := s.c.findMR(path, iid)
	if err != nil {
		return gitlab.MergeRequest{}, err
	}
	// A recheck in flight finishes with the scripted status on the next read.
	if k := mrKey(path, iid); s.c.data.rechecking[k] && mr.MergeStatus == "checking" {
		if st, ok := s.c.data.recheck[k]; ok {
			mr.MergeStatus = st
			delete(s.c.data.rechecking, k)
		}
	}
	return s.c.withMe(*mr), nil
}

func (s mrService) Discussions(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.Discussion, error) {
	if err := s.c.enter(ctx, "MergeRequests.Discussions", p.FullPath, iid); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return slices.Clone(s.c.data.discs[mrKey(s.c.pathOf(p), iid)]), nil
}

func (s mrService) Pipelines(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.PipelineSummary, error) {
	if err := s.c.enter(ctx, "MergeRequests.Pipelines", p.FullPath, iid); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return slices.Clone(s.c.data.mrPipes[mrKey(s.c.pathOf(p), iid)]), nil
}

func (s mrService) Approve(ctx context.Context, p gitlab.ProjectRef, iid int64, sha string) error {
	if err := s.c.enter(ctx, "MergeRequests.Approve", p.FullPath, iid, sha); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	mr, err := s.c.findMR(s.c.pathOf(p), iid)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(mr.Approvals.ApprovedBy, func(u gitlab.User) bool { return u.Username == s.c.User.Username }) {
		mr.Approvals.ApprovedBy = append(mr.Approvals.ApprovedBy, s.c.User)
		mr.Approvals.Left = max(0, mr.Approvals.Left-1)
	}
	return nil
}

func (s mrService) Unapprove(ctx context.Context, p gitlab.ProjectRef, iid int64) error {
	if err := s.c.enter(ctx, "MergeRequests.Unapprove", p.FullPath, iid); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	mr, err := s.c.findMR(s.c.pathOf(p), iid)
	if err != nil {
		return err
	}
	before := len(mr.Approvals.ApprovedBy)
	mr.Approvals.ApprovedBy = slices.DeleteFunc(slices.Clone(mr.Approvals.ApprovedBy), func(u gitlab.User) bool {
		return u.Username == s.c.User.Username
	})
	if len(mr.Approvals.ApprovedBy) < before && mr.Approvals.Required > 0 {
		mr.Approvals.Left++
	}
	return nil
}

func (s mrService) Merge(ctx context.Context, p gitlab.ProjectRef, iid int64, o gitlab.MergeOptions) (gitlab.MergeRequest, error) {
	if err := s.c.enter(ctx, "MergeRequests.Merge", p.FullPath, iid, o); err != nil {
		return gitlab.MergeRequest{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	mr, err := s.c.findMR(s.c.pathOf(p), iid)
	if err != nil {
		return gitlab.MergeRequest{}, err
	}
	if !o.AutoMerge {
		mr.State = gitlab.MRStateMerged
		mr.MergedAt = s.c.now()
		by := s.c.User
		mr.MergedBy = &by
	}
	return s.c.withMe(*mr), nil
}

// SetRecheckResult scripts the status an "unchecked" MR gets once a recheck
// was requested: RecheckMergeStatus moves it to "checking" and the next Get
// reports status.
func (c *Client) SetRecheckResult(path string, iid int64, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.recheck[mrKey(path, iid)] = status
}

// RecheckMergeStatus turns "unchecked" into "checking"; any other status is
// returned unchanged.
func (s mrService) RecheckMergeStatus(ctx context.Context, p gitlab.ProjectRef, iid int64) (string, error) {
	if err := s.c.enter(ctx, "MergeRequests.RecheckMergeStatus", p.FullPath, iid); err != nil {
		return "", err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	mr, err := s.c.findMR(path, iid)
	if err != nil {
		return "", err
	}
	if mr.MergeStatus == "unchecked" {
		mr.MergeStatus = "checking"
		s.c.data.rechecking[mrKey(path, iid)] = true
	}
	return mr.MergeStatus, nil
}

func (s mrService) Rebase(ctx context.Context, p gitlab.ProjectRef, iid int64) error {
	if err := s.c.enter(ctx, "MergeRequests.Rebase", p.FullPath, iid); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	mr, err := s.c.findMR(s.c.pathOf(p), iid)
	if err != nil {
		return err
	}
	mr.RebaseInProgress = true
	return nil
}

type pipelineService struct{ c *Client }

func (s pipelineService) List(ctx context.Context, p gitlab.ProjectRef, f gitlab.PipelineFilter, cursor string, limit int) (gitlab.Page[gitlab.PipelineSummary], error) {
	if err := s.c.enter(ctx, "Pipelines.List", p.FullPath, f, cursor, limit); err != nil {
		return gitlab.Page[gitlab.PipelineSummary]{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	var all []gitlab.PipelineSummary
	for _, pl := range s.c.data.pipes[path] {
		if f.Ref != "" && pl.Ref != f.Ref {
			continue
		}
		if f.Status != "" && string(pl.Status) != f.Status {
			continue
		}
		if f.Source != "" && pl.Source != f.Source {
			continue
		}
		if f.Username != "" && (pl.User == nil || pl.User.Username != f.Username) {
			continue
		}
		all = append(all, pl.PipelineSummary)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	return paginate(all, cursor, limit), nil
}

func (c *Client) findPipeline(path string, id int64) (*gitlab.Pipeline, error) {
	for i := range c.data.pipes[path] {
		if c.data.pipes[path][i].ID == id {
			return &c.data.pipes[path][i], nil
		}
	}
	return nil, notFound(fmt.Sprintf("pipeline #%d", id))
}

func (s pipelineService) Get(ctx context.Context, p gitlab.ProjectRef, id int64) (gitlab.Pipeline, error) {
	if err := s.c.enter(ctx, "Pipelines.Get", p.FullPath, id); err != nil {
		return gitlab.Pipeline{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	pl, err := s.c.findPipeline(s.c.pathOf(p), id)
	if err != nil {
		return gitlab.Pipeline{}, err
	}
	out := *pl
	out.Jobs = slices.Clone(pl.Jobs)
	out.Bridges = slices.Clone(pl.Bridges)
	if len(out.Stages) == 0 {
		out.Stages = stagesFromJobs(out)
	}
	return out, nil
}

func (s pipelineService) Stages(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.StageSummary, error) {
	if err := s.c.enter(ctx, "Pipelines.Stages", p.FullPath, iid); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	for _, pl := range s.c.data.pipes[s.c.pathOf(p)] {
		if pl.IID == iid {
			if len(pl.Stages) > 0 {
				return slices.Clone(pl.Stages), nil
			}
			return stagesFromJobs(pl), nil
		}
	}
	return nil, notFound(fmt.Sprintf("pipeline iid %d", iid))
}

// stagesFromJobs derives the ordered stage list from jobs and bridges (in
// ID order, like the real REST adapter) when none were scripted.
func stagesFromJobs(pl gitlab.Pipeline) []gitlab.StageSummary {
	type item struct {
		id     int64
		stage  string
		status gitlab.PipelineStatus
		dur    time.Duration
	}
	var items []item
	for _, j := range pl.Jobs {
		items = append(items, item{j.ID, j.Stage, j.Status, j.Duration})
	}
	for _, b := range pl.Bridges {
		items = append(items, item{b.ID, b.Stage, b.Status, b.Duration})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].id < items[j].id })
	var order []string
	byStage := map[string][]gitlab.PipelineStatus{}
	dur := map[string]time.Duration{}
	for _, it := range items {
		if _, ok := byStage[it.stage]; !ok {
			order = append(order, it.stage)
		}
		byStage[it.stage] = append(byStage[it.stage], it.status)
		dur[it.stage] += it.dur
	}
	out := make([]gitlab.StageSummary, len(order))
	for i, name := range order {
		out[i] = gitlab.NewStage(name, byStage[name])
		out[i].Duration = dur[name]
	}
	return out
}

func (s pipelineService) Create(ctx context.Context, p gitlab.ProjectRef, ref string, vars []gitlab.CIVariable) (gitlab.PipelineSummary, error) {
	if err := s.c.enter(ctx, "Pipelines.Create", p.FullPath, ref, slices.Clone(vars)); err != nil {
		return gitlab.PipelineSummary{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	s.c.data.nextPipeID++
	user := s.c.User
	pl := gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{
		Project: gitlab.ProjectRef{ID: p.ID, FullPath: path}, ID: s.c.data.nextPipeID, IID: s.c.data.nextPipeID,
		Status: gitlab.StatusPending, Ref: ref, Source: "web", User: &user, CreatedAt: s.c.now(),
	}}
	s.c.data.pipes[path] = append(s.c.data.pipes[path], pl)
	return pl.PipelineSummary, nil
}

func (s pipelineService) Retry(ctx context.Context, p gitlab.ProjectRef, id int64) error {
	if err := s.c.enter(ctx, "Pipelines.Retry", p.FullPath, id); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	pl, err := s.c.findPipeline(s.c.pathOf(p), id)
	if err != nil {
		return err
	}
	pl.Status = gitlab.StatusRunning
	for i := range pl.Jobs {
		if pl.Jobs[i].Status == gitlab.StatusFailed {
			pl.Jobs[i].Status = gitlab.StatusPending
		}
	}
	return nil
}

func (s pipelineService) Cancel(ctx context.Context, p gitlab.ProjectRef, id int64) error {
	if err := s.c.enter(ctx, "Pipelines.Cancel", p.FullPath, id); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	pl, err := s.c.findPipeline(s.c.pathOf(p), id)
	if err != nil {
		return err
	}
	pl.Status = gitlab.StatusCanceled
	for i := range pl.Jobs {
		if pl.Jobs[i].Status.Active() {
			pl.Jobs[i].Status = gitlab.StatusCanceled
		}
	}
	return nil
}

type jobService struct{ c *Client }

// findJob locates a job (or bridge) by ID across the project's pipelines.
func (c *Client) findJob(path string, id int64) (*gitlab.Job, error) {
	for i := range c.data.pipes[path] {
		pl := &c.data.pipes[path][i]
		for j := range pl.Jobs {
			if pl.Jobs[j].ID == id {
				return &pl.Jobs[j], nil
			}
			for k := range pl.Jobs[j].Attempts {
				if pl.Jobs[j].Attempts[k].ID == id {
					return &pl.Jobs[j].Attempts[k], nil
				}
			}
		}
		for j := range pl.Bridges {
			if pl.Bridges[j].ID == id {
				return &pl.Bridges[j].Job, nil
			}
		}
	}
	return nil, notFound(fmt.Sprintf("job #%d", id))
}

func (s jobService) Get(ctx context.Context, p gitlab.ProjectRef, id int64) (gitlab.Job, error) {
	if err := s.c.enter(ctx, "Jobs.Get", p.FullPath, id); err != nil {
		return gitlab.Job{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	j, err := s.c.findJob(s.c.pathOf(p), id)
	if err != nil {
		return gitlab.Job{}, err
	}
	return *j, nil
}

func (s jobService) Trace(ctx context.Context, p gitlab.ProjectRef, id int64) ([]byte, error) {
	if err := s.c.enter(ctx, "Jobs.Trace", p.FullPath, id); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return slices.Clone(s.c.data.traces[id]), nil
}

func (s jobService) Retry(ctx context.Context, p gitlab.ProjectRef, id int64) (gitlab.Job, error) {
	if err := s.c.enter(ctx, "Jobs.Retry", p.FullPath, id); err != nil {
		return gitlab.Job{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	old, err := s.c.findJob(path, id)
	if err != nil {
		return gitlab.Job{}, err
	}
	s.c.data.nextJobID++
	nj := *old
	nj.ID = s.c.data.nextJobID
	nj.Status = gitlab.StatusPending
	nj.Duration = 0
	nj.Attempts = append([]gitlab.Job{*old}, old.Attempts...)
	nj.Attempts[0].Attempts = nil
	for i := range s.c.data.pipes[path] {
		pl := &s.c.data.pipes[path][i]
		for j := range pl.Jobs {
			if pl.Jobs[j].ID == id {
				pl.Jobs[j] = nj
				return nj, nil
			}
		}
	}
	return nj, nil
}

func (s jobService) Cancel(ctx context.Context, p gitlab.ProjectRef, id int64) error {
	if err := s.c.enter(ctx, "Jobs.Cancel", p.FullPath, id); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	j, err := s.c.findJob(s.c.pathOf(p), id)
	if err != nil {
		return err
	}
	j.Status = gitlab.StatusCanceled
	return nil
}

func (s jobService) Play(ctx context.Context, p gitlab.ProjectRef, id int64, vars []gitlab.CIVariable) (gitlab.Job, error) {
	if err := s.c.enter(ctx, "Jobs.Play", p.FullPath, id, slices.Clone(vars)); err != nil {
		return gitlab.Job{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	j, err := s.c.findJob(s.c.pathOf(p), id)
	if err != nil {
		return gitlab.Job{}, err
	}
	j.Status = gitlab.StatusPending
	return *j, nil
}

type tagService struct{ c *Client }

func (s tagService) List(ctx context.Context, p gitlab.ProjectRef, search, cursor string, limit int) (gitlab.Page[gitlab.Tag], error) {
	if err := s.c.enter(ctx, "Tags.List", p.FullPath, search, cursor, limit); err != nil {
		return gitlab.Page[gitlab.Tag]{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	var all []gitlab.Tag
	for _, t := range s.c.data.tags[s.c.pathOf(p)] {
		if strings.Contains(strings.ToLower(t.Name), strings.ToLower(search)) {
			all = append(all, t)
		}
	}
	return paginate(all, cursor, limit), nil
}

func (s projects) Branches(ctx context.Context, p gitlab.ProjectRef, search string) ([]string, error) {
	if err := s.c.enter(ctx, "Projects.Branches", p.FullPath, search); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	var out []string
	for _, b := range s.c.data.branches[s.c.pathOf(p)] {
		if strings.Contains(strings.ToLower(b), strings.ToLower(search)) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (s projects) CIConfigVariables(ctx context.Context, p gitlab.ProjectRef, gitRef string) ([]gitlab.CIConfigVariable, error) {
	if err := s.c.enter(ctx, "Projects.CIConfigVariables", p.FullPath, gitRef); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return slices.Clone(s.c.data.ciVars[s.c.pathOf(p)+"@"+gitRef]), nil
}

// now is the fake's clock for created objects.
func (c *Client) now() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }

// SetMembers registers the members of a project.
func (c *Client) SetMembers(path string, users ...gitlab.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.members[path] = users
}

// SetLabels registers the label names of a project.
func (c *Client) SetLabels(path string, names ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.labels[path] = names
}

func (s projects) Members(ctx context.Context, p gitlab.ProjectRef, search string) ([]gitlab.User, error) {
	if err := s.c.enter(ctx, "Projects.Members", p.FullPath, search); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	q := strings.ToLower(search)
	var out []gitlab.User
	for _, u := range s.c.data.members[s.c.pathOf(p)] {
		if strings.Contains(strings.ToLower(u.Username), q) || strings.Contains(strings.ToLower(u.Name), q) {
			out = append(out, u)
		}
	}
	return out, nil
}

func (s projects) Labels(ctx context.Context, p gitlab.ProjectRef, search string) ([]string, error) {
	if err := s.c.enter(ctx, "Projects.Labels", p.FullPath, search); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	var out []string
	for _, l := range s.c.data.labels[s.c.pathOf(p)] {
		if strings.Contains(strings.ToLower(l), strings.ToLower(search)) {
			out = append(out, l)
		}
	}
	return out, nil
}
