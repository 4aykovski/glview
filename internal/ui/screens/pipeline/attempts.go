package pipeline

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/joblog"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

// attemptsList is the overlay with the attempts of a retried job, newest
// first; enter opens the log of the chosen one.
type attemptsList struct {
	env        *app.Env
	ref        gitlab.ProjectRef
	pipelineID int64
	jobs       []gitlab.Job // jobs[0] is the latest attempt
	sel        int
}

func newAttemptsList(env *app.Env, ref gitlab.ProjectRef, pipelineID int64, job gitlab.Job) *attemptsList {
	return &attemptsList{env: env, ref: ref, pipelineID: pipelineID, jobs: attemptsOf(job)}
}

// attemptsOf lists a job and its earlier attempts, newest first.
func attemptsOf(job gitlab.Job) []gitlab.Job {
	return append([]gitlab.Job{job}, job.Attempts...)
}

func (a *attemptsList) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
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
		return nil, app.Push(a.open())
	}
	return a, nil
}

// open is the log of the selected attempt; an earlier one is numbered from
// the oldest: "attempt 2/3".
func (a *attemptsList) open() app.Screen {
	if a.sel == 0 {
		return joblog.New(a.env, a.ref, a.pipelineID, a.jobs[0])
	}
	n := len(a.jobs)
	return joblog.NewAttempt(a.env, a.ref, a.pipelineID, a.jobs[a.sel], a.jobs[0], n-a.sel, n)
}

// View is a table "#id status duration started-ago", the columns aligned;
// the modal is as wide as its content.
func (a *attemptsList) View(width, _ int) string {
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

// attemptsLine is the panel summary "#305 ✓ · #303 ✗ · #302 ✗" within w
// cells; the attempts that do not fit are counted as "+N".
func attemptsLine(st *common.Styles, job gitlab.Job, w int) string {
	jobs := attemptsOf(job)
	var b strings.Builder
	used := 0
	for i, j := range jobs {
		sep := ""
		if i > 0 {
			sep = " · "
		}
		part := fmt.Sprintf("%s#%d ", sep, j.ID)
		partW := ansi.StringWidth(part + common.Glyph(j.Status))
		// Room for this attempt and, unless it is the last, the "+N" after it.
		need := partW
		if left := len(jobs) - i - 1; left > 0 {
			need += ansi.StringWidth(fmt.Sprintf(" · +%d", left))
		}
		if i > 0 && used+need > w {
			b.WriteString(st.Muted.Render(fmt.Sprintf(" · +%d", len(jobs)-i)))
			break
		}
		b.WriteString(st.Muted.Render(part) + st.StatusGlyph(j.Status))
		used += partW
	}
	return b.String()
}
