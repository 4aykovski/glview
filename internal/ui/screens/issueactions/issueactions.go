package issueactions

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

const requestTimeout = 15 * time.Second

// StateMsg reports a finished close or reopen of issue IID of Ref.
type StateMsg struct {
	Owner  common.Token // the screen that asked: it alone toasts
	Ref    gitlab.ProjectRef
	IID    int64
	Closed bool // the issue was closed (false: reopened)
	Issue  gitlab.Issue
	Err    error
}

// Toggle closes an open issue after a confirm (the default is "Cancel") and
// reopens a closed one at once: reopening can be undone, closing is asked
// about (SPEC §13.1, item 12).
func Toggle(env *app.Env, owner common.Token, ref gitlab.ProjectRef, i gitlab.Issue) tea.Cmd {
	closing := i.State != "closed"
	set := setState(env, owner, ref, i.IID, closing)
	if !closing {
		return set
	}
	return app.OpenOverlay(app.NewConfirm(env, env.Msg.Issue.ConfirmClose(i.IID), i.Title, set))
}

func setState(env *app.Env, owner common.Token, ref gitlab.ProjectRef, iid int64, closed bool) tea.Cmd {
	client := env.Client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		issue, err := client.Issues().SetState(ctx, ref, iid, closed)
		return StateMsg{Owner: owner, Ref: ref, IID: iid, Closed: closed, Issue: issue, Err: err}
	}
}

// Done drops the caches of the issue and the project's lists and returns the
// toast; the caller reloads its list or the issue on success.
func Done(env *app.Env, msg StateMsg) tea.Cmd {
	Invalidate(env, msg.Ref, msg.IID)
	if msg.Err != nil {
		return env.Fail(msg.Err)
	}
	m := env.Msg.Issue
	text := m.Reopened(msg.IID)
	if msg.Closed {
		text = m.Closed(msg.IID)
	}
	return app.Toast(common.ToastSuccess, text, "")
}
