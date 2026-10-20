package gitlab

import "context"

// Client is the entry point the UI uses. Services are added as screens need
// them (M2: workspace batches, search; M3: issues, tag mutations) — see
// SPEC §6.3 for the target shape.
type Client interface {
	Users() UserService
	Projects() ProjectService
	MergeRequests() MergeRequestService
	Pipelines() PipelineService
	Jobs() JobService
	Tags() TagService
	Instance() InstanceService
}

type UserService interface {
	Current(ctx context.Context) (User, error)
}

type ProjectService interface {
	Get(ctx context.Context, ref ProjectRef) (Project, error)
	// Branches returns branch names matching search (server side).
	Branches(ctx context.Context, p ProjectRef, search string) ([]string, error)
	// Members returns project members (including inherited) whose name or
	// username matches search, for filter suggestions.
	Members(ctx context.Context, p ProjectRef, search string) ([]User, error)
	// Labels returns project label names matching search.
	Labels(ctx context.Context, p ProjectRef, search string) ([]string, error)
	// CIConfigVariables returns the variable hints declared in
	// .gitlab-ci.yml for gitRef. GitLab computes them asynchronously: a
	// result that is not ready yet is reported as ErrNotReady.
	CIConfigVariables(ctx context.Context, p ProjectRef, gitRef string) ([]CIConfigVariable, error)
}

type MergeRequestService interface {
	List(ctx context.Context, p ProjectRef, f MRFilter, cursor string, limit int) (Page[MergeRequest], error)
	Get(ctx context.Context, p ProjectRef, iid int64) (MergeRequest, error)
	// Discussions returns all discussions of the MR, system notes included
	// (the UI filters them).
	Discussions(ctx context.Context, p ProjectRef, iid int64) ([]Discussion, error)
	Pipelines(ctx context.Context, p ProjectRef, iid int64) ([]PipelineSummary, error)
	// Approve passes sha, the head SHA the user has seen.
	Approve(ctx context.Context, p ProjectRef, iid int64, sha string) error
	Unapprove(ctx context.Context, p ProjectRef, iid int64) error
	Merge(ctx context.Context, p ProjectRef, iid int64, o MergeOptions) (MergeRequest, error)
	Rebase(ctx context.Context, p ProjectRef, iid int64) error
	// RecheckMergeStatus asks GitLab to recompute the merge status (REST,
	// with_merge_status_recheck=true) and returns detailed_merge_status,
	// usually "checking". GitLab ignores the flag for roles below Developer.
	RecheckMergeStatus(ctx context.Context, p ProjectRef, iid int64) (string, error)
}

type PipelineService interface {
	List(ctx context.Context, p ProjectRef, f PipelineFilter, cursor string, limit int) (Page[PipelineSummary], error)
	// Get returns the pipeline with its jobs (retries collapsed) and bridges.
	Get(ctx context.Context, p ProjectRef, id int64) (Pipeline, error)
	// Stages returns per-stage progress for the list preview; iid is the
	// pipeline's project-level IID.
	Stages(ctx context.Context, p ProjectRef, iid int64) ([]StageSummary, error)
	Create(ctx context.Context, p ProjectRef, ref string, vars []CIVariable) (PipelineSummary, error)
	Retry(ctx context.Context, p ProjectRef, id int64) error
	Cancel(ctx context.Context, p ProjectRef, id int64) error
}

type JobService interface {
	Get(ctx context.Context, p ProjectRef, id int64) (Job, error)
	Trace(ctx context.Context, p ProjectRef, id int64) ([]byte, error)
	Retry(ctx context.Context, p ProjectRef, id int64) (Job, error)
	Cancel(ctx context.Context, p ProjectRef, id int64) error
	Play(ctx context.Context, p ProjectRef, id int64, vars []CIVariable) (Job, error)
}

// TagService is limited to List in M1 (ref autocompletion); tag management
// arrives in M3.
type TagService interface {
	List(ctx context.Context, p ProjectRef, search, cursor string, limit int) (Page[Tag], error)
}

type InstanceService interface {
	// URL is the instance base URL (with relative URL root, no trailing slash).
	URL() string
	Version(ctx context.Context) (Version, error)
}
