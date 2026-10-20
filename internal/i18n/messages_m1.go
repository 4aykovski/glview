package i18n

// M1 message groups. They are filled by fillM1EN / fillM1RU; the
// completeness test covers every field.

// ProjectMessages are the strings of the project screen and its list tabs.
type ProjectMessages struct {
	Later func(tab string) string // placeholder tab body

	// Table
	LoadingMore   string // "◐ loading more…"
	LoadFailed    string // "✗ could not load, r — retry"
	Loading       string
	Counter       func(loaded, total int, atLeast bool) string // "20 of 1 482"
	FilterHint    string                                       // placeholder of the filter input
	FilterUnknown func(key string, known string) string
	FilterBad     func(key, value, allowed string) string
	PreviewHidden string // hint when the preview is hidden (narrow terminal)

	// Descriptions of the filter keys shown in suggestions
	KeyState, KeyScope, KeyReviewer, KeyDraft, KeyLabels string
	KeyRef, KeyStatus, KeySource, KeyUser                string
	NoteBranch, NoteTag                                  string
	StatusPoll                                           func(interval string) string
	StatusLive                                           func(running int, interval string) string
}

// MRMessages are the strings of the MR list, MR detail and MR actions.
type MRMessages struct {
	// Columns
	ColIID, ColTitle, ColAuthor, ColBranch, ColPipeline, ColAppr, ColThr, ColUpd string

	Draft     string // "Draft:" prefix is GitLab's own; this is the filter label
	Conflicts string // "⚠ conflicts"

	// Empty states
	EmptyBase                                                                       func(state string) string // "No open MRs"
	EmptyWhere                                                                      string                    // ", where "
	QualReviewerMe, QualAuthorMe, QualAssigneeMe, QualMine, QualDraft, QualNotDraft string
	QualReviewer                                                                    func(user string) string
	QualLabels                                                                      func(labels string) string
	QualText                                                                        func(text string) string
	EmptyHint                                                                       string

	// Preview
	PrevApprovals   func(names string) string
	PrevNoApprovals string
	PrevPipeline    string
	PrevDiff        func(add, del, files int) string
	PrevNoDesc      string

	// Detail screen
	Crumb                                                   string // "MRs" in the breadcrumbs
	TabOverview, TabDiscussions, TabPipelines               string
	Opened                                                  func(age string) string
	Merged                                                  func(by, age string) string // by may be empty
	Closed                                                  func(age string) string
	ChipApprovals                                           func(have, required int) string
	ChipThreads                                             func(n int) string
	SecApprovals, SecLabels, SecPipeline, SecLastThread     string
	None, ThreadResolved, ThreadUnresolved                  string
	EmptyDiscussions, EmptyPipelines, Loading               string
	ColStatus, ColID, ColRef, ColSource, ColCommit, ColUser string
	ColDur, ColAgo                                          string
	TabCommits, EmptyCommits, ColSHA                        string
	NeedApprovals                                           func(left int) string

	// Actions
	ToastApproved, ToastUnapproved, ToastMerged, ToastAutoMerge func(iid int64) string
	MergePushed                                                 string
	CannotMerge                                                 func(status string) string
	RebaseTitle                                                 func(iid int64) string
	RebaseBody, RebaseDone, RebaseStillRunning                  string
	RebaseFailed                                                func(reason string) string
	RebaseChip                                                  string
	MergeTitle                                                  func(iid int64) string
	MergeNow, MergeWhenPipeline, MergeSquash, MergeRemoveBranch string
	MergeReason                                                 func(status, label string) string
	MergeHint, MergeHintBlocked                                 string
	CheckoutFailed                                              func(code int) string
	GlabNotLoggedIn                                             string

	// detailed_merge_status captions (the status itself is never translated)
	StatusLabel func(status string) string
}

// PipelineMessages are the strings of pipelines, pipeline and job screens.
type PipelineMessages struct {
	// List tab
	EmptyBase                                 string
	EmptyWhere                                string
	QualRef, QualStatus, QualSource, QualUser func(v string) string
	QualMine                                  string
	EmptyHint                                 string
	RetryTitle, CancelTitle                   func(id int64) string
	RetryBody, CancelBody                     string
	ToastRetried, ToastCanceled               func(id int64) string
	NoLogJobs                                 string
	PrevOn                                    func(ref string) string
	PrevStagesLoading, PrevNoStages           string

	// Pipeline screen
	Crumb                                  string
	Summary                                func(jobs string, parts string) string // "9 jobs: 4 ✓ 2 ◐"
	PanelStage, PanelRunner, PanelAttempts string
	PanelAllowFailure                      string
	PanelNoLog                             string
	BridgeWaiting, BridgeNoAccess          string
	Downstream                             string
	YAMLErrors                             string
	ToastNoAccess                          string
	NoJobs                                 string
	JobRetried, JobCanceled, JobPlayed     func(name string) string
	CancelJobTitle                         func(name string) string
	CancelJobBody                          string
	PlayTitle                              func(name string) string
	NotManual                              string

	// Attempts overlay
	AttemptsTitle               func(name string) string
	AttemptLatest, AttemptsKeys string
}

// LogMessages are the strings of the job log.
type LogMessages struct {
	Follow, NoFollow string
	Updated          func(age string) string
	Truncated        string
	Empty            func(status string) string
	SearchPrompt     string
	MatchOf          func(i, n int) string
	NoMatches        string
	Crumb            string
	SearchKeys       string
	Attempt          func(i, n int) string // "attempt 2/3" of a retried job
}

// FormMessages are the strings of overlays and forms.
type FormMessages struct {
	ColKey, ColValue, ColType        string
	KeyPlaceholder, ValuePlaceholder string // empty cells of the variables table
	VarsEmpty                        string
	VarsHint                         string
	RunHint                          string
	TypeEnv, TypeFile                string

	// Run pipeline
	RunTitle, RefLabel, VarsLabel       string
	RefCurrent, RefTag                  string
	RefSearching, NoRefs, RefsFailed    string
	RefBranches, RefTags                string // group names in the summary line
	HintsLabel, HintsLoading, HintsNone string
	HintsUnavailable                    string
	HintsAll                            func(n int) string // "s all (12)" at the end of a cut hints line
	WillCreate                          func(ref string, vars int) string
	RefRequired                         string
	PickValue                           func(key string) string
	Created                             func(id int64) string
	RunNoVars                           string
	RunButton                           string
	// Key lines at the bottom of the form, by focus.
	RefKeys, VarsKeys, ButtonKeys, EditKeys, PickKeys string
	// Key lines while the form is locked by a run: the tag check, the creation.
	CheckingKeys, CreatingKeys string
	// Running a pipeline on a tag that already has one.
	TagRerunTitle  func(tag string) string
	TagRerunBody   func(id int64, status string) string
	TagCheckFailed func(err string) string
}
