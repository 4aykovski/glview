// Package mractions holds the actions on a merge request shared by the MR
// list and the MR detail screen: approve, the merge dialog, rebase with
// progress tracking (and, later, checkout through glab).
package mractions

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/glab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

const (
	actionTimeout = 30 * time.Second
	rebaseEvery   = 2 * time.Second
	rebaseLimit   = 2 * time.Minute
)

// Kind is the type of a finished action.
type Kind int

const (
	KindApprove Kind = iota
	KindUnapprove
	KindMerge
	KindRebase
)

// ActionDoneMsg reports a finished mutation. Mutations do not use the
// screen's generation counter (SPEC §6.4).
type ActionDoneMsg struct {
	Kind   Kind
	Ref    gitlab.ProjectRef
	IID    int64
	Status string // detailed_merge_status the user saw (for error texts)
	Auto   bool   // merge was set to "when the pipeline succeeds"
	Err    error
}

// RebaseOutcome is how a tracked rebase ended.
type RebaseOutcome int

const (
	RebaseNone RebaseOutcome = iota
	RebaseFinished
	RebaseFailed
	RebaseTimeout
)

type (
	rebaseTickMsg   struct{ tok common.Token }
	rebasePolledMsg struct {
		tok common.Token
		mr  gitlab.MergeRequest
		err error
	}
)

// Controller drives the actions of one screen. The screen forwards a, M, b
// keys to Key and every other message to Update.
type Controller struct {
	env  *app.Env
	ref  func() gitlab.ProjectRef
	repo func() *gitremote.Repo // local folder of the project, nil if none

	gen   common.Gen // rebase tracker token
	iid   int64      // MR being tracked
	since time.Time
	on    bool
}

// New creates a controller; ref returns the project (it may gain its ID
// after the project loaded).
func New(env *app.Env, ref func() gitlab.ProjectRef, repo func() *gitremote.Repo) *Controller {
	return &Controller{env: env, ref: ref, repo: repo, gen: common.NewGen()}
}

// CheckoutAvailable reports whether C can work: glab found and the project
// has a local folder.
func (c *Controller) CheckoutAvailable() bool { return c.env.Glab != nil && c.repo() != nil }

// Bindings are the footer/help entries of the MR actions. Checkout is
// hidden without glab or a local folder.
func (c *Controller) Bindings() []common.Binding {
	k := c.env.Msg.Keys
	co := common.NewBinding([]string{"C"}, "", k.Checkout, common.PriorityNormal)
	co.Hidden = !c.CheckoutAvailable()
	return []common.Binding{
		common.NewBinding([]string{"a"}, "", k.Approve, common.PriorityNormal),
		common.NewBinding([]string{"M"}, "", k.Merge, common.PriorityNormal),
		common.NewBinding([]string{"b"}, "", k.Rebase, common.PriorityLow),
		co,
	}
}

// Tracking reports whether a rebase is being tracked.
func (c *Controller) Tracking() bool { return c.on }

// Stop ends rebase tracking (the screen is no longer on top).
func (c *Controller) Stop() {
	c.on = false
	c.gen.Next()
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), actionTimeout)
}

// Key handles a, M and b for mr. handled is false for other keys.
func (c *Controller) Key(k string, mr gitlab.MergeRequest) (cmd tea.Cmd, handled bool) {
	switch k {
	case "a":
		return c.approve(mr), true
	case "M":
		return app.OpenOverlay(NewMergeDialog(c.env, c.ref(), mr, c.merge)), true
	case "C":
		return c.checkout(mr), true
	case "b":
		m := c.env.Msg.MR
		return app.OpenOverlay(app.NewConfirm(c.env, m.RebaseTitle(mr.IID), m.RebaseBody, c.rebase(mr))), true
	}
	return nil, false
}

type glabCheckedMsg struct {
	ok bool
	mr gitlab.MergeRequest
}

// checkout runs `glab mr checkout <iid>` in the project folder. glab login
// is verified once per session.
func (c *Controller) checkout(mr gitlab.MergeRequest) tea.Cmd {
	env := c.env
	if !c.CheckoutAvailable() {
		return nil
	}
	if env.GlabLoggedIn == nil {
		g, host := env.Glab, config.Hostname(env.Host)
		return func() tea.Msg {
			cx, cancel := context.WithTimeout(context.Background(), glab.DefaultTimeout)
			defer cancel()
			return glabCheckedMsg{ok: g.LoggedIn(cx, host), mr: mr}
		}
	}
	return c.runCheckout(mr, *env.GlabLoggedIn)
}

func (c *Controller) runCheckout(mr gitlab.MergeRequest, loggedIn bool) tea.Cmd {
	env := c.env
	if !loggedIn {
		return app.Toast(common.ToastError, env.Msg.MR.GlabNotLoggedIn, env.Msg.Errors.HintGlabToken(config.Hostname(env.Host)))
	}
	action, _ := glab.Find("mr.checkout")
	cmd, err := action.Cmd(env.Glab, glab.TemplateData{Host: env.Host, MR: &mr}, c.repo().Root)
	if err != nil {
		return app.Toast(common.ToastError, err.Error(), "")
	}
	return env.Exec(cmd, func(err error) tea.Msg { return app.ExecFinishedMsg{Action: action.ID, Err: err} })
}

func (c *Controller) approve(mr gitlab.MergeRequest) tea.Cmd {
	client, ref := c.env.Client, c.ref()
	unapprove := mr.Approvals.ApprovedByMe
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		msg := ActionDoneMsg{Kind: KindApprove, Ref: ref, IID: mr.IID, Status: mr.MergeStatus}
		if unapprove {
			msg.Kind = KindUnapprove
			msg.Err = client.MergeRequests().Unapprove(cx, ref, mr.IID)
		} else {
			msg.Err = client.MergeRequests().Approve(cx, ref, mr.IID, mr.SHA)
		}
		return msg
	}
}

func (c *Controller) merge(mr gitlab.MergeRequest, o gitlab.MergeOptions) tea.Cmd {
	client, ref := c.env.Client, c.ref()
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		_, err := client.MergeRequests().Merge(cx, ref, mr.IID, o)
		return ActionDoneMsg{Kind: KindMerge, Ref: ref, IID: mr.IID, Status: mr.MergeStatus, Auto: o.AutoMerge, Err: err}
	}
}

func (c *Controller) rebase(mr gitlab.MergeRequest) tea.Cmd {
	client, ref := c.env.Client, c.ref()
	return func() tea.Msg {
		cx, cancel := ctx()
		defer cancel()
		err := client.MergeRequests().Rebase(cx, ref, mr.IID)
		return ActionDoneMsg{Kind: KindRebase, Ref: ref, IID: mr.IID, Err: err}
	}
}

// Result is what Update tells the screen.
type Result struct {
	Cmd     tea.Cmd
	Reload  bool          // refetch the MR / list now
	Started bool          // a rebase was queued: show the rebase chip
	Outcome RebaseOutcome // a tracked rebase ended
	IID     int64
}

// Update handles ActionDoneMsg and the rebase tracker's messages. active is
// whether the screen is on top: only the top screen reacts to results, so a
// covered screen does not toast twice.
func (c *Controller) Update(msg tea.Msg, active bool) (Result, bool) {
	env := c.env
	switch msg := msg.(type) {
	case ActionDoneMsg:
		if !active {
			return Result{}, true
		}
		c.invalidate(msg.Ref, msg.IID)
		if msg.Err != nil {
			return Result{Cmd: c.errorToast(msg), Reload: msg.Kind != KindRebase}, true
		}
		if msg.Kind == KindRebase {
			c.iid, c.since, c.on = msg.IID, env.Now(), true
			return Result{Cmd: tickAfter(rebaseEvery, c.gen.Next()), Started: true, Reload: false, IID: msg.IID}, true
		}
		return Result{Cmd: c.successToast(msg), Reload: true, IID: msg.IID}, true

	case glabCheckedMsg:
		ok := msg.ok
		env.GlabLoggedIn = &ok
		if !active {
			return Result{}, true
		}
		return Result{Cmd: c.runCheckout(msg.mr, ok)}, true

	case app.ExecFinishedMsg:
		if !active || msg.Action != "mr.checkout" {
			return Result{}, true
		}
		if r := c.repo(); r != nil {
			_ = r.RefreshHEAD()
		}
		c.invalidate(c.ref(), 0)
		res := Result{Reload: true}
		var exit *exec.ExitError
		if errors.As(msg.Err, &exit) {
			res.Cmd = app.Toast(common.ToastError, env.Msg.MR.CheckoutFailed(exit.ExitCode()), "")
		} else if msg.Err != nil {
			res.Cmd = app.Toast(common.ToastError, msg.Err.Error(), "")
		}
		return res, true

	case rebaseTickMsg:
		if !c.gen.Valid(msg.tok) || !c.on || !active {
			return Result{}, true
		}
		client, ref, iid, tok := env.Client, c.ref(), c.iid, msg.tok
		return Result{Cmd: func() tea.Msg {
			cx, cancel := ctx()
			defer cancel()
			mr, err := client.MergeRequests().Get(cx, ref, iid)
			return rebasePolledMsg{tok: tok, mr: mr, err: err}
		}}, true

	case rebasePolledMsg:
		if !c.gen.Valid(msg.tok) || !c.on {
			return Result{}, true
		}
		if msg.err != nil && errors.Is(msg.err, gitlab.ErrUnauthorized) {
			c.Stop()
			return Result{Cmd: env.Fail(msg.err)}, true
		}
		if msg.err == nil && !msg.mr.RebaseInProgress {
			iid := c.iid
			c.Stop()
			c.invalidate(c.ref(), iid)
			out := Result{Reload: true, IID: iid, Outcome: RebaseFinished}
			if msg.mr.MergeError != "" {
				out.Outcome = RebaseFailed
				out.Cmd = app.Toast(common.ToastError, env.Msg.MR.RebaseFailed(msg.mr.MergeError), "")
			} else {
				out.Cmd = app.Toast(common.ToastSuccess, env.Msg.MR.RebaseDone, "")
			}
			return out, true
		}
		if env.Now().Sub(c.since) >= rebaseLimit {
			iid := c.iid
			c.Stop()
			return Result{Reload: true, IID: iid, Outcome: RebaseTimeout,
				Cmd: app.Toast(common.ToastWarn, env.Msg.MR.RebaseStillRunning, "")}, true
		}
		return Result{Cmd: tickAfter(rebaseEvery, c.gen.Current())}, true
	}
	return Result{}, false
}

// tickAfter schedules the next rebase poll with the current token.
func tickAfter(d time.Duration, tok common.Token) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return rebaseTickMsg{tok: tok} })
}

func (c *Controller) invalidate(ref gitlab.ProjectRef, iid int64) {
	if c.env.Cache == nil {
		return
	}
	p := ref.FullPath
	c.env.Cache.Invalidate("mr:" + p + ":" + itoa(iid))
	c.env.Cache.Invalidate("mrdisc:" + p + ":" + itoa(iid))
	c.env.Cache.Invalidate("mrpipes:" + p + ":" + itoa(iid))
	c.env.Cache.Invalidate("mrs:" + p + ":")
}

func (c *Controller) errorToast(msg ActionDoneMsg) tea.Cmd {
	m := c.env.Msg.MR
	if msg.Kind == KindMerge {
		switch {
		case errors.Is(msg.Err, gitlab.ErrConflict):
			return app.Toast(common.ToastError, m.MergePushed, "")
		case errors.Is(msg.Err, gitlab.ErrNotPossible):
			return app.Toast(common.ToastError, m.CannotMerge(msg.Status), "")
		}
	}
	if msg.Kind == KindRebase && errors.Is(msg.Err, gitlab.ErrConflict) {
		return app.Toast(common.ToastError, m.RebaseNotQueued, "")
	}
	return c.env.Fail(msg.Err)
}

func (c *Controller) successToast(msg ActionDoneMsg) tea.Cmd {
	m := c.env.Msg.MR
	switch msg.Kind {
	case KindApprove:
		return app.Toast(common.ToastSuccess, m.ToastApproved(msg.IID), "")
	case KindUnapprove:
		return app.Toast(common.ToastSuccess, m.ToastUnapproved(msg.IID), "")
	case KindMerge:
		if msg.Auto {
			return app.Toast(common.ToastSuccess, m.ToastAutoMerge(msg.IID), "")
		}
		return app.Toast(common.ToastSuccess, m.ToastMerged(msg.IID), "")
	}
	return nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// TickMsg returns the tracker tick that is due next, so tests can deliver
// timers explicitly.
func (c *Controller) TickMsg() tea.Msg { return rebaseTickMsg{tok: c.gen.Current()} }
