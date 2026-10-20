package pipeline

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/vartable"
)

// playForm is the overlay that plays a manual job with variables.
type playForm struct {
	env *app.Env
	ref gitlab.ProjectRef
	job gitlab.Job
	vt  *vartable.Model
}

func newPlayForm(env *app.Env, ref gitlab.ProjectRef, job gitlab.Job) *playForm {
	f := env.Msg.Form
	return &playForm{env: env, ref: ref, job: job, vt: vartable.New(env.Styles, vartable.Labels{
		Key: f.ColKey, Value: f.ColValue, Type: f.ColType, Empty: f.VarsEmpty, TypeEnv: f.TypeEnv, TypeFile: f.TypeFile,
		KeyPlaceholder: f.KeyPlaceholder, ValuePlaceholder: f.ValuePlaceholder,
	})}
}

func (f *playForm) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}
	if f.vt.CapturesInput() {
		_, cmd := f.vt.Update(k)
		return f, cmd
	}
	switch k.String() {
	case "esc":
		return nil, nil
	case "ctrl+s":
		return nil, f.play()
	}
	_, cmd := f.vt.Update(k)
	return f, cmd
}

func (f *playForm) play() tea.Cmd {
	client, ref, job, vars := f.env.Client, f.ref, f.job, f.vt.Vars()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		_, err := client.Jobs().Play(ctx, ref, job.ID, vars)
		return jobActionMsg{kind: actPlay, name: job.Name, err: err}
	}
}

func (f *playForm) View(width, _ int) string {
	s, m := f.env.Styles, f.env.Msg
	w := min(72, width-6)
	var b strings.Builder
	b.WriteString(s.ModalTitle.Render(m.Pipe.PlayTitle(f.job.Name)) + "\n\n")
	table := strings.Split(f.vt.View(w-4), "\n")
	for i := range table {
		table[i] = " " + table[i]
	}
	b.WriteString(common.Box(s, strings.Join(table, "\n"), w, len(table)+2, common.BoxActive) + "\n\n")
	b.WriteString(s.Muted.Render(m.Form.VarsHint) + "\n")
	b.WriteString(s.ButtonActive.Render("ctrl+s "+m.Form.RunButton) + "  " + s.Muted.Render(m.Form.RunHint))
	return s.Modal.Width(w + 4).Render(b.String())
}
