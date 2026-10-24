package i18n

// T37 message group: the New MR form (SPEC §3.18). Filled by fillT37EN /
// fillT37RU; the completeness test covers every field.

// NewMRMessages are the strings of the New MR form.
type NewMRMessages struct {
	Crumb                                       string // "new MR" in the breadcrumbs
	KeyNew                                      string // footer label of n
	KeyField, KeyClose                          string // footer labels of tab and esc
	LblFrom, LblInto, LblTitle, LblTemplate     string
	LblDesc, LblAssignees, LblReviewers         string
	LblLabels, LblMilestone                     string
	CheckDraft, CheckRemove, CheckSquash        string
	ByProject                                   string // "by project setting"
	NoTemplate                                  string
	TemplatesLoading, TemplatesFailed           string
	PhBranch, PhSearch, PhDesc                  string
	Comparing, CompareErr, Checking, Creating   string
	DupCheckErr                                 string                          // the open-MR lookup failed
	Stat                                        func(commits, files int) string // "3 commits · 5 files"
	NoChanges                                   func(src, dst string) string
	NotPushed                                   func(src string) string
	Unpushed                                    func(n int) string
	DupOpen                                     func(src string, iid int64) string
	ErrNoBranch, ErrSame, ErrNoTitle            string
	ConfirmReplaceTitle                         func(name string) string
	ConfirmReplaceBody                          string
	ConfirmDup                                  func(src string, iid int64, title string) string
	ConfirmDupTitle                             string
	PrevWill                                    func(src, dst string) string
	PrevDraft                                   string
	PrevAssignee                                func(name string) string
	PrevAssignees, PrevReviewers, PrevLabels    func(n int) string
	PrevMilestone                               func(title string) string
	ButtonCreate                                string
	KeysText, KeysHints, KeysDesc, KeysCheck    string
	KeysTemplate, KeysButton, KeysWait, KeysRun string
	Created                                     func(iid int64) string
}
