// Package localgit is the UI state of local git operations (SPEC §10.4):
// the running fetch or pull, the last result per folder and the
// ahead/behind counts. One Tracker lives in app.Env; the root feeds it the
// messages of its commands and every screen reads it, so a screen under
// the stack is up to date when it comes back.
package localgit

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitexec"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
)

// AB is the ahead/behind of a folder's branch against its upstream.
type AB struct {
	Ahead, Behind int
	Known         bool // false: no upstream, or not counted yet
}

type (
	// ProgressMsg reports one finished repository of the running operation.
	ProgressMsg struct {
		tok    common.Token
		Result gitexec.Result
		next   tea.Cmd
	}
	// DoneMsg ends the running operation.
	DoneMsg struct {
		tok common.Token
		Op  gitexec.Op
	}
	// AheadBehindMsg carries a local ahead/behind count. gen is the
	// folder's generation when the count started: a fetch or pull of the
	// folder since then makes it stale.
	AheadBehindMsg struct {
		Dir string
		AB  AB
		gen int
	}
)

// Tracker holds the git state of the session. A nil runner (git not found
// or disabled) makes it unavailable; methods are safe on a nil *Tracker.
type Tracker struct {
	runner gitexec.Runner
	gen    common.Gen
	cancel context.CancelFunc

	busy        bool
	op          gitexec.Op
	done, total int
	pending     map[string]bool // folders of the running operation not finished yet
	repos       map[string]*gitremote.Repo
	results     map[string]gitexec.Result // last result per folder
	ab          map[string]AB
	abGen       map[string]int   // per folder: bumped by every fetch/pull of it
	run         []gitexec.Result // results of the running (or last) operation
}

// New creates a tracker; runner may be nil.
func New(runner gitexec.Runner) *Tracker {
	return &Tracker{runner: runner, gen: common.NewGen(), pending: map[string]bool{},
		repos: map[string]*gitremote.Repo{}, results: map[string]gitexec.Result{}, ab: map[string]AB{}, abGen: map[string]int{}}
}

// Available reports whether git can be used (u/U are hidden otherwise).
func (t *Tracker) Available() bool { return t != nil && t.runner != nil }

// Busy reports whether an operation is running.
func (t *Tracker) Busy() bool { return t != nil && t.busy }

// Op returns the running (or last) operation.
func (t *Tracker) Op() gitexec.Op { return t.op }

// Progress returns how many repositories finished and how many there are.
func (t *Tracker) Progress() (done, total int) { return t.done, t.total }

// Running reports whether dir is part of the running operation and not
// finished yet (the ◐ mark).
func (t *Tracker) Running(dir string) bool { return t != nil && t.pending[dir] }

// Result returns the last result for dir.
func (t *Tracker) Result(dir string) (gitexec.Result, bool) {
	if t == nil {
		return gitexec.Result{}, false
	}
	r, ok := t.results[dir]
	return r, ok
}

// AheadBehind returns the known ahead/behind of dir.
func (t *Tracker) AheadBehind(dir string) AB {
	if t == nil {
		return AB{}
	}
	return t.ab[dir]
}

// Start runs op in repos and returns the command listening for results.
// It returns nil when git is unavailable or an operation is running.
func (t *Tracker) Start(op gitexec.Op, repos []*gitremote.Repo) tea.Cmd {
	if !t.Available() || t.busy || len(repos) == 0 {
		return nil
	}
	tok := t.gen.Next()
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.busy, t.op, t.done, t.total, t.run = true, op, 0, len(repos), nil
	t.pending = map[string]bool{}
	list := make([]gitremote.Repo, len(repos))
	for i, r := range repos {
		t.pending[r.Root] = true
		t.repos[r.Root] = r
		t.abGen[r.Root]++ // counts started before the operation are stale
		list[i] = *r
	}
	return listen(tok, op, t.runner.Run(ctx, op, list))
}

func listen(tok common.Token, op gitexec.Op, ch <-chan gitexec.Result) tea.Cmd {
	return func() tea.Msg {
		res, ok := <-ch
		if !ok {
			return DoneMsg{tok: tok, Op: op}
		}
		return ProgressMsg{tok: tok, Result: res, next: listen(tok, op, ch)}
	}
}

// Stop cancels the running operation and kills its git processes at once
// (quit: they run in their own sessions and would outlive the TUI).
func (t *Tracker) Stop() {
	if t == nil {
		return
	}
	if t.cancel != nil {
		t.cancel()
	}
	if k, ok := t.runner.(interface{ Kill() }); ok {
		k.Kill()
	}
}

// CountAheadBehind counts ahead/behind of repos locally, at most eight at
// once; every count arrives as an AheadBehindMsg.
func (t *Tracker) CountAheadBehind(repos []*gitremote.Repo) tea.Cmd {
	if !t.Available() || len(repos) == 0 {
		return nil
	}
	runner := t.runner
	sem := make(chan struct{}, gitexec.AheadBehindParallel)
	cmds := make([]tea.Cmd, 0, len(repos))
	for _, r := range repos {
		t.repos[r.Root] = r
		repo, gen := *r, t.abGen[r.Root]
		cmds = append(cmds, func() tea.Msg {
			sem <- struct{}{}
			defer func() { <-sem }()
			a, b, err := runner.AheadBehind(context.Background(), repo)
			return AheadBehindMsg{Dir: repo.Root, AB: AB{Ahead: a, Behind: b, Known: err == nil}, gen: gen}
		})
	}
	return tea.Batch(cmds...)
}

// Handle applies a tracker message. ok is false for other messages; cmd
// continues listening; done is set when the operation ended.
func (t *Tracker) Handle(msg tea.Msg) (ok, done bool, cmd tea.Cmd) {
	if t == nil {
		return false, false, nil
	}
	switch msg := msg.(type) {
	case ProgressMsg:
		if !t.gen.Valid(msg.tok) {
			return true, false, nil
		}
		r := msg.Result
		t.done++
		t.abGen[r.Dir]++ // the result carries fresher counts than any running one
		delete(t.pending, r.Dir)
		t.results[r.Dir] = r
		t.run = append(t.run, r)
		if r.HasUpstream {
			t.ab[r.Dir] = AB{Ahead: r.Ahead, Behind: r.Behind, Known: true}
		} else if r.Status == gitexec.StatusUpdated {
			t.ab[r.Dir] = AB{}
		}
		if repo := t.repos[r.Dir]; repo != nil && r.Op == gitexec.OpPullFF {
			_ = repo.RefreshHEAD()
		}
		return true, false, msg.next
	case DoneMsg:
		if !t.gen.Valid(msg.tok) {
			return true, false, nil
		}
		t.busy, t.cancel = false, nil
		t.pending = map[string]bool{}
		return true, true, nil
	case AheadBehindMsg:
		if msg.gen != t.abGen[msg.Dir] {
			return true, false, nil // counted before a fetch or pull of the folder
		}
		t.ab[msg.Dir] = msg.AB
		return true, false, nil
	}
	return false, false, nil
}

// ReasonText translates a result's reason.
func ReasonText(m *i18n.Messages, r gitexec.Result) string {
	g := m.Git
	switch r.Reason {
	case gitexec.ReasonAuth:
		return g.ReasonAuth
	case gitexec.ReasonNetwork:
		return g.ReasonNetwork
	case gitexec.ReasonDiverged:
		return g.ReasonDiverged
	case gitexec.ReasonDirty:
		return g.ReasonDirty
	case gitexec.ReasonTimeout:
		return g.ReasonTimeout
	case gitexec.ReasonDetached:
		return g.ReasonDetached
	case gitexec.ReasonNoUpstream:
		return g.ReasonNoUpstream
	case gitexec.ReasonInProgress:
		return g.ReasonInProgress
	case gitexec.ReasonNoRemote:
		return g.ReasonNoRemote
	}
	return r.Detail
}

// Summary is the final toast of the last operation: "fetch: 12 ✓, 1 ✗
// (route-planner-ui: needs authentication)", whether something failed and
// the hint for an authentication failure.
func (t *Tracker) Summary(m *i18n.Messages) (text string, failed bool, hint string) {
	var ok, skipped, bad int
	first := ""
	results := append([]gitexec.Result(nil), t.run...)
	sort.SliceStable(results, func(i, j int) bool { return results[i].Dir < results[j].Dir })
	for _, r := range results {
		switch r.Status {
		case gitexec.StatusUpdated, gitexec.StatusUpToDate:
			ok++
		case gitexec.StatusSkipped:
			skipped++
		default:
			bad++
			if first == "" {
				first = base(r.Dir) + ": " + ReasonText(m, r)
				if r.Reason == gitexec.ReasonAuth {
					hint = m.Git.AuthHint(r.Dir)
				}
			}
		}
	}
	return m.Git.Summary(t.op.String(), ok, skipped, bad, first), bad > 0, hint
}

// Header is the progress shown in every screen's header ("⟳ fetch 7/13"),
// empty when idle.
func (t *Tracker) Header(m *i18n.Messages) string {
	if !t.Busy() {
		return ""
	}
	return m.Git.Progress(t.op.String(), t.done, t.total)
}

// Cell is the ↑↓ cell of a folder: ◐ while running, ✗ / ○ for a failed or
// skipped last operation, "↓3 ↑1" otherwise (empty when even).
func (t *Tracker) Cell(s *common.Styles, dir string) string {
	if !t.Available() {
		return ""
	}
	if t.Running(dir) {
		return s.StatusStyle("running").Render("◐")
	}
	if r, ok := t.results[dir]; ok {
		switch r.Status {
		case gitexec.StatusFailed:
			return s.Error.Render("✗")
		case gitexec.StatusSkipped:
			return s.Warning.Render("○")
		}
	}
	return ABText(t.ab[dir])
}

// ABText renders "↓3 ↑1" (empty when even or unknown).
func ABText(ab AB) string {
	if !ab.Known {
		return ""
	}
	var parts []string
	if ab.Behind > 0 {
		parts = append(parts, "↓"+itoa(ab.Behind))
	}
	if ab.Ahead > 0 {
		parts = append(parts, "↑"+itoa(ab.Ahead))
	}
	return strings.Join(parts, " ")
}

func itoa(n int) string { return strconv.Itoa(n) }

func base(dir string) string { return filepath.Base(dir) }

// PullBody is the text of the pull confirmation: "update 11 repositories,
// skip 2 (detached, no upstream)", from the prechecks of every repository.
func PullBody(m *i18n.Messages, repos []*gitremote.Repo) string {
	update := 0
	var reasons []string
	seen := map[string]bool{}
	for _, r := range repos {
		reason := gitexec.Precheck(*r)
		if r.Remote == "" {
			reason = gitexec.ReasonNoRemote
		}
		if reason == gitexec.ReasonNone {
			update++
			continue
		}
		text := ReasonText(m, gitexec.Result{Reason: reason})
		if !seen[text] {
			seen[text] = true
			reasons = append(reasons, text)
		}
	}
	return m.Git.PullBody(update, len(repos)-update, strings.Join(reasons, ", "))
}
