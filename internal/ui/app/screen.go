// Package app is the root of the UI: a stack of screens, an optional
// overlay (help, confirm, later pickers and forms), the footer and toasts.
package app

import (
	"fmt"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/browser"
	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/glab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/mrlinks"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/localgit"
)

// Settings are the config values screens need.
type Settings struct {
	PageSize      int           // project.page_size
	PollActive    time.Duration // polling.active
	PollIdle      time.Duration // polling.idle
	PollWorkspace time.Duration // polling.workspace

	PerProject       int  // workspace.per_project
	MaxGroupProjects int  // workspace.max_group_projects
	FetchOnStart     bool // git.fetch_on_start

	// RefsTagsFirst puts tags before branches in Run pipeline ref
	// suggestions (run_pipeline.refs_first: tags).
	RefsTagsFirst bool
	// PrefillLocalBranch puts the local branch into the Run pipeline ref
	// field and first among the suggestions (run_pipeline.prefill_local_branch).
	PrefillLocalBranch bool

	// LinkRules are the compiled links rules of the config (MR links).
	LinkRules []mrlinks.Rule
}

// Defaults of Settings (SPEC §7).
const (
	DefaultPageSize   = 20
	DefaultPollActive = 3 * time.Second
	DefaultPollIdle   = 30 * time.Second

	DefaultPollWorkspace    = 60 * time.Second
	DefaultPerProject       = 3
	DefaultMaxGroupProjects = 100
)

// Env is shared by all screens. The root mutates it on language and theme
// changes and then broadcasts LangChangedMsg / ThemeChangedMsg so screens
// can rebuild cached strings and key maps.
type Env struct {
	Styles *common.Styles
	// ThemeName is the config theme; the root rebuilds Styles from it when
	// the terminal background changes. Empty = terminal.
	ThemeName string
	Msg       *i18n.Messages
	Lang      i18n.Lang
	Client    gitlab.Client // nil until authenticated (setup screen)
	User      gitlab.User
	Host      string // instance URL
	Now       func() time.Time

	Cache    *cache.Cache // nil disables caching (every load goes to the network)
	Glab     *glab.Glab   // nil when glab is not installed or disabled
	Settings Settings

	// TokenSource tells where the token came from, to hint at the right
	// place to fix it after a 401.
	TokenSource config.Source
	// AuthLost is set when a request was rejected with 401 mid-session.
	AuthLost bool
	// Net tracks reachability of the host (offline mark, poll backoff,
	// recovery toast). May be nil: always online.
	Net *gitlab.Monitor
	// Focused reports whether the terminal window has focus.
	Focused bool
	// Browser opens a URL (browser.Open; replaced in tests).
	Browser func(url string) error
	// Exec runs an external program with the terminal handed over
	// (tea.ExecProcess; replaced in tests).
	Exec func(cmd *exec.Cmd, done func(error) tea.Msg) tea.Cmd
	// GlabLoggedIn caches the one-time `glab auth status` check (nil = not
	// checked yet in this session).
	GlabLoggedIn *bool

	// Disk holds the snapshots on disk (workspace overview, starred); nil
	// keeps nothing on disk.
	Disk *cache.Disk
	// Catalog lists the projects known without network for p and F.
	Catalog *Catalog
	// Search builds the project picker (p, global false) and the global
	// search (F, global true) overlays; nil disables both keys.
	Search func(global bool) Overlay
	// Todos builds the To-Do screen (T); nil disables the key and the
	// header counter (SPEC §3.17).
	Todos func() Screen
	// TodoCount is the number of pending to-dos for the header, valid when
	// TodoKnown; negative means over 10 000 (no X-Total).
	TodoCount int
	TodoKnown bool
	// Git is the state of local fetch/pull; unavailable without git.
	Git *localgit.Tracker

	// drafts holds the unsent comment texts of the session by DraftKey
	// (memory only, SPEC §3.16).
	drafts map[string]string
	// stash holds screen state kept for the session (stash.go).
	stash map[string]any
}

// DraftKey is the key of a comment draft: host, project, kind ("issue" or
// "mr") and IID.
func (e *Env) DraftKey(project, kind string, iid int64) string {
	return fmt.Sprintf("%s|%s|%s|%d", e.Host, project, kind, iid)
}

// Draft returns the comment draft stored under key, "" when none.
func (e *Env) Draft(key string) string { return e.drafts[key] }

// SetDraft stores the comment draft; an empty text forgets it.
func (e *Env) SetDraft(key, text string) {
	if text == "" {
		delete(e.drafts, key)
		return
	}
	if e.drafts == nil {
		e.drafts = map[string]string{}
	}
	e.drafts[key] = text
}

// NewEnv returns an Env with messages, default (dark) styles and clock set,
// ready to be passed to screen constructors.
func NewEnv(lang i18n.Lang) *Env {
	return &Env{
		Lang:   lang,
		Msg:    i18n.Get(lang),
		Styles: common.NewStyles(common.TerminalTheme(true)),
		Now:    time.Now,

		Settings: Settings{
			PageSize: DefaultPageSize, PollActive: DefaultPollActive, PollIdle: DefaultPollIdle, PrefillLocalBranch: true,
			PollWorkspace: DefaultPollWorkspace, PerProject: DefaultPerProject, MaxGroupProjects: DefaultMaxGroupProjects,
		},
		Focused: true,
		Browser: browser.Open,
		Exec:    func(c *exec.Cmd, done func(error) tea.Msg) tea.Cmd { return tea.ExecProcess(c, done) },
	}
}

// Screen is one entry of the navigation stack.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	// View renders the screen into the size given by SetSize (the root adds
	// the footer below).
	View() string
	Title() string
	KeyMap() common.KeyMap
	SetSize(width, height int)
}

// StatusProvider is implemented by screens that show a status in the right
// part of the footer when there is no toast ("poll 3s while running").
type StatusProvider interface {
	StatusHint() string
}

// InputCapturer is implemented by screens that currently have a focused
// text input: the root then forwards every key (except ctrl+c) to them
// instead of interpreting global shortcuts like q, ? or esc.
type InputCapturer interface {
	CapturesInput() bool
}

// Overlay is drawn centered above the top screen and receives keys first.
type Overlay interface {
	Update(msg tea.Msg) (Overlay, tea.Cmd)
	View(width, height int) string
}

// Navigation and app-level messages.
type (
	PushMsg        struct{ Screen Screen }
	PopMsg         struct{}
	ReplaceRootMsg struct{ Screen Screen }
	// ActivatedMsg is sent to a screen when it becomes the top of the stack.
	ActivatedMsg struct{}
	// DeactivatedMsg is sent when another screen covers it or it is popped.
	DeactivatedMsg struct{}

	// OpenProjectMsg opens a project chosen in search above the topmost
	// Workspace of the stack, or above the root without one; everything
	// above is closed.
	OpenProjectMsg struct{ Screen Screen }
	// OpenGroupMsg opens the Workspace of a group chosen in search above
	// the root (the start screen); everything above the root is closed, so
	// the stack holds one group Workspace at most.
	OpenGroupMsg struct{ Screen Screen }

	OpenOverlayMsg  struct{ Overlay Overlay }
	CloseOverlayMsg struct{}

	ToastMsg struct {
		Level common.ToastLevel
		Text  string
		Hint  string
	}

	LangChangedMsg  struct{ Lang i18n.Lang }
	ThemeChangedMsg struct{ Styles *common.Styles }

	// CacheRevalidatedMsg tells that the cache refreshed Key in the
	// background; the screen owning the key reloads it from the cache.
	CacheRevalidatedMsg struct{ Key string }

	// PipelineChangedMsg tells that an action on one screen (a job retried
	// or canceled in the log) changed pipeline ID of Ref; a screen showing
	// it, covered by the one that acted, reloads it when it is on top again.
	PipelineChangedMsg struct {
		Ref gitlab.ProjectRef
		ID  int64
	}

	// AuthLostMsg means a request was rejected with 401. The root stops
	// polling (screens check env.AuthLost) and shows the hint.
	AuthLostMsg struct{ Err error }
	// NetChangedMsg reports that the host became unreachable (Online false)
	// or reachable again (true).
	NetChangedMsg struct{ Online bool }
	// AuthRestoredMsg follows a successful token re-check; screens reload.
	AuthRestoredMsg struct{}

	// FocusChangedMsg reports terminal focus changes to screens.
	FocusChangedMsg struct{ Focused bool }

	// ExecFinishedMsg is the result of an external program run with
	// tea.ExecProcess (glab actions).
	ExecFinishedMsg struct {
		Action string
		Err    error
	}

	toastExpiredMsg struct{ id int }
	langSavedMsg    struct{ err error }
	authCheckedMsg  struct {
		user gitlab.User
		err  error
	}
)

func msgCmd(m tea.Msg) tea.Cmd { return func() tea.Msg { return m } }

// Push opens s on top of the stack.
func Push(s Screen) tea.Cmd { return msgCmd(PushMsg{Screen: s}) }

// Pop closes the top screen.
func Pop() tea.Cmd { return msgCmd(PopMsg{}) }

// ReplaceRoot clears the stack and makes s the only screen.
func ReplaceRoot(s Screen) tea.Cmd { return msgCmd(ReplaceRootMsg{Screen: s}) }

// OpenProject opens a project screen chosen in search (OpenProjectMsg).
func OpenProject(s Screen) tea.Cmd { return msgCmd(OpenProjectMsg{Screen: s}) }

// OpenGroup opens the Workspace of a group chosen in search (OpenGroupMsg).
func OpenGroup(s Screen) tea.Cmd { return msgCmd(OpenGroupMsg{Screen: s}) }

// OpenOverlay shows o above the current screen.
func OpenOverlay(o Overlay) tea.Cmd { return msgCmd(OpenOverlayMsg{Overlay: o}) }

// CloseOverlay hides the current overlay.
func CloseOverlay() tea.Cmd { return msgCmd(CloseOverlayMsg{}) }

// PipelineChanged tells every screen that pipeline id of ref changed.
func PipelineChanged(ref gitlab.ProjectRef, id int64) tea.Cmd {
	return msgCmd(PipelineChangedMsg{Ref: ref, ID: id})
}

// Toast shows a toast.
func Toast(level common.ToastLevel, text, hint string) tea.Cmd {
	return msgCmd(ToastMsg{Level: level, Text: text, Hint: hint})
}
