package mractions

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

// mergeable reports whether the dialog may confirm for a status: mergeable,
// or a pipeline that is still running / must pass (auto-merge covers it).
func mergeable(status string) bool {
	switch status {
	case "mergeable", "ci_still_running", "ci_must_pass":
		return true
	}
	return false
}

const (
	fieldMethod = iota
	fieldSquash
	fieldRemove
	fieldCount
)

// MergeDialog is the merge overlay. It has its own controls instead of a
// huh form because enter must confirm from any field.
type MergeDialog struct {
	env     *app.Env
	ref     gitlab.ProjectRef
	mr      gitlab.MergeRequest
	onMerge func(gitlab.MergeRequest, gitlab.MergeOptions) tea.Cmd

	auto, squash, remove bool
	field                int
}

// NewMergeDialog builds the dialog with the defaults from the MR: squash and
// branch removal follow its settings, auto-merge is preselected while the
// pipeline is still running.
func NewMergeDialog(env *app.Env, ref gitlab.ProjectRef, mr gitlab.MergeRequest, onMerge func(gitlab.MergeRequest, gitlab.MergeOptions) tea.Cmd) *MergeDialog {
	auto := mr.MergeStatus == "ci_still_running" || (mr.HeadPipeline != nil && mr.HeadPipeline.Status.Active())
	return &MergeDialog{env: env, ref: ref, mr: mr, onMerge: onMerge, auto: auto, squash: mr.Squash, remove: mr.ForceRemoveSourceBranch}
}

// Options returns the options the dialog would merge with (for tests).
func (d *MergeDialog) Options() gitlab.MergeOptions {
	return gitlab.MergeOptions{AutoMerge: d.auto, Squash: d.squash, RemoveSourceBranch: d.remove, SHA: d.mr.SHA}
}

func (d *MergeDialog) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	blocked := !mergeable(d.mr.MergeStatus)
	switch k.String() {
	case "esc", "q":
		return nil, nil
	case "enter":
		if blocked {
			return d, nil
		}
		return nil, d.onMerge(d.mr, d.Options())
	}
	if blocked {
		return d, nil
	}
	switch k.String() {
	case "j", "down", "tab":
		d.field = (d.field + 1) % fieldCount
	case "k", "up", "shift+tab":
		d.field = (d.field + fieldCount - 1) % fieldCount
	case "left", "h":
		if d.field == fieldMethod {
			d.auto = false
		}
	case "right", "l":
		if d.field == fieldMethod {
			d.auto = true
		}
	case "space":
		switch d.field {
		case fieldMethod:
			d.auto = !d.auto
		case fieldSquash:
			d.squash = !d.squash
		case fieldRemove:
			d.remove = !d.remove
		}
	}
	return d, nil
}

func (d *MergeDialog) View(width, _ int) string {
	s, m := d.env.Styles, d.env.Msg.MR
	var b strings.Builder
	b.WriteString(s.ModalTitle.Render(m.MergeTitle(d.mr.IID)) + "  " + s.Muted.Render(d.mr.Title) + "\n\n")
	if !mergeable(d.mr.MergeStatus) {
		b.WriteString(s.Warning.Render(m.MergeReason(d.mr.MergeStatus, m.StatusLabel(d.mr.MergeStatus))) + "\n\n")
		b.WriteString(s.Muted.Render(m.MergeHintBlocked))
		return s.Modal.MaxWidth(width - 2).Render(b.String())
	}
	opts := strings.Join([]string{
		d.line(fieldMethod, radio(!d.auto)+" "+m.MergeNow+"   "+radio(d.auto)+" "+m.MergeWhenPipeline),
		d.line(fieldSquash, box(d.squash)+" "+m.MergeSquash),
		d.line(fieldRemove, box(d.remove)+" "+m.MergeRemoveBranch),
	}, "\n")
	b.WriteString(common.Box(s, opts, lipgloss.Width(d.line(fieldMethod, radio(false)+" "+m.MergeNow+"   "+radio(true)+" "+m.MergeWhenPipeline))+2, 5, common.BoxActive) + "\n\n")
	b.WriteString(s.Muted.Render(m.MergeHint) + "\n" + s.Muted.Render("sha "+render.ShortSHA(d.mr.SHA)))
	return s.Modal.MaxWidth(width - 2).Render(b.String())
}

func (d *MergeDialog) line(field int, text string) string {
	if d.field == field {
		return d.env.Styles.Accent.Render("› ") + text
	}
	return "  " + text
}

func radio(on bool) string {
	if on {
		return "(•)"
	}
	return "( )"
}

func box(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}
