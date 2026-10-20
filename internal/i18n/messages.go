package i18n

// Messages is the full set of UI strings for one language. Every string
// field must be non-empty and every func non-nil in each language; a
// reflection test enforces it.
type Messages struct {
	Lang Lang
	Name string // language name shown in Help ("English", "Русский")

	Tabs struct {
		Projects, MRs, Pipelines, Issues, Tags string
	}

	Keys struct {
		Up, Down, Top, Bottom, Open, Back, Quit, Refresh, Browser, Copy,
		Help, Language, NextTab, PrevTab, Tabs, Project, Search, Me string

		// M1
		Filter, Approve, Merge, Rebase, Checkout, Retry, Cancel, NewPipeline, Log,
		Play, Follow, Fold, FoldAll, Stage, Job, Edit, Add, Delete, Submit, Next,
		Details, Comment, Attempts, PrevNextAttempt string
		State, Status, Reset                string
		BrowserList, BrowserOne, BrowserJob string
		Find                                string
		Threads                             string
	}

	Help struct {
		Title, Screen, Global, Close, LanguageLine, TokenSource string
		GlabFound                                               func(host string) string
		GlabNotFound                                            string
	}

	Confirm struct {
		Yes, No string
	}

	Header struct {
		Workspace string
		Updated   func(age string) string
		Offline   string
		AuthError string
	}

	Toast struct {
		LanguageSwitched  string
		ConfigSaveFailed  func(err string) string
		ConfigBroken      func(detail string) string
		ConfigPermissions func(path string) string
		ConfigBadValue    func(key, value, def string) string
		BrowserFailed     string
		Copied            func(what string) string
		AuthRestored      string
	}

	Errors struct {
		Unauthorized, Forbidden, InsufficientScope, NotFound, Conflict,
		NotPossible, Network, TLS, Server, Unknown string
		RateLimited func(seconds int) string

		HintConfigToken string
		HintEnvToken    string
		HintGlabToken   func(host string) string
		HintTLS         string
	}

	Setup struct {
		Title, Intro, URL, URLHint, Token, TokenHint, Submit, Checking,
		BadToken, Unreachable, TLSFailed, Saved string
		UseGlab    func(host string) string
		LoggedInAs func(user string) string
	}

	Startup struct {
		NotOnInstance        func(instance string) string
		Remote, RemoteHost   string
		InstanceHost         string
		ProjectNotFound      string
		NoUsableRemote       string
		EmptyWorkspace       func(dir string) string
		EmptyWorkspaceDetail func(scanned, foreign, noGit int) string
		TooSmall             func(minW, minH int) string
		PickProject          string
		GlobalSearch         string
		OpenWorkspace        string
		ComingSoon           string
		SkippedNoGit         func(names string) string
		ForeignRemote        string
		NoRemote             string
	}

	Columns struct {
		Folder, GitLab, LocalBranch string
	}

	Fields struct {
		Project, Web, DefaultBranch, LocalDir, Branch, Activity, Detached string
	}

	Project ProjectMessages
	MR      MRMessages
	Pipe    PipelineMessages
	Log     LogMessages
	Form    FormMessages

	Plural struct {
		GitLabProjects func(n int) string // "5 GitLab projects" / "5 GitLab-проектов"
		Projects       func(n int) string
		More           func(n int) string // "3 more" / "ещё 3"
		Tags           func(n int) string
		Lines          func(n int) string
		MRs            func(n int) string
		Pipelines      func(n int) string
		Jobs           func(n int) string
		Threads        func(n int) string
		Files          func(n int) string
		Replies        func(n int) string
	}
}
