package newmr

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/mrdetail"
)

// snapshot is what enter on the button took from the fields: the checks and
// the creation work with it, not with the fields.
type snapshot struct {
	key                string // the pair of branches
	src, dst           string
	title, description string
	assignees          []item
	reviewers          []item
	labels             []string
	milestone          *item
	remove, squash     bool
}

func (f *Form) snapshot() snapshot {
	s := snapshot{
		key: f.pairKey(), src: f.src.text(), dst: f.dst.text(),
		title: withDraft(f.title.Value(), f.draft), description: f.desc.Value(),
		assignees: append([]item(nil), f.assignees.chips...), reviewers: append([]item(nil), f.reviewers.chips...),
		labels: f.labels.names(), remove: f.remove, squash: f.squash,
	}
	if len(f.milestone.chips) > 0 {
		ms := f.milestone.chips[0]
		s.milestone = &ms
	}
	return s
}

// submit validates the form, takes the snapshot and goes on to the checks.
func (f *Form) submit() tea.Cmd {
	m := f.env.Msg.NewMR
	src, dst := f.src.text(), f.dst.text()
	switch {
	case src == "":
		f.errLine = m.ErrNoBranch
		return f.setFocus(fSrc)
	case dst == "":
		f.errLine = m.ErrNoBranch
		return f.setFocus(fDst)
	case src == dst:
		f.errLine = m.ErrSame
		return f.setFocus(fDst)
	case strings.TrimSpace(f.title.Value()) == "":
		f.errLine = m.ErrNoTitle
		return f.setFocus(fTitle)
	}
	snap := f.snapshot()
	f.snap = &snap
	if st := f.check; st.done && st.key == snap.key {
		return f.proceed()
	}
	f.waiting = true
	f.blurAll()
	if !f.check.started {
		f.tickGen.Next() // the pause is over: ask now
		return f.startCheck()
	}
	return nil
}

// proceed continues with the answer of the check of the snapshot's branches.
func (f *Form) proceed() tea.Cmd {
	st, snap, m := f.check, f.snap, f.env.Msg.NewMR
	if p := f.problem(st, snap.src, snap.dst); p != "" {
		f.errLine, f.snap = p, nil
		return f.focusCmd()
	}
	if st.mr != nil {
		mr := *st.mr
		tok := f.confirmGen.Next()
		f.snap = nil
		return app.OpenOverlay(app.NewConfirm(f.env, m.ConfirmDupTitle, m.ConfirmDup(snap.src, mr.IID, mr.Title),
			func() tea.Msg { return openDupMsg{tok: tok, mr: mr} }).DefaultYes())
	}
	return f.create()
}

func (f *Form) create() tea.Cmd {
	f.creating = true
	f.blurAll()
	snap := *f.snap
	n := gitlab.NewMR{
		SourceBranch: snap.src, TargetBranch: snap.dst, Title: snap.title, Description: snap.description,
		Labels: snap.labels, RemoveSourceBranch: snap.remove, Squash: snap.squash,
	}
	for _, a := range snap.assignees {
		n.AssigneeIDs = append(n.AssigneeIDs, a.id)
	}
	for _, r := range snap.reviewers {
		n.ReviewerIDs = append(n.ReviewerIDs, r.id)
	}
	if snap.milestone != nil {
		n.MilestoneID = snap.milestone.id
	}
	tok := f.runGen.Next()
	client, p := f.env.Client, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		mr, err := client.MergeRequests().Create(ctx, p, n)
		return createdMsg{tok: tok, mr: mr, err: err}
	}
}

func (f *Form) onCreated(msg createdMsg) tea.Cmd {
	if !f.runGen.Valid(msg.tok) || !f.creating {
		return nil
	}
	f.creating = false
	if msg.err != nil {
		return tea.Batch(f.fail(msg.err), f.focusCmd())
	}
	f.env.SetStash(f.draftKey, nil)
	f.invalidate()
	mr := msg.mr
	if mr.Project.FullPath == "" {
		mr.Project = f.project
	}
	p := f.project
	return tea.Sequence(
		app.Toast(common.ToastSuccess, f.env.Msg.NewMR.Created(mr.IID), ""),
		f.openMR(mr),
		func() tea.Msg { return CreatedMsg{Project: p, MR: mr} },
	)
}

// openMR replaces the form with the MR detail of mr.
func (f *Form) openMR(mr gitlab.MergeRequest) tea.Cmd {
	return tea.Sequence(app.Pop(), app.Push(mrdetail.New(f.env, f.project, f.repo, mr, false)))
}

// invalidate drops the cached MR lists of the project: there is a new MR.
func (f *Form) invalidate() {
	if c := f.env.Cache; c != nil {
		c.Invalidate("mrs:" + f.project.FullPath + ":")
		c.Invalidate("mrbranch:" + f.project.FullPath + "@")
	}
}
