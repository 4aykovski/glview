// Package gitlab holds gitlab-tui's own domain types and service
// interfaces. The UI depends only on this package; transports (REST via
// client-go, GraphQL) live in sub-packages and map into these types.
package gitlab

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ProjectRef identifies a project. ID may be 0 until it is known.
type ProjectRef struct {
	ID       int64
	FullPath string // "platform/route-service"
}

// Key returns a stable identifier for maps and cache keys.
func (r ProjectRef) Key() string { return r.FullPath }

type Project struct {
	ProjectRef
	Name           string
	WebURL         string
	DefaultBranch  string
	LastActivityAt time.Time
	Archived       bool
	Starred        bool
}

type User struct {
	ID       int64
	Username string
	Name     string
	WebURL   string
}

// Version is the GitLab instance version (GET /version).
type Version struct {
	Version  string // "17.11.2-ee"
	Revision string
}

type Group struct {
	ID       int64
	FullPath string
	Name     string
	WebURL   string
}

type MRState string

const (
	MRStateOpened MRState = "opened"
	MRStateMerged MRState = "merged"
	MRStateClosed MRState = "closed"
	MRStateLocked MRState = "locked"
	MRStateAll    MRState = "all"
)

type MergeRequest struct {
	Project      ProjectRef
	IID          int64
	Title        string
	Description  string // markdown; filled in detail views only
	State        MRState
	Draft        bool
	Author       User
	Assignees    []User
	Reviewers    []User
	SourceBranch string
	TargetBranch string
	SHA          string
	Labels       []string
	MergeStatus  string // detailed_merge_status as returned by GitLab
	HasConflicts bool
	HeadPipeline *PipelineSummary
	Approvals    Approvals
	Discussions  DiscussionStats
	Diff         *DiffStats
	WebURL       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// MergedAt, ClosedAt and MergedBy are filled in detail views only (zero/nil
	// otherwise, and when the MR was not merged/closed).
	MergedAt         time.Time
	ClosedAt         time.Time
	MergedBy         *User
	RebaseInProgress bool
	MergeError       string // reason of the last failed rebase/merge
	// Squash and ForceRemoveSourceBranch are the MR's settings, used as
	// defaults of the merge dialog.
	Squash                  bool
	ForceRemoveSourceBranch bool
}

// MRScope narrows MR lists to the user's own MRs.
type MRScope string

const (
	MRScopeAll          MRScope = "all"
	MRScopeCreatedByMe  MRScope = "created_by_me"
	MRScopeAssignedToMe MRScope = "assigned_to_me"
)

// MRFilter selects merge requests of a project.
type MRFilter struct {
	State    MRState
	Scope    MRScope // "" = all
	Reviewer string  // "" = any, "@me" or a username
	Draft    *bool   // nil = any
	Labels   []string
	Search   string // matched against the title
	Mine     bool   // "me" filter: author ∪ assignee ∪ reviewer
}

// Key is a canonical signature of the filter for cache keys and equality.
func (f MRFilter) Key() string {
	draft := "any"
	if f.Draft != nil {
		draft = strconv.FormatBool(*f.Draft)
	}
	scope := f.Scope
	if scope == "" {
		scope = MRScopeAll
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%t", f.State, scope, f.Reviewer, draft, strings.Join(f.Labels, ","), f.Search, f.Mine)
}

// MergeOptions are the parameters of a merge request merge.
type MergeOptions struct {
	AutoMerge          bool // merge when the pipeline succeeds
	Squash             bool
	RemoveSourceBranch bool
	SHA                string // head SHA the user has seen
}

// PipelineFilter selects pipelines of a project.
type PipelineFilter struct {
	Ref, Status, Source, Username string
	Scope                         string // "", "tags", "branches"
}

// Key is a canonical signature of the filter for cache keys and equality.
func (f PipelineFilter) Key() string {
	return strings.Join([]string{f.Ref, f.Status, f.Source, f.Username, f.Scope}, "|")
}

type Approvals struct {
	ApprovedBy   []User
	Required     int // 0 on CE or without approval rules
	Left         int
	ApprovedByMe bool
}

type DiscussionStats struct{ Total, Unresolved int }

type DiffStats struct{ Additions, Deletions, Files int }

type Discussion struct {
	ID         string
	Resolvable bool
	Resolved   bool
	Notes      []Note
	Position   *NotePosition
}

type Note struct {
	ID        int64
	Author    User
	Body      string
	System    bool
	CreatedAt time.Time
}

// NotePosition locates a diff note; the diff itself is not shown in the MVP.
type NotePosition struct {
	Path string
	Line int
}

type PipelineSummary struct {
	Project     ProjectRef
	ID          int64
	IID         int64
	Status      PipelineStatus
	Ref         string
	SHA         string
	Source      string
	User        *User
	CommitTitle string
	Duration    time.Duration
	CreatedAt   time.Time
	FinishedAt  *time.Time
	WebURL      string
	Stages      []StageSummary // only when requested (preview)
}

type StageSummary struct {
	Name        string
	Status      PipelineStatus
	Done, Total int
	Duration    time.Duration // sum of the jobs' durations, 0 when unknown
}

type Pipeline struct {
	PipelineSummary
	Jobs       []Job
	Bridges    []Bridge
	YAMLErrors string
}

type Job struct {
	ID           int64
	Name         string
	Stage        string
	Status       PipelineStatus
	AllowFailure bool
	Duration     time.Duration
	Runner       string
	StartedAt    *time.Time
	WebURL       string
	Attempts     []Job // earlier (retried) attempts, newest first
}

// Bridge is a trigger job; Downstream is nil until the pipeline is created.
type Bridge struct {
	Job
	DownstreamProject string
	Downstream        *PipelineSummary
}

type VarType string

const (
	VarEnv  VarType = "env_var"
	VarFile VarType = "file"
)

type CIVariable struct {
	Key   string
	Value string
	Type  VarType
}

// CIConfigVariable is a variable hint from .gitlab-ci.yml (ciConfigVariables).
type CIConfigVariable struct {
	Key          string
	Value        string
	ValueOptions []string
	Description  string
}

type Tag struct {
	Project   ProjectRef
	Name      string
	Message   string
	Commit    CommitSummary
	Protected bool
	Release   *ReleaseSummary
	Pipeline  *PipelineSummary
}

type ReleaseSummary struct {
	Name        string
	Description string
}

type CommitSummary struct {
	SHA        string
	ShortSHA   string
	Title      string
	AuthorName string
	CreatedAt  time.Time
}

type Issue struct {
	Project        ProjectRef
	IID            int64
	Title          string
	Description    string
	State          string // opened | closed
	Author         User
	Assignees      []User
	Labels         []string
	Milestone      string
	UserNotesCount int
	RelatedMRs     []int64
	WebURL         string
	UpdatedAt      time.Time
}

// ProjectGroup is one project's slice of a multi-project batch.
type ProjectGroup[T any] struct {
	Project ProjectRef
	Items   []T
	Total   int   // for "N more"; -1 when unknown
	Err     error // per-project error (not found, no access)
}

// Page is one page of a list.
type Page[T any] struct {
	Items      []T
	NextCursor string // empty = last page; REST page number as a string
	Total      int    // -1 when unknown
}

// BatchResult is one completed batch of a multi-project request.
type BatchResult[T any] struct {
	Groups []ProjectGroup[T]
	Err    error // whole-batch error; also copied into every group's Err
}
