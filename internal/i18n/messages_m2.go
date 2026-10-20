package i18n

// M2 message groups: the workspace, project search and local git. They are
// filled by fillM2EN / fillM2RU; the completeness test covers every field.

// WorkspaceMessages are the strings of the Workspace screen.
type WorkspaceMessages struct {
	ShownOf func(shown, total int) string // "showing 100 of 342"

	// Projects tab
	ColFolder, ColGitLab, ColBranch, ColAheadBehind, ColMRs, ColDefault, ColTag, ColActivity string
	NoAccess                                                                                 string // "no access"
	Archived                                                                                 string
	EmptyHint                                                                                string // "p pick a project · F search GitLab · q quit"
	FilterHint                                                                               string // placeholder of the text filter
	Search                                                                                   string // filter bar label when no text filter is set

	PrevRemote  string
	PrevOpenMRs func(open, review int) string // "6 open MRs (2 waiting for your review)"
	PrevLocal   func(branch string) string    // "on fix/slot-race locally"
	PrevNoMR    string                        // no open MR from the local branch
	PrevLoading string
	PrevNoPipe  string // the default branch has no pipelines

	// Group tabs
	GroupCount   func(n int, state string, atLeast bool) string // "6 open", "≥4 open"
	GroupWaiting func(n int) string                             // "2 waiting for you"
	GroupShown   func(n int) string                             // "showing 3"
	More         func(n int) string                             // "↳ 11 more · f filter by project"
	EmptyMRs     func(state string, mine bool) string           // "no open", "no MRs of yours"
	EmptyPipes   func(mine bool) string
	Loading      string // "loading…"
	ColYou       string
	MeMRs        string // filter bar: "me: author/assignee/reviewer"
	MePipes      string // filter bar: "me: mine"
	StatusGroups string // footer status: "groups by latest activity"

	// Keys
	KeyGroups, KeyCollapse, KeyProject, KeyOpenProject string
}

// SearchMessages are the strings of the project picker (p) and the global
// search (F).
type SearchMessages struct {
	PickerTitle, GlobalTitle                                      string
	SecRemote, SecWorkspace, SecStarred, SecRecent                string
	SecProjects, SecGroups                                        string
	Current, Local                                                string
	MRs                                                           func(n int) string // "3 MR"
	NothingFound, NothingGlobal, TypeMore, Searching, PickerHints string
	GlobalHints                                                   string
	Failed                                                        func(err string) string
	Counter                                                       func(shown, total int) string // "6 / 214"
	KeyPicker, KeyGlobal                                          string
}

// GitMessages are the strings of fetch and pull (SPEC §10.4).
type GitMessages struct {
	Progress func(op string, done, total int) string // "⟳ fetch 7/13"
	Busy     func(op string) string                  // "fetch is already running"
	// Summary is the final toast: "fetch: 12 ✓, 1 ✗ (route-planner-ui:
	// needs authentication)"; first describes the first failure ("" if none).
	Summary func(op string, ok, skipped, failed int, first string) string

	ReasonAuth, ReasonNetwork, ReasonDiverged, ReasonDirty, ReasonTimeout string
	ReasonDetached, ReasonNoUpstream, ReasonInProgress, ReasonNoGit       string
	AuthHint                                                              func(dir string) string
	UpToDate                                                              string
	Moved                                                                 func(n int) string // "+3"

	PullTitle string
	PullBody  func(update, skip int, reasons string) string // "update 11 repositories, skip 2 (detached, no upstream)"

	KeyFetch, KeyPull string
}
