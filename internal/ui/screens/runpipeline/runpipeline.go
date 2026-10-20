// Package runpipeline is the "run a new pipeline" overlay: a ref with
// branch and tag autocompletion, a variables table and hints taken from
// .gitlab-ci.yml.
package runpipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/vartable"
	"github.com/4aykovski/glview/internal/ui/screens/pipeline"
)

const (
	debounce       = 300 * time.Millisecond
	requestTimeout = 15 * time.Second
	maxCands       = 6
	searchLimit    = 20
)

// hintRetries are the waits before asking for the CI config variables
// again while GitLab is still computing them.
var hintRetries = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

type candKind int

const (
	kindBranch candKind = iota
	kindTag
	kindCurrent
)

type cand struct {
	name string
	kind candKind
}

type hintState int

const (
	hintsIdle hintState = iota
	hintsLoading
	hintsReady
	hintsUnavailable
)

type (
	searchTickMsg struct{ tok int }
	branchesMsg   struct {
		tok   int
		names []string
		err   error
	}
	tagsMsg struct {
		tok   int
		names []string
		err   error
	}
	hintsMsg struct {
		tok     int
		attempt int
		vars    []gitlab.CIConfigVariable
		err     error
	}
	hintsRetryMsg struct{ tok, attempt int }
	createdMsg    struct {
		p   gitlab.PipelineSummary
		err error
	}
)

// Form is the overlay.
type Form struct {
	env        *app.Env
	ref        gitlab.ProjectRef
	localRef   string // current local branch, "" if unknown
	input      textinput.Model
	vt         *vartable.Model
	focusVars  bool
	inputTok   int // bumped on every edit of the ref (debounce)
	searchTok  int
	cands      []cand
	branches   []string
	tags       []string
	candCursor int
	searching  bool

	hintTok   int
	hintFor   string
	hints     []gitlab.CIConfigVariable
	hintState hintState
	picking   *gitlab.CIConfigVariable // choosing a value for this hint
	pickIdx   int

	creating bool
}

// New creates the form. initial is the starting ref (local branch or the
// default branch), local the current branch of the local clone.
func New(env *app.Env, ref gitlab.ProjectRef, initial, local string) *Form {
	in := common.NewInput(env.Styles)
	in.SetValue(initial)
	in.CursorEnd()
	f := env.Msg.Form
	return &Form{
		env: env, ref: ref, localRef: local, input: in,
		vt: vartable.New(env.Styles, vartable.Labels{Key: f.ColKey, Value: f.ColValue, Type: f.ColType, Empty: f.RunNoVars, TypeEnv: f.TypeEnv, TypeFile: f.TypeFile}),
	}
}

// Init returns the commands that load the first candidates and hints; the
// opener runs it right after OpenOverlay.
func (f *Form) Init() tea.Cmd {
	cmds := []tea.Cmd{f.input.Focus(), f.search()}
	if strings.TrimSpace(f.input.Value()) != "" {
		cmds = append(cmds, f.loadHints(0))
	}
	return tea.Batch(cmds...)
}

// Ref returns the typed ref (for tests).
func (f *Form) Ref() string { return strings.TrimSpace(f.input.Value()) }

// Vars returns the variables of the table (for tests).
func (f *Form) Vars() []gitlab.CIVariable { return f.vt.Vars() }

// DebounceMsg returns the pending debounce tick, so tests can deliver
// timers explicitly.
func (f *Form) DebounceMsg() tea.Msg { return searchTickMsg{tok: f.inputTok} }

// ---- loading ----

func (f *Form) search() tea.Cmd {
	f.searchTok++
	tok, q := f.searchTok, strings.TrimSpace(f.input.Value())
	f.searching = true
	client, ref := f.env.Client, f.ref
	branches := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		names, err := client.Projects().Branches(ctx, ref, q)
		return branchesMsg{tok: tok, names: names, err: err}
	}
	tags := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		page, err := client.Tags().List(ctx, ref, q, "", searchLimit)
		names := make([]string, len(page.Items))
		for i, t := range page.Items {
			names[i] = t.Name
		}
		return tagsMsg{tok: tok, names: names, err: err}
	}
	return tea.Batch(branches, tags)
}

func (f *Form) loadHints(attempt int) tea.Cmd {
	ref := f.Ref()
	if ref == "" {
		return nil
	}
	if attempt == 0 {
		f.hintTok++
		f.hintFor, f.hints, f.hintState = ref, nil, hintsLoading
	}
	tok := f.hintTok
	client, project := f.env.Client, f.ref
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		vars, err := client.Projects().CIConfigVariables(ctx, project, ref)
		return hintsMsg{tok: tok, attempt: attempt, vars: vars, err: err}
	}
}

func (f *Form) rebuildCands() {
	q := strings.ToLower(f.Ref())
	f.cands = f.cands[:0]
	seen := map[string]bool{}
	if f.localRef != "" && strings.Contains(strings.ToLower(f.localRef), q) {
		f.cands = append(f.cands, cand{f.localRef, kindCurrent})
		seen[f.localRef] = true
	}
	for _, b := range f.branches {
		if !seen[b] {
			f.cands = append(f.cands, cand{b, kindBranch})
			seen[b] = true
		}
	}
	for _, t := range f.tags {
		if !seen[t] {
			f.cands = append(f.cands, cand{t, kindTag})
			seen[t] = true
		}
	}
	f.candCursor = max(0, min(f.candCursor, len(f.cands)-1))
}

// ---- update ----

func (f *Form) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	env := f.env
	switch msg := msg.(type) {
	case searchTickMsg:
		if msg.tok != f.inputTok {
			return f, nil // a newer keystroke is pending
		}
		return f, f.search()
	case branchesMsg:
		if msg.tok == f.searchTok && msg.err == nil {
			f.branches = msg.names
			f.rebuildCands()
		}
		return f, f.settle(msg.tok)
	case tagsMsg:
		if msg.tok == f.searchTok && msg.err == nil {
			f.tags = msg.names
			f.rebuildCands()
		}
		return f, f.settle(msg.tok)
	case hintsMsg:
		return f, f.onHints(msg)
	case hintsRetryMsg:
		if msg.tok != f.hintTok {
			return f, nil
		}
		return f, f.loadHints(msg.attempt)
	case createdMsg:
		f.creating = false
		if msg.err != nil {
			return f, env.Fail(msg.err)
		}
		p := msg.p
		if env.Cache != nil {
			env.Cache.Invalidate("pipes:" + f.ref.FullPath + ":")
		}
		return nil, tea.Batch(
			app.Toast(common.ToastSuccess, env.Msg.Form.Created(p.ID), ""),
			app.Push(pipeline.New(env, f.ref, p.ID, &p)),
		)
	case tea.KeyPressMsg:
		return f.key(msg)
	}
	return f, nil
}

// settle ends the searching state once the answers of the latest search are in.
func (f *Form) settle(tok int) tea.Cmd {
	if tok != f.searchTok {
		return nil
	}
	f.searching = false
	// When the typed ref is exactly a known branch or tag, load its hints.
	if ref := f.Ref(); ref != "" && ref != f.hintFor {
		for _, c := range f.cands {
			if c.name == ref {
				return f.loadHints(0)
			}
		}
	}
	return nil
}

func (f *Form) onHints(msg hintsMsg) tea.Cmd {
	if msg.tok != f.hintTok {
		return nil
	}
	switch {
	case msg.err == nil:
		f.hints, f.hintState = msg.vars, hintsReady
	case errors.Is(msg.err, gitlab.ErrNotReady) && msg.attempt < len(hintRetries):
		tok, next := f.hintTok, msg.attempt+1
		return tea.Tick(hintRetries[msg.attempt], func(time.Time) tea.Msg { return hintsRetryMsg{tok: tok, attempt: next} })
	default:
		f.hintState = hintsUnavailable
	}
	return nil
}

func (f *Form) key(k tea.KeyPressMsg) (app.Overlay, tea.Cmd) {
	s := k.String()
	if f.picking != nil {
		return f, f.pickKey(s)
	}
	switch s {
	case "esc":
		if f.focusVars && f.vt.CapturesInput() {
			_, cmd := f.vt.Update(k)
			return f, cmd
		}
		return nil, nil
	case "ctrl+s":
		return f, f.create()
	case "tab":
		if f.focusVars && f.vt.CapturesInput() {
			return f, nil
		}
		f.focusVars = !f.focusVars
		if f.focusVars {
			f.input.Blur()
			return f, nil
		}
		return f, f.input.Focus()
	}
	if f.focusVars {
		return f, f.varsKey(k)
	}
	return f, f.refKey(k)
}

func (f *Form) refKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "down", "ctrl+n":
		f.candCursor = min(f.candCursor+1, max(0, len(f.cands)-1))
		return nil
	case "up", "ctrl+p":
		f.candCursor = max(f.candCursor-1, 0)
		return nil
	case "enter":
		if f.candCursor < len(f.cands) {
			f.input.SetValue(f.cands[f.candCursor].name)
			f.input.CursorEnd()
			f.inputTok++
			return f.loadHints(0)
		}
		return nil
	}
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(k)
	if f.input.Value() == before {
		return cmd
	}
	// Debounce: only the tick of the last keystroke starts a search.
	f.inputTok++
	tok := f.inputTok
	return tea.Batch(cmd, tea.Tick(debounce, func(time.Time) tea.Msg { return searchTickMsg{tok: tok} }))
}

func (f *Form) varsKey(k tea.KeyPressMsg) tea.Cmd {
	if !f.vt.CapturesInput() {
		if s := k.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			if i := int(s[0] - '1'); i < len(f.hints) {
				return f.addHint(f.hints[i])
			}
			return nil
		}
	}
	_, cmd := f.vt.Update(k)
	return cmd
}

func (f *Form) addHint(h gitlab.CIConfigVariable) tea.Cmd {
	if len(h.ValueOptions) > 0 {
		f.picking, f.pickIdx = &h, 0
		for i, o := range h.ValueOptions {
			if o == h.Value {
				f.pickIdx = i
			}
		}
		return nil
	}
	f.vt.Set(h.Key, h.Value)
	return nil
}

func (f *Form) pickKey(s string) tea.Cmd {
	h := f.picking
	switch s {
	case "esc":
		f.picking = nil
	case "j", "down":
		f.pickIdx = min(f.pickIdx+1, len(h.ValueOptions)-1)
	case "k", "up":
		f.pickIdx = max(f.pickIdx-1, 0)
	case "enter":
		f.vt.Set(h.Key, h.ValueOptions[f.pickIdx])
		f.picking = nil
	}
	return nil
}

func (f *Form) create() tea.Cmd {
	env := f.env
	ref := f.Ref()
	if ref == "" {
		return app.Toast(common.ToastWarn, env.Msg.Form.RefRequired, "")
	}
	if f.creating {
		return nil
	}
	f.creating = true
	client, project, vars := env.Client, f.ref, f.vt.Vars()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		p, err := client.Pipelines().Create(ctx, project, ref, vars)
		return createdMsg{p: p, err: err}
	}
}

// ---- view ----

func (f *Form) View(width, _ int) string {
	s, m := f.env.Styles, f.env.Msg
	w := min(76, width-6)
	var b strings.Builder
	b.WriteString(s.ModalTitle.Render(m.Form.RunTitle) + "  " + s.Muted.Render(f.ref.FullPath) + "\n\n")

	refLabel := m.Form.RefLabel
	varsLabel := m.Form.VarsLabel
	refKind, varsKind := common.BoxActive, common.BoxNormal
	if f.focusVars {
		refKind, varsKind = common.BoxNormal, common.BoxActive
		varsLabel = s.Accent.Render(varsLabel)
	} else {
		refLabel = s.Accent.Render(refLabel)
	}
	b.WriteString(refLabel + "\n")
	b.WriteString(common.Box(s, " "+f.input.View(), w, 3, refKind) + "\n")
	if list := f.candidatesView(); list != "" {
		b.WriteString(list + "\n")
	}
	b.WriteString(varsLabel + "\n")
	table := f.vt.View(w - 4)
	rows := strings.Split(table, "\n")
	for i := range rows {
		rows[i] = " " + rows[i]
	}
	b.WriteString(common.Box(s, strings.Join(rows, "\n"), w, len(rows)+2, varsKind) + "\n")
	b.WriteString(f.hintsView(w) + "\n")
	if f.picking != nil {
		b.WriteString("\n" + s.Bold.Render(m.Form.PickValue(f.picking.Key)) + "\n")
		for i, o := range f.picking.ValueOptions {
			if i == f.pickIdx {
				b.WriteString(s.Accent.Render("› ") + o + "\n")
			} else {
				b.WriteString("  " + o + "\n")
			}
		}
	}
	b.WriteString(common.Box(s, " "+s.Muted.Render(m.Form.WillCreate(f.Ref(), f.vt.Len())), w, 3, common.BoxNormal) + "\n")
	keys := s.Muted.Render(m.Form.RunKeys)
	button := s.ButtonActive.Render("ctrl+s " + m.Form.RunButton)
	b.WriteString(button + "  " + keys)
	return s.Modal.Width(w + 4).Render(b.String())
}

func (f *Form) candidatesView() string {
	s, m := f.env.Styles, f.env.Msg.Form
	if f.focusVars {
		return ""
	}
	if len(f.cands) == 0 {
		if f.searching {
			return "  " + s.Muted.Render(m.RefSearching)
		}
		return "  " + s.Muted.Render(m.NoRefs)
	}
	var lines []string
	for i, c := range f.cands[:min(len(f.cands), maxCands)] {
		tag := ""
		switch c.kind {
		case kindCurrent:
			tag = "  " + s.Success.Render("("+m.RefCurrent+")")
		case kindTag:
			tag = "  " + s.Muted.Render("("+m.RefTag+")")
		}
		mark := "  "
		if i == f.candCursor {
			mark = s.Accent.Render("› ")
		}
		lines = append(lines, mark+f.highlight(c.name)+tag)
	}
	return strings.Join(lines, "\n")
}

func (f *Form) hintsView(w int) string {
	s, m := f.env.Styles, f.env.Msg.Form
	switch f.hintState {
	case hintsLoading:
		return s.Muted.Render(m.HintsLoading)
	case hintsUnavailable:
		return s.Muted.Render(m.HintsUnavailable)
	case hintsReady:
		if len(f.hints) == 0 {
			return s.Muted.Render(m.HintsNone)
		}
		parts := make([]string, len(f.hints))
		for i, h := range f.hints {
			p := fmt.Sprintf("%d %s", i+1, h.Key)
			if len(h.ValueOptions) > 0 {
				p += " [" + strings.Join(h.ValueOptions, ", ") + "]"
			}
			parts[i] = p
		}
		return s.Muted.Render(m.HintsLabel) + " " + strings.Join(parts, ", ") + "\n" + s.Muted.Render(m.PickHint)
	}
	return ""
}

// highlight marks the part of a candidate that matches the typed ref.
func (f *Form) highlight(name string) string {
	q := f.Ref()
	if q == "" {
		return name
	}
	i := strings.Index(strings.ToLower(name), strings.ToLower(q))
	if i < 0 {
		return name
	}
	st := f.env.Styles
	return name[:i] + st.Accent.Bold(true).Render(name[i:i+len(q)]) + name[i+len(q):]
}
