package i18n

// M3 message groups: MR links (T34), tags (T23), issues and comments (T24). Filled by fillM3EN / fillM3RU; the
// completeness test covers every field.

// LinksMessages are the strings of the MR link overlay (L) and the links
// config rules.
type LinksMessages struct {
	Key        string                  // footer label of L
	Title      func(ref string) string // overlay title: "Links · !479"
	Hints      string                  // overlay footer hints
	NoLinks    string                  // toast: the MR has no links
	What       string                  // "copied: link"
	BadRule    func(detail string) string
	LoadFailed string // toast: the MR details for the links failed
}

// ApproversMessages are the strings of the approvers overlay (A, SPEC 3.5).
type ApproversMessages struct {
	Key         string                  // footer label of A
	Title       func(ref string) string // overlay title: "Approvers · !479"
	Hints       string                  // overlay footer hints
	NoApprovers string                  // toast: the MR has no approvers
	More        string                  // last line of the APPROVALS column: "A — all"
}

// TagMessages are the strings of the Tags tabs, the New tag form and the
// tag actions (SPEC §3.4, §3.10).
type TagMessages struct {
	// Row actions
	DeleteTitle    func(name string) string
	DeleteBody     func(sha string) string
	DeleteProtect  string // appended to the body of a protected tag
	Deleted        func(name string) string
	Protected      func(name string) string // toast: 403 on a protected tag
	Created        func(name string) string
	NoPipeline     string
	WhatName       string // "copied: tag name"
	KeyNew         string
	KeyDelete      string
	KeyPipeline    string
	KeyCopyName    string
	KeySearch      string
	CommitsPlural  func(n int) string // "2 commits" / "2 коммита"
	PrevProtected  string
	PrevAhead      func(branch string, n int) string
	PrevSame       func(branch string) string
	PrevComparing  string
	PrevCompareErr string
	PrevSuggest    func(name string) string
	PrevRelease    string // "Release notes:"

	// New tag form
	FormTitle                     string
	LblName, LblRef, LblMessage   string
	LblCommits                    string
	Custom                        string
	SuggestedFrom                 func(kind, base string) string // "suggested: patch from v1.14.2"
	SuggestedNoBase               func(name string) string
	LoadingTags                   string
	RefCaption                    func(ref, sha string, n int, last string) string
	RefFirstTag                   func(ref string) string
	RefComparing, RefCompareErr   string
	NoNewCommits                  func(last string) string
	CommitsMore                   func(n int) string
	ErrNoName, ErrNoRef           string
	ErrExists                     func(name string) string
	ConfirmTitle                  func(name, ref, sha string) string
	ConfirmAnnotated              func(msg string) string
	ConfirmLightweight            string
	ButtonCreate                  string
	KeysName, KeysCustom, KeysRef string
	KeysRefList, KeysMessage      string
	KeysButton, KeysCreating      string

	// Tags tab
	ColTag, ColMessage     string
	ReleaseMark            string // "◆ release"
	SearchLabel, SortLabel string
	SearchHint             string
	EmptyNone              string
	EmptyQuery             func(q string) string

	// Workspace Tags tab
	GroupEmpty  string // "no tags"
	NoMine      string // "me: not applied to tags"
	PerProject  func(n int) string
	SortVersion string
}

// CommentMessages are the strings of the comment form (SPEC §3.16), shared
// by issues and merge requests.
type CommentMessages struct {
	Title   func(ref, title string) string // "Comment · #1318 Surge…"
	Hint    string                         // under the field
	Close   string                         // "esc close"
	Send    string                         // button label
	Empty   string                         // "comment is empty"
	Sending string
	Added   string // toast
}

// IssueMessages are the strings of the Issues tab, the issue screen and the
// issue actions (SPEC §3.4, §3.15).
type IssueMessages struct {
	// Tab
	Crumb         string // "Issues" in the breadcrumbs of the issue screen
	ColLabels     string
	EmptyOpen     string // no issues, default filters
	EmptyMine     string // no issues with the "me" filter
	EmptyFiltered string // no issues with other filters
	KeyAssignee   string // descriptions of the filter keys
	KeyLabel      string
	KeyMilestone  string
	KeyToggle     string // x: "close/reopen"
	KeyBrowser    string // O: "issue in browser"

	// Preview and issue screen
	StatusOpen, StatusClosed string
	Assignee                 func(names string) string // "assignee: egor"
	Milestone                func(title string) string // "milestone: 2026-Q4"
	NoDescription            string
	PrevComments             func(n int) string // "COMMENTS 2"
	NotesLoading             string
	NotesFailed              string
	WriteHint                string // "c — write a comment…"
	Labels                   string // "Labels:"
	Comments                 func(n int) string
	NoComments               string

	// Close and reopen (x)
	ConfirmClose func(iid int64) string // confirm title
	Closed       func(iid int64) string // toast
	Reopened     func(iid int64) string
}
