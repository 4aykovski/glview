package newmr

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

// block is the rows of one field with the suggestions under it.
type block struct {
	id    fieldID
	lines []string
}

func (f *Form) labelWidth() int {
	m := f.env.Msg.NewMR
	w := 0
	for _, l := range []string{m.LblFrom, m.LblInto, m.LblTitle, m.LblTemplate, m.LblDesc, m.LblAssignees, m.LblReviewers, m.LblLabels, m.LblMilestone} {
		w = max(w, ansi.StringWidth(l))
	}
	return w
}

func (f *Form) View() string {
	env := f.env
	s, m := env.Styles, env.Msg
	crumbs := s.Muted.Render(f.project.FullPath+" › "+m.MR.Crumb+" › ") + s.Accent.Render(m.NewMR.Crumb)
	head := common.Header(s, crumbs, env.HeaderRight(time.Time{}), f.w)
	bottom := f.bottomView()
	errLine := ""
	if f.errLine != "" {
		errLine = "  " + render.Clip(s.Error.Render(f.errLine), f.w-4)
	}
	bodyH := max(1, f.h-2-lineCount(bottom)-lineCount(errLine))
	body := f.window(f.blocks(), bodyH)
	parts := []string{head, common.Rule(s, f.w), body}
	if errLine != "" {
		parts = append(parts, errLine)
	}
	parts = append(parts, bottom)
	return strings.Join(parts, "\n")
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// window cuts the rows that fit bodyH out of the blocks: the focused field
// with its suggestions stays visible.
func (f *Form) window(blocks []block, h int) string {
	var lines []string
	start, end := 0, 0
	for _, b := range blocks {
		if b.id == f.focus {
			start = len(lines)
		}
		lines = append(lines, b.lines...)
		if b.id == f.focus {
			end = len(lines)
		}
	}
	if f.focus == fButton {
		start, end = len(lines), len(lines)
	}
	f.scroll = min(f.scroll, start)
	if end > f.scroll+h {
		f.scroll = end - h
	}
	f.scroll = min(f.scroll, start)
	f.scroll = max(0, min(f.scroll, len(lines)-h))

	out := make([]string, h)
	s := f.env.Styles
	for i := range out {
		if f.scroll+i < len(lines) {
			out[i] = render.Clip(lines[f.scroll+i], f.w-1)
		}
		mark := " "
		switch {
		case i == 0 && f.scroll > 0:
			mark = s.Muted.Render("▲")
		case i == h-1 && f.scroll+h < len(lines):
			mark = s.Muted.Render("▼")
		}
		out[i] = render.Pad(out[i], f.w-1) + mark
	}
	return strings.Join(out, "\n")
}

// blocks lays the fields out, top to bottom.
func (f *Form) blocks() []block {
	m := f.env.Msg.NewMR
	lw := f.labelWidth()
	vw := max(10, f.w-4-lw-1)
	f.desc.MinHeight = descMinRows
	row := &rows{f: f, lw: lw, vw: vw}

	var bs []block
	bs = append(bs, block{fSrc, append(row.first(fSrc, m.LblFrom, row.input(f.src, m.PhBranch)), row.hints(f.src)...)})
	dst := append(row.first(fDst, m.LblInto, row.input(f.dst, m.PhBranch)), row.hints(f.dst)...)
	bs = append(bs, block{fDst, append(dst, row.status()...)})
	bs = append(bs, block{fTitle, row.first(fTitle, m.LblTitle, row.text(f.title.View(), f.title.Value() == "", ""))})
	bs = append(bs, block{fDraft, row.check(fDraft, f.draft, m.CheckDraft, "")})
	bs = append(bs, block{fTemplate, row.template()})
	bs = append(bs, block{fDesc, row.desc()})
	for _, c := range []struct {
		id    fieldID
		label string
		h     *hintInput
	}{{fAssignees, m.LblAssignees, f.assignees}, {fReviewers, m.LblReviewers, f.reviewers}, {fLabels, m.LblLabels, f.labels}, {fMilestone, m.LblMilestone, f.milestone}} {
		bs = append(bs, block{c.id, append(row.first(c.id, c.label, row.input(c.h, m.PhSearch)), row.hints(c.h)...)})
	}
	bs = append(bs, block{fRemove, row.check(fRemove, f.remove, m.CheckRemove, "")})
	note := ""
	if f.squashLocked {
		note = m.ByProject
	}
	bs = append(bs, block{fSquash, row.check(fSquash, f.squash, m.CheckSquash, note)})
	bs = append(bs, block{-1, row.preview()})
	for _, b := range bs {
		if b.id == f.focus && len(b.lines) > 0 { // every row starts with two plain columns
			b.lines[0] = f.env.Styles.Accent.Render("›") + b.lines[0][1:]
		}
	}
	return bs
}

// rows renders the rows of the fields.
type rows struct {
	f      *Form
	lw, vw int
}

func (r *rows) first(id fieldID, label, value string) []string {
	s := r.f.env.Styles
	l := render.Pad(label, r.lw)
	if r.f.focus == id {
		l = s.Accent.Bold(true).Render(l)
	} else {
		l = s.Muted.Render(l)
	}
	return []string{"  " + l + "  " + value}
}

func (r *rows) cont(text string) string { return "  " + strings.Repeat(" ", r.lw) + "  " + text }

// text clips a rendered value to the value column.
func (r *rows) text(v string, empty bool, ph string) string {
	if empty && ph != "" {
		return r.f.env.Styles.Muted.Render(ph)
	}
	return render.Clip(v, r.vw)
}

// input is the chips and the text field of h.
func (r *rows) input(h *hintInput, ph string) string {
	s := r.f.env.Styles
	chips := ""
	for _, c := range h.chips {
		chips += s.Accent.Render(c.name) + s.Muted.Render(" ✕") + " "
	}
	if h.full() {
		return strings.TrimRight(chips, " ")
	}
	h.in.Placeholder = ph
	if len(h.chips) > 0 {
		h.in.Placeholder = ""
	}
	h.in.SetWidth(max(6, r.vw-ansi.StringWidth(chips)-1))
	return render.Clip(chips+h.in.View(), r.vw)
}

func (r *rows) hints(h *hintInput) []string {
	if !r.f.hintsShown(h) {
		return nil
	}
	s := r.f.env.Styles
	var out []string
	for i, it := range h.hints {
		mark := "  "
		if i == h.idx {
			mark = s.Accent.Render("› ")
		}
		text := it.name
		switch {
		case it.me:
			text = "@me" + s.Muted.Render(" · "+it.name)
		case it.note != "":
			text += s.Muted.Render("  " + it.note)
		}
		out = append(out, r.cont(render.Clip(mark+text, r.vw)))
	}
	return out
}

// status is the lines under the branches: the comparison, the notes.
func (r *rows) status() []string {
	f := r.f
	s, m := f.env.Styles, f.env.Msg.NewMR
	src, dst := f.src.text(), f.dst.text()
	if src == "" || dst == "" || src == dst {
		return nil
	}
	var out, dup []string
	if st := f.check; st.key == f.pairKey() {
		if !st.done {
			out = append(out, s.Muted.Render(m.Comparing))
		} else {
			switch p := f.problem(st, src, dst); {
			case p == m.CompareErr:
				out = append(out, s.Muted.Render(p))
			case p != "":
				out = append(out, s.Warning.Render(p))
			default:
				out = append(out, s.Muted.Render(m.Stat(len(st.cmp.Commits), st.cmp.Files)))
			}
			if st.mr != nil {
				dup = append(dup, s.Warning.Render(m.DupOpen(src, st.mr.IID)))
			}
		}
	}
	if n := f.unpushed(src); n > 0 {
		out = append(out, s.Muted.Render(m.Unpushed(n)))
	}
	out = append(out, dup...)
	for i := range out {
		out[i] = r.cont(render.Clip(out[i], r.vw))
	}
	return out
}

func (r *rows) check(id fieldID, on bool, label, note string) []string {
	s := r.f.env.Styles
	box := "[ ]"
	if on {
		box = "[x]"
	}
	text := box + " " + label
	if r.f.focus == id {
		text = s.Accent.Render(text)
	} else {
		text = s.Text.Render(text)
	}
	if note != "" {
		text += s.Muted.Render(" · " + note)
	}
	return []string{r.cont(render.Clip(text, r.vw))}
}

func (r *rows) template() []string {
	f := r.f
	s, m := f.env.Styles, f.env.Msg.NewMR
	var value string
	switch {
	case f.tpl.failed:
		value = s.Muted.Render(m.TemplatesFailed)
	case !f.tpl.loaded:
		value = s.Muted.Render(m.TemplatesLoading)
	case f.tpl.sel == 0:
		value = s.Muted.Render(m.NoTemplate)
	default:
		value = s.Text.Render(f.tpl.names[f.tpl.sel-1])
	}
	out := r.first(fTemplate, m.LblTemplate, value)
	if f.focus != fTemplate || !f.tpl.loaded || len(f.tpl.names) == 0 {
		return out
	}
	opts := append([]string{m.NoTemplate}, f.tpl.names...)
	from := max(0, min(f.tpl.hi-2, len(opts)-maxHints))
	for i := from; i < min(len(opts), from+maxHints); i++ {
		mark, text := "  ", s.Text.Render(opts[i])
		if i == f.tpl.sel {
			text = s.Accent.Render(opts[i])
		}
		if i == f.tpl.hi {
			mark = s.Accent.Render("› ")
		}
		out = append(out, r.cont(render.Clip(mark+text, r.vw)))
	}
	return out
}

func (r *rows) desc() []string {
	f := r.f
	m := f.env.Msg.NewMR
	maxRows := max(descMinRows, f.h/3)
	f.desc.MaxHeight = maxRows
	f.desc.Placeholder = m.PhDesc
	f.desc.SetWidth(r.vw)
	f.desc.SetHeight(min(max(f.desc.Height(), descMinRows), maxRows))
	lines := strings.Split(f.desc.View(), "\n")
	out := r.first(fDesc, m.LblDesc, render.Clip(lines[0], r.vw))
	for _, l := range lines[1:] {
		out = append(out, r.cont(render.Clip(l, r.vw)))
	}
	return out
}

// preview is the Muted summary line(s): what is going to be created.
func (r *rows) preview() []string {
	f := r.f
	s, m := f.env.Styles, f.env.Msg.NewMR
	src, dst := f.src.text(), f.dst.text()
	if src == "" || dst == "" {
		return nil
	}
	parts := []string{m.PrevWill(src, dst)}
	if f.draft {
		parts = append(parts, m.PrevDraft)
	}
	switch n := len(f.assignees.chips); {
	case n == 1:
		parts = append(parts, m.PrevAssignee(f.assignees.chips[0].name))
	case n > 1:
		parts = append(parts, m.PrevAssignees(n))
	}
	if n := len(f.reviewers.chips); n > 0 {
		parts = append(parts, m.PrevReviewers(n))
	}
	if n := len(f.labels.chips); n > 0 {
		parts = append(parts, m.PrevLabels(n))
	}
	if len(f.milestone.chips) > 0 {
		parts = append(parts, m.PrevMilestone(f.milestone.chips[0].name))
	}
	out := []string{""}
	for _, l := range wrapParts(parts, f.w-4) {
		out = append(out, "  "+s.Muted.Render(l))
	}
	return out
}

// wrapParts joins parts with " · " and wraps between them at width w.
func wrapParts(parts []string, w int) []string {
	var out []string
	line := ""
	for _, p := range parts {
		switch {
		case line == "":
			line = p
		case ansi.StringWidth(line+" · "+p) <= w:
			line += " · " + p
		default:
			out = append(out, line)
			line = p
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// keysLine is the key hint for the current state.
func (f *Form) keysLine() string {
	m := f.env.Msg.NewMR
	switch {
	case f.creating:
		return m.KeysRun
	case f.waiting:
		return m.KeysWait
	}
	switch f.focus {
	case fTemplate:
		return m.KeysTemplate
	case fDesc:
		return m.KeysDesc
	case fDraft, fRemove, fSquash:
		return m.KeysCheck
	case fButton:
		return m.KeysButton
	}
	if h := f.hintOf(f.focus); h != nil && f.hintsShown(h) {
		return m.KeysHints
	}
	return m.KeysText
}

// bottomView puts the key line (wrapped between its items) left of the
// create button.
func (f *Form) bottomView() string {
	s, m := f.env.Styles, f.env.Msg.NewMR
	button := s.Button.Render("enter " + m.ButtonCreate)
	if f.focus == fButton && !f.creating && !f.waiting {
		button = s.ButtonActive.Render("enter " + m.ButtonCreate)
	}
	keysW := max(10, f.w-4-lipgloss.Width(button)-2)
	lines := wrapParts(strings.Split(f.keysLine(), " · "), keysW)
	for i := range lines {
		lines[i] = render.Pad(s.Muted.Render(lines[i]), keysW)
	}
	joined := lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(lines, "\n"), "  ", button)
	return "  " + strings.ReplaceAll(joined, "\n", "\n  ")
}
