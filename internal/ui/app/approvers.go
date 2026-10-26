package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

// ApproversRefreshMsg replaces the list of the open approvers overlay of the
// MR identified by Key (a screen sends it when it gets fresh MR details).
type ApproversRefreshMsg struct {
	Key string
	MR  gitlab.MergeRequest
}

// ApproversOverlay is the A overlay: everybody the APPROVALS column cuts
// down; enter opens the profile, y copies @username, esc closes (SPEC 3.5,
// "Оверлей A").
type ApproversOverlay struct {
	env   *Env
	key   string // identifies the MR for ApproversRefreshMsg
	ref   string // "!479", shown in the title
	modal *common.ApproversModal
}

var _ Overlay = (*ApproversOverlay)(nil)

// NewApproversOverlay opens the approvers list of mr, identified by key.
func NewApproversOverlay(env *Env, key, ref string, mr gitlab.MergeRequest) *ApproversOverlay {
	o := &ApproversOverlay{env: env, key: key, ref: ref, modal: common.NewApproversModal(nil)}
	o.modal.SetRows(o.rows(mr))
	return o
}

func (o *ApproversOverlay) rows(mr gitlab.MergeRequest) []common.ApproverRow {
	return render.ApproverRows(o.env.Styles, o.env.Msg, render.NewApprovalView(mr, o.env.User.Username))
}

func (o *ApproversOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case ApproversRefreshMsg:
		if msg.Key == o.key {
			o.modal.SetRows(o.rows(msg.MR))
		}
	case tea.KeyPressMsg:
		switch k := msg.String(); k {
		case "esc", "q", "A":
			return nil, nil
		case "enter":
			if r, ok := o.modal.Selected(); ok && r.User.WebURL != "" {
				return nil, OpenURL(o.env, r.User.WebURL)
			}
		case "y":
			if r, ok := o.modal.Selected(); ok {
				return nil, Copy(o.env, "@"+r.User.Username, "@"+r.User.Username) // closes: SPEC 3.5
			}
		default:
			o.modal.Move(k)
		}
	}
	return o, nil
}

func (o *ApproversOverlay) View(width, height int) string {
	m := o.env.Msg.Approvers
	return o.modal.View(o.env.Styles, m.Title(o.ref), m.Hints, width, height)
}

// ApproversRefresh tells the open approvers overlay of the MR identified by
// key that the MR changed.
func ApproversRefresh(key string, mr gitlab.MergeRequest) tea.Cmd {
	return msgCmd(ApproversRefreshMsg{Key: key, MR: mr})
}

// OpenApprovers shows the approvers overlay of mr, or the "no approvers"
// toast. ref is "!479".
func OpenApprovers(env *Env, key, ref string, mr gitlab.MergeRequest) tea.Cmd {
	if !render.NewApprovalView(mr, env.User.Username).People() {
		return Toast(common.ToastInfo, env.Msg.Approvers.NoApprovers, "")
	}
	return OpenOverlay(NewApproversOverlay(env, key, ref, mr))
}
