package fake

import (
	"context"
	"fmt"
	"maps"
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
	mrCommits  map[string][]gitlab.CommitSummary // key: path!iid, newest first
	pipes      map[string][]gitlab.Pipeline      // by project path
	traces     map[int64][]byte
	branches   map[string][]string
	tags       map[string][]gitlab.Tag
	compares   map[string]gitlab.Comparison         // key: compareKey
	ciVars     map[string][]gitlab.CIConfigVariable // key: path@ref
	members    map[string][]gitlab.User
	labels     map[string][]string
	recheck    map[string]string // key: path!iid -> final status after a recheck
	rechecking map[string]bool
	issues     map[string][]gitlab.Issue // by project path
	inotes     map[string][]gitlab.Note  // key: path#iid, oldest first
	idiscs     map[string][]gitlab.Discussion
	irelated   map[string][]gitlab.RelatedMR
	milestones map[string][]gitlab.Milestone
	nextJobID  int64
	nextPipeID int64
	nextNoteID int64
	mrDiffs    map[string][]gitlab.DiffFile // key: path!iid; created on first SetMRDiffs
}

func newData() data {
	return data{
		mrs:        map[string][]gitlab.MergeRequest{},
		discs:      map[string][]gitlab.Discussion{},
		mrPipes:    map[string][]gitlab.PipelineSummary{},
		mrCommits:  map[string][]gitlab.CommitSummary{},
		pipes:      map[string][]gitlab.Pipeline{},
		traces:     map[int64][]byte{},
		branches:   map[string][]string{},
		tags:       map[string][]gitlab.Tag{},
		compares:   map[string]gitlab.Comparison{},
		ciVars:     map[string][]gitlab.CIConfigVariable{},
		members:    map[string][]gitlab.User{},
		labels:     map[string][]string{},
		recheck:    map[string]string{},
		rechecking: map[string]bool{},
		issues:     map[string][]gitlab.Issue{},
		inotes:     map[string][]gitlab.Note{},
		idiscs:     map[string][]gitlab.Discussion{},
		irelated:   map[string][]gitlab.RelatedMR{},
		milestones: map[string][]gitlab.Milestone{},
		nextJobID:  900000,
		nextPipeID: 900000,
		nextNoteID: 700000,
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

// SetMRCommits replaces the commits of an MR (newest first, as the API
// returns them). It does not touch MergeRequest.CommitCount.
func (c *Client) SetMRCommits(path string, iid int64, commits []gitlab.CommitSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.mrCommits[mrKey(path, iid)] = commits
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
	mr.Approvals.Rules = slices.Clone(mr.Approvals.Rules)
	for i := range mr.Approvals.Rules {
		mr.Approvals.Rules[i].ApprovedBy = slices.Clone(mr.Approvals.Rules[i].ApprovedBy)
	}
	mr.ReviewStates = maps.Clone(mr.ReviewStates)
	mr.Approvals.ApprovedByMe = slices.ContainsFunc(mr.Approvals.ApprovedBy, func(u gitlab.User) bool {
		return u.Username == c.User.Username
	})
	return mr
}

// row strips what a list row (MRRow) does not carry: the real API gives the
// HTML links in MR details only.
func row(mr gitlab.MergeRequest) gitlab.MergeRequest {
	mr.TitleLinks, mr.DescriptionLinks = nil, nil
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
			all = append(all, row(s.c.withMe(mr)))
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
	if f.SourceBranch != "" && mr.SourceBranch != f.SourceBranch {
		return false
	}
	if f.TargetBranch != "" && mr.TargetBranch != f.TargetBranch {
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

// Comment adds a note to the MR as a one-note discussion by the fake user.
func (s mrService) Comment(ctx context.Context, p gitlab.ProjectRef, iid int64, body string) (gitlab.Note, error) {
	if err := s.c.enter(ctx, "MergeRequests.Comment", p.FullPath, iid, body); err != nil {
		return gitlab.Note{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	if _, err := s.c.findMR(path, iid); err != nil {
		return gitlab.Note{}, err
	}
	n := s.c.newNote(body)
	k := mrKey(path, iid)
	s.c.data.discs[k] = append(s.c.data.discs[k], gitlab.Discussion{ID: fmt.Sprintf("n%d", n.ID), Notes: []gitlab.Note{n}})
	return n, nil
}

func (s mrService) Pipelines(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.PipelineSummary, error) {
	if err := s.c.enter(ctx, "MergeRequests.Pipelines", p.FullPath, iid); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return slices.Clone(s.c.data.mrPipes[mrKey(s.c.pathOf(p), iid)]), nil
}

func (s mrService) Commits(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.CommitSummary, error) {
	if err := s.c.enter(ctx, "MergeRequests.Commits", p.FullPath, iid); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return slices.Clone(s.c.data.mrCommits[mrKey(s.c.pathOf(p), iid)]), nil
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
		s.c.approveRules(mr, true)
	}
	return nil
}

// approveRules records (or withdraws) the fake user's approval in the rules
// they are eligible for and in the reviewer states.
func (c *Client) approveRules(mr *gitlab.MergeRequest, approve bool) {
	me := c.User
	rules := slices.Clone(mr.Approvals.Rules)
	for i := range rules {
		r := &rules[i]
		if r.Type != gitlab.ApprovalAnyApprover && !slices.ContainsFunc(r.Eligible, func(u gitlab.User) bool { return u.Username == me.Username }) {
			continue
		}
		by := slices.DeleteFunc(slices.Clone(r.ApprovedBy), func(u gitlab.User) bool { return u.Username == me.Username })
		if approve {
			by = append(by, me)
		}
		r.ApprovedBy = by
		r.Approved = len(by) >= r.Required
	}
	mr.Approvals.Rules = rules
	if _, ok := mr.ReviewStates[me.Username]; ok {
		states := maps.Clone(mr.ReviewStates)
		states[me.Username] = gitlab.ReviewUnapproved
		if approve {
			states[me.Username] = gitlab.ReviewApproved
		}
		mr.ReviewStates = states
	}
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
		s.c.approveRules(mr, false)
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
		// Other scopes (running, finished…) are ignored, as in the GraphQL client.
		switch f.Scope {
		case "tags":
			if !s.c.isTag(path, pl.Ref) {
				continue
			}
		case "branches":
			if s.c.isTag(path, pl.Ref) {
				continue
			}
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
	nj.Attempts = nil
	// As in GitLab, the new job becomes the latest attempt of its name even
	// when an earlier attempt is retried; the previous latest one moves to
	// the attempts.
	if latest := s.c.latestAttempt(path, id); latest != nil {
		nj.Attempts = latest.AllAttempts()
		*latest = nj
	}
	return nj, nil
}

// latestAttempt finds the latest attempt of the job one of whose attempts
// has the given ID; nil for a bridge or an unknown ID.
func (c *Client) latestAttempt(path string, id int64) *gitlab.Job {
	for i := range c.data.pipes[path] {
		pl := &c.data.pipes[path][i]
		for j := range pl.Jobs {
			job := &pl.Jobs[j]
			if job.ID == id || slices.ContainsFunc(job.Attempts, func(a gitlab.Job) bool { return a.ID == id }) {
				return job
			}
		}
	}
	return nil
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

// List serves the tags in the order they were registered (newest version
// first) without pipelines, like REST.
func (s tagService) List(ctx context.Context, p gitlab.ProjectRef, search, cursor string, limit int) (gitlab.Page[gitlab.Tag], error) {
	if err := s.c.enter(ctx, "Tags.List", p.FullPath, search, cursor, limit); err != nil {
		return gitlab.Page[gitlab.Tag]{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return paginate(s.c.matchTags(s.c.pathOf(p), search), cursor, limit), nil
}

// matchTags returns the tags of path matching search, without pipelines.
// The caller holds c.mu.
func (c *Client) matchTags(path, search string) []gitlab.Tag {
	var all []gitlab.Tag
	for _, t := range c.data.tags[path] {
		if matchSearch(t.Name, search) {
			t.Pipeline = nil
			all = append(all, t)
		}
	}
	return all
}

// Pipelines serves the pipelines whose ref is a tag, newest first.
func (s tagService) Pipelines(ctx context.Context, p gitlab.ProjectRef, limit int) ([]gitlab.PipelineSummary, error) {
	if err := s.c.enter(ctx, "Tags.Pipelines", p.FullPath, limit); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.c.tagPipelines(s.c.pathOf(p), limit), nil
}

// tagPipelines is the newest-first list of pipelines run for tags. The
// caller holds c.mu.
func (c *Client) tagPipelines(path string, limit int) []gitlab.PipelineSummary {
	names := map[string]bool{}
	for _, t := range c.data.tags[path] {
		names[t.Name] = true
	}
	var out []gitlab.PipelineSummary
	for _, pl := range c.data.pipes[path] {
		if names[pl.Ref] {
			out = append(out, pl.PipelineSummary)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Create adds the tag first in the list. A duplicate name answers 400 like
// GitLab; the commit is the head of a registered comparison to ref, or a
// made-up one.
func (s tagService) Create(ctx context.Context, p gitlab.ProjectRef, name, ref, message string) (gitlab.Tag, error) {
	if err := s.c.enter(ctx, "Tags.Create", p.FullPath, name, ref, message); err != nil {
		return gitlab.Tag{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	for _, t := range s.c.data.tags[path] {
		if t.Name == name {
			return gitlab.Tag{}, &gitlab.Error{Kind: gitlab.ErrNotPossible, Status: 400, Message: "Tag " + name + " already exists"}
		}
	}
	commit := gitlab.CommitSummary{SHA: "0b7d3e9c1a5f4e2d8b6a0c3f7e9d1b4a6c8e0f2a", ShortSHA: "0b7d3e9", Title: "Merge branch 'fix/slot-race' into '" + ref + "'", CreatedAt: time.Now()}
	for key, cmp := range s.c.data.compares {
		if strings.HasSuffix(key, "\x00"+ref) && cmp.Head != nil {
			commit = *cmp.Head
		}
	}
	tag := gitlab.Tag{Project: p, Name: name, Message: message, Commit: commit}
	s.c.data.tags[path] = append([]gitlab.Tag{tag}, s.c.data.tags[path]...)
	return tag, nil
}

// Delete removes the tag; a protected one answers 403 like GitLab.
func (s tagService) Delete(ctx context.Context, p gitlab.ProjectRef, name string) error {
	if err := s.c.enter(ctx, "Tags.Delete", p.FullPath, name); err != nil {
		return err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	for i, t := range s.c.data.tags[path] {
		if t.Name != name {
			continue
		}
		if t.Protected {
			return &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403, Message: "You are not allowed to delete protected tags"}
		}
		s.c.data.tags[path] = slices.Delete(slices.Clone(s.c.data.tags[path]), i, i+1)
		return nil
	}
	return notFound("tag " + name)
}

// Compare serves the comparison registered with SetCompare; an unregistered
// one is empty for a known ref and 404 for any other.
func (s tagService) Compare(ctx context.Context, p gitlab.ProjectRef, from, to string) (gitlab.Comparison, error) {
	if err := s.c.enter(ctx, "Tags.Compare", p.FullPath, from, to); err != nil {
		return gitlab.Comparison{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	cmp, ok := s.c.data.compares[compareKey(path, from, to)]
	if !ok && !s.c.knownRef(path, to) {
		return gitlab.Comparison{}, notFound("ref " + to)
	}
	cmp.Commits = slices.Clone(cmp.Commits)
	return cmp, nil
}

// knownRef reports whether ref is a branch, a tag or the default branch of
// the project. The caller holds c.mu.
func (c *Client) knownRef(path, ref string) bool {
	if slices.Contains(c.data.branches[path], ref) || c.ProjectsByPath[path].DefaultBranch == ref {
		return true
	}
	return slices.ContainsFunc(c.data.tags[path], func(t gitlab.Tag) bool { return t.Name == ref })
}

func compareKey(path, from, to string) string { return path + "\x00" + from + "\x00" + to }

// SetCompare registers the comparison from..to of a project (commits
// newest first).
func (c *Client) SetCompare(path, from, to string, cmp gitlab.Comparison) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.compares[compareKey(path, from, to)] = cmp
}

func (s projects) Branches(ctx context.Context, p gitlab.ProjectRef, search string) ([]string, error) {
	if err := s.c.enter(ctx, "Projects.Branches", p.FullPath, search); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	var out []string
	for _, b := range s.c.data.branches[s.c.pathOf(p)] {
		if matchSearch(b, search) && len(out) < branchLimit {
			out = append(out, b)
		}
	}
	return out, nil
}

// branchLimit mirrors the page size of the REST branch search.
const branchLimit = 20

// matchSearch is GitLab's ref search: a case-insensitive substring, "^x"
// matches names starting with x and "x$" names ending with x.
func matchSearch(name, search string) bool {
	name, search = strings.ToLower(name), strings.ToLower(search)
	prefix, suffix := strings.HasPrefix(search, "^"), strings.HasSuffix(search, "$")
	search = strings.TrimSuffix(strings.TrimPrefix(search, "^"), "$")
	switch {
	case prefix && suffix:
		return name == search
	case prefix:
		return strings.HasPrefix(name, search)
	case suffix:
		return strings.HasSuffix(name, search)
	}
	return strings.Contains(name, search)
}

// isTag reports whether ref is a registered tag of the project (the
// caller holds the lock).
func (c *Client) isTag(path, ref string) bool {
	for _, t := range c.data.tags[path] {
		if t.Name == ref {
			return true
		}
	}
	return false
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

// SetMilestones registers the active milestone titles of a project; IDs are
// 1, 2, … in the given order.
func (c *Client) SetMilestones(path string, titles ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ms := make([]gitlab.Milestone, len(titles))
	for i, t := range titles {
		ms[i] = gitlab.Milestone{ID: int64(i + 1), Title: t}
	}
	c.data.milestones[path] = ms
}

func (s projects) Milestones(ctx context.Context, p gitlab.ProjectRef, search string) ([]gitlab.Milestone, error) {
	if err := s.c.enter(ctx, "Projects.Milestones", p.FullPath, search); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	var out []gitlab.Milestone
	for _, m := range s.c.data.milestones[s.c.pathOf(p)] {
		if matchSearch(m.Title, search) {
			out = append(out, m)
		}
	}
	return out, nil
}

// newNote makes a note by the fake user; the caller holds c.mu.
func (c *Client) newNote(body string) gitlab.Note {
	c.data.nextNoteID++
	return gitlab.Note{ID: c.data.nextNoteID, Author: c.User, Body: body, CreatedAt: c.now()}
}
