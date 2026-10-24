package newmr

import (
	"fmt"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
)

// formState is the draft of a closed form: kept in memory for the session
// by host and project, never on disk (SPEC §3.18).
type formState struct {
	src, dst, title string
	titleFollows    bool
	draft           bool
	template        string // name, "" = none
	desc, subst     string
	assignees       []item
	reviewers       []item
	labels          []item
	milestone       []item
	remove, squash  bool
	removeTouched   bool
	squashTouched   bool
}

func draftKey(env *app.Env, p gitlab.ProjectRef) string {
	return fmt.Sprintf("newmr|%s|%s", env.Host, p.FullPath)
}

// saveDraft keeps the values of the form for the next one of the project.
func (f *Form) saveDraft() {
	d := &formState{
		src: f.src.in.Value(), dst: f.dst.in.Value(), title: f.title.Value(), titleFollows: f.titleFollows,
		draft: f.draft, desc: f.desc.Value(), subst: f.tpl.subst,
		assignees: f.assignees.chips, reviewers: f.reviewers.chips, labels: f.labels.chips, milestone: f.milestone.chips,
		remove: f.remove, squash: f.squash, removeTouched: f.removeTouched, squashTouched: f.squashTouched,
	}
	if f.tpl.sel > 0 && f.tpl.sel <= len(f.tpl.names) {
		d.template = f.tpl.names[f.tpl.sel-1]
	}
	f.env.SetStash(f.draftKey, d)
}

// restore puts a draft into the fresh form.
func (f *Form) restore(d *formState) {
	f.src.in.SetValue(d.src)
	f.dst.in.SetValue(d.dst)
	f.title.SetValue(d.title)
	f.titleFollows = d.titleFollows
	f.draft = d.draft
	f.desc.SetValue(d.desc)
	f.tpl.subst = d.subst
	f.restoredTpl = d.template
	f.keepDesc = d.desc != "" // the template list must not overwrite it
	f.assignees.chips = append([]item(nil), d.assignees...)
	f.reviewers.chips = append([]item(nil), d.reviewers...)
	f.labels.chips = append([]item(nil), d.labels...)
	f.milestone.chips = append([]item(nil), d.milestone...)
	f.remove, f.squash = d.remove, d.squash
	f.removeTouched, f.squashTouched = d.removeTouched, d.squashTouched
}
