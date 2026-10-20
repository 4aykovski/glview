package joblog

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

// AttemptsOf lists a job and its earlier attempts, newest first; none of
// them carries attempts of its own.
func AttemptsOf(job gitlab.Job) []gitlab.Job {
	latest := job
	latest.Attempts = nil
	return append([]gitlab.Job{latest}, job.Attempts...)
}

// AttemptsList is the overlay with the attempts of a retried job, newest
// first; enter hands the index of the chosen one to pick.
type AttemptsList struct {
	env  *app.Env
	jobs []gitlab.Job // jobs[0] is the latest attempt
	sel  int
	pick func(i int) tea.Cmd
}

// NewAttemptsList lists jobs (as AttemptsOf returns them) with attempt sel
// selected.
func NewAttemptsList(env *app.Env, jobs []gitlab.Job, sel int, pick func(i int) tea.Cmd) *AttemptsList {
	return &AttemptsList{env: env, jobs: jobs, sel: max(0, min(sel, len(jobs)-1)), pick: pick}
}

func (a *AttemptsList) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return a, nil
	}
	switch k.String() {
	case "esc", "q", "a":
		return nil, nil
	case "j", "down":
		a.sel = min(a.sel+1, len(a.jobs)-1)
	case "k", "up":
		a.sel = max(a.sel-1, 0)
	case "g", "home":
		a.sel = 0
	case "G", "end":
		a.sel = len(a.jobs) - 1
	case "enter":
		return nil, a.pick(a.sel)
	}
	return a, nil
}

// View is a table "#id status duration started-ago", the columns aligned;
// the modal is as wide as its content.
func (a *AttemptsList) View(width, _ int) string {
	st, m := a.env.Styles, a.env.Msg.Pipe
	cells := make([][4]string, len(a.jobs))
	var colW [4]int
	for i, j := range a.jobs {
		c := &cells[i]
		c[0] = fmt.Sprintf("#%d", j.ID)
		c[1] = common.Glyph(j.Status) + " " + string(j.Status)
		if j.Duration > 0 {
			c[2] = render.Duration(j.Duration)
		}
		if j.StartedAt != nil {
			c[3] = render.Ago(a.env.Lang, a.env.Now(), *j.StartedAt)
		}
		for k, v := range c {
			colW[k] = max(colW[k], ansi.StringWidth(v))
		}
	}
	title, keys := m.AttemptsTitle(a.jobs[0].Name), m.AttemptsKeys
	texts := make([]string, len(a.jobs))
	w := max(ansi.StringWidth(title), ansi.StringWidth(keys))
	for i, c := range cells {
		parts := make([]string, len(c))
		for k, v := range c {
			parts[k] = pad(v, colW[k])
		}
		if i == 0 {
			parts = append(parts, m.AttemptLatest)
		}
		texts[i] = strings.TrimRight(strings.Join(parts, "  "), " ")
		w = max(w, ansi.StringWidth(texts[i])+1) // and the selection marker
	}
	w = min(w, width-6)

	rows := make([]string, len(a.jobs))
	for i, j := range a.jobs {
		text := pad(ansi.Truncate(texts[i], w-1, "…"), w-1)
		if i == a.sel {
			rows[i] = st.RowSelected.Render("›" + text)
			continue
		}
		glyph := common.Glyph(j.Status)
		rows[i] = " " + strings.Replace(text, glyph, st.StatusGlyph(j.Status), 1)
	}
	title = st.ModalTitle.Render(ansi.Truncate(title, w, "…"))
	keys = st.Muted.Render(ansi.Truncate(keys, w, "…"))
	return st.Modal.Width(w + 4).Render(title + "\n\n" + strings.Join(rows, "\n") + "\n\n" + keys)
}

func pad(s string, w int) string {
	if n := w - ansi.StringWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
