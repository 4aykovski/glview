// Package runpipeline is the "run a new pipeline" overlay: a ref with
// branch and tag autocompletion, a variables table and hints taken from
// .gitlab-ci.yml (SPEC §3.9).
package runpipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/common/vartable"
	"github.com/4aykovski/glview/internal/ui/screens/pipeline"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	debounce       = 300 * time.Millisecond
	requestTimeout = 15 * time.Second
	candRows       = 6  // visible ref suggestions
	hintRows       = 8  // visible rows of the hints list
	searchLimit    = 20 // branches and tags per search (the REST page size)
	numberedHints  = 9  // hints added with the keys 1-9
)

// hintRetries are the waits before asking for the CI config variables
// again while GitLab is still computing them.
var hintRetries = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

type candKind int

const (
	kindBranch candKind = iota
	kindTag
)

type cand struct {
	name    string
	kind    candKind
	current bool // the branch of the local clone
}

// focus is the part of the form that receives keys; tab cycles through
// them in this order.
type focus int

const (
	focusRef focus = iota
	focusVars
	focusButton
	focusCount
)

type hintState int

const (
	hintsIdle hintState = iota
	hintsLoading
	hintsReady
	hintsUnavailable
)

// Every request of the form carries a token of one of its generation
// counters (common.Gen): answers to a closed form, or to an earlier request
// of this one, are dropped.
type (
	searchTickMsg struct{ tok common.Token }
	branchesMsg   struct {
		tok   common.Token
		names []string
		err   error
	}
	tagsMsg struct {
		tok   common.Token
		names []string
		err   error
	}
	hintsMsg struct {
		tok     common.Token
		attempt int
		vars    []gitlab.CIConfigVariable
		err     error
	}
	hintsRetryMsg struct {
		tok     common.Token
		attempt int
	}
	createdMsg struct {
		tok common.Token
		p   gitlab.PipelineSummary
		err error
	}
	// tagCheckMsg answers "is ref a tag, and does it already have a pipeline?".
	tagCheckMsg struct {
		tok  common.Token
		run  launch
		tag  bool
		last *gitlab.PipelineSummary
		err  error
	}
	// confirmedMsg is sent by the confirm modal for a tag with a pipeline.
	confirmedMsg struct {
		tok common.Token
		run launch
	}
)

// launch is what enter on the Run button took from the form: the tag check,
// the confirmation and the creation work with it, not with the fields.
type launch struct {
	ref  string
	vars []gitlab.CIVariable
}

// refAnswers are the answers of one search; the suggestions are replaced
// by them only when both have come.
type refAnswers struct {
	query             string
	branches, tags    []string
	branchErr, tagErr error
}

// Form is the overlay.
type Form struct {
	env      *app.Env
	ref      gitlab.ProjectRef
	localRef string // current local branch, "" if unknown
	input    textinput.Model
	vt       *vartable.Model

	focus focus
	// prefilled: the ref was put into the field on open and acts as
	// selected text: it does not filter, the first typed character
	// replaces it.
	prefilled bool

	inputGen  common.Gen // bumped on every edit of the ref (debounce)
	searchGen common.Gen
	next      refAnswers // answers of the latest search so far
	pending   int        // answers of the latest search still to come
	query     string     // query of the shown suggestions
	branches  []string
	tags      []string
	branchErr error
	tagErr    error

	cands      []cand
	candCursor int
	candOff    int    // first visible suggestion
	candShown  int    // suggestion rows that fit the last render
	want       string // put the cursor on this ref once it is listed

	hintGen   common.Gen
	hintFor   string
	hints     []gitlab.CIConfigVariable
	hintState hintState
	picking   *gitlab.CIConfigVariable // choosing a value for this hint
	pickIdx   int
	hintList  bool // the list of all hints is open
	hintIdx   int
	hintOff   int
	hintShown int // hints list rows that fit the last render

	// While the tag check or the creation runs the form is locked.
	runGen   common.Gen
	confirm  *app.Confirm // run on a tag that already has a pipeline
	checking bool         // the tag check is running
	creating bool
}

// New creates the form. initial is the starting ref (local branch or the
// default branch, "" for an empty field), local the current branch of the
// local clone. The order of suggestions follows env.Settings.
func New(env *app.Env, ref gitlab.ProjectRef, initial, local string) *Form {
	in := common.NewInput(env.Styles)
	in.SetValue(initial)
	in.CursorEnd()
	f := env.Msg.Form
	return &Form{
		env: env, ref: ref, localRef: local, input: in,
		prefilled: initial != "", want: initial,
		candShown: candRows, hintShown: hintRows,
		inputGen: common.NewGen(), searchGen: common.NewGen(), hintGen: common.NewGen(), runGen: common.NewGen(),
		vt: vartable.New(env.Styles, vartable.Labels{
			Key: f.ColKey, Value: f.ColValue, Type: f.ColType, Empty: f.RunNoVars, TypeEnv: f.TypeEnv, TypeFile: f.TypeFile,
			KeyPlaceholder: f.KeyPlaceholder, ValuePlaceholder: f.ValuePlaceholder,
		}),
	}
}

// Init returns the commands that load the first candidates and hints; the
// opener runs it right after OpenOverlay.
func (f *Form) Init() tea.Cmd {
	cmds := []tea.Cmd{f.input.Focus(), f.search()}
	if f.Ref() != "" {
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
func (f *Form) DebounceMsg() tea.Msg { return searchTickMsg{tok: f.inputGen.Current()} }

// filter is what the suggestions are searched by: nothing while the
// prefilled value is untouched.
func (f *Form) filter() string {
	if f.prefilled {
		return ""
	}
	return f.Ref()
}

func (f *Form) searching() bool { return f.pending > 0 }

// ---- loading ----

func (f *Form) search() tea.Cmd {
	tok, q := f.searchGen.Next(), f.filter()
	f.next, f.pending = refAnswers{query: q}, 2
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
		f.hintGen.Next()
		f.hintFor, f.hints, f.hintState = ref, nil, hintsLoading
		f.hintList = false
	}
	tok := f.hintGen.Current()
	client, project := f.env.Client, f.ref
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		vars, err := client.Projects().CIConfigVariables(ctx, project, ref)
		return hintsMsg{tok: tok, attempt: attempt, vars: vars, err: err}
	}
}

// rebuildCands orders the suggestions: the local branch first when it is
// prefilled (run_pipeline.prefill_local_branch), then the groups in the
// order of run_pipeline.refs_first.
func (f *Form) rebuildCands() {
	q := strings.ToLower(f.query)
	seen := map[string]bool{}
	var head, branches, tags []cand
	if f.env.Settings.PrefillLocalBranch && f.localRef != "" && strings.Contains(strings.ToLower(f.localRef), q) {
		head = append(head, cand{name: f.localRef, kind: kindBranch, current: true})
		seen[f.localRef] = true
	}
	for _, b := range f.branches {
		if !seen[b] {
			branches = append(branches, cand{name: b, kind: kindBranch, current: b == f.localRef})
			seen[b] = true
		}
	}
	for _, t := range f.tags {
		if !seen[t] {
			tags = append(tags, cand{name: t, kind: kindTag})
			seen[t] = true
		}
	}
	if f.env.Settings.RefsTagsFirst {
		f.cands = slices.Concat(head, tags, branches)
	} else {
		f.cands = slices.Concat(head, branches, tags)
	}
	if f.want != "" {
		if i := slices.IndexFunc(f.cands, func(c cand) bool { return c.name == f.want }); i >= 0 {
			f.candCursor = i
		}
	}
	f.setCursor(f.candCursor)
}

// setCursor moves the suggestion cursor and scrolls the window so the
// selected row stays visible.
func (f *Form) setCursor(i int) {
	f.candCursor = max(0, min(i, len(f.cands)-1))
	f.candOff = window(f.candCursor, f.candOff, len(f.cands), f.candShown)
}

// window returns the first visible row of a list of n rows shown size at
// a time, so that cursor is visible.
func window(cursor, off, n, size int) int {
	if cursor < off {
		off = cursor
	}
	if cursor >= off+size {
		off = cursor - size + 1
	}
	return max(0, min(off, n-size))
}

// groupStarts returns the indexes where a group of suggestions begins.
func (f *Form) groupStarts() []int {
	var out []int
	for i, c := range f.cands {
		if i == 0 || c.kind != f.cands[i-1].kind {
			out = append(out, i)
		}
	}
	return out
}

func (f *Form) nextGroup() {
	for _, s := range f.groupStarts() {
		if s > f.candCursor {
			f.setCursor(s)
			return
		}
	}
}

func (f *Form) prevGroup() {
	starts := f.groupStarts()
	cur := 0
	for _, s := range starts {
		if s <= f.candCursor {
			cur = s
		}
	}
	prev := 0
	for _, s := range starts {
		if s < cur {
			prev = s
		}
	}
	f.setCursor(prev)
}

// ---- update ----

func (f *Form) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	env := f.env
	switch msg := msg.(type) {
	case searchTickMsg:
		if !f.inputGen.Valid(msg.tok) {
			return f, nil // a newer keystroke is pending
		}
		return f, f.search()
	case branchesMsg:
		if !f.searchGen.Valid(msg.tok) {
			return f, nil
		}
		f.next.branches, f.next.branchErr = msg.names, msg.err
		return f, f.answered()
	case tagsMsg:
		if !f.searchGen.Valid(msg.tok) {
			return f, nil
		}
		f.next.tags, f.next.tagErr = msg.names, msg.err
		return f, f.answered()
	case hintsMsg:
		return f, f.onHints(msg)
	case hintsRetryMsg:
		if !f.hintGen.Valid(msg.tok) {
			return f, nil
		}
		return f, f.loadHints(msg.attempt)
	case tagCheckMsg:
		return f, f.onTagCheck(msg)
	case confirmedMsg:
		if !f.runGen.Valid(msg.tok) {
			return f, nil
		}
		return f, f.create(msg.run)
	case createdMsg:
		if !f.runGen.Valid(msg.tok) || !f.creating {
			return f, nil
		}
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

// answered handles one answer (branches or tags) of the latest search.
// Until both are in, the suggestions of the previous search stay; then
// both replace them at once. A failed request empties its group, one toast
// even when both failed.
func (f *Form) answered() tea.Cmd {
	f.pending--
	if f.searching() {
		return nil
	}
	a := f.next
	if a.branchErr != nil {
		a.branches = nil
	}
	if a.tagErr != nil {
		a.tags = nil
	}
	if a.query != f.query {
		f.candCursor, f.candOff = 0, 0
	}
	f.query, f.branches, f.tags, f.branchErr, f.tagErr = a.query, a.branches, a.tags, a.branchErr, a.tagErr
	f.rebuildCands()
	var cmds []tea.Cmd
	switch {
	case f.branchErr != nil:
		cmds = append(cmds, f.env.Fail(f.branchErr))
	case f.tagErr != nil:
		cmds = append(cmds, f.env.Fail(f.tagErr))
	}
	// When the typed ref is exactly a known branch or tag, load its hints.
	if ref := f.Ref(); ref != "" && ref != f.hintFor {
		if slices.ContainsFunc(f.cands, func(c cand) bool { return c.name == ref }) {
			cmds = append(cmds, f.loadHints(0))
		}
	}
	return tea.Batch(cmds...)
}

func (f *Form) onHints(msg hintsMsg) tea.Cmd {
	if !f.hintGen.Valid(msg.tok) {
		return nil
	}
	switch {
	case msg.err == nil:
		f.hints, f.hintState = msg.vars, hintsReady
	case errors.Is(msg.err, gitlab.ErrNotReady) && msg.attempt < len(hintRetries):
		tok, next := msg.tok, msg.attempt+1
		return tea.Tick(hintRetries[msg.attempt], func(time.Time) tea.Msg { return hintsRetryMsg{tok: tok, attempt: next} })
	default:
		f.hintState = hintsUnavailable
	}
	return nil
}

func (f *Form) key(k tea.KeyPressMsg) (app.Overlay, tea.Cmd) {
	if f.confirm != nil {
		o, cmd := f.confirm.Update(k)
		if o == nil {
			f.confirm = nil
		}
		return f, cmd
	}
	s := k.String()
	if f.checking || f.creating {
		// Locked: the run works with what enter took. Esc drops the tag
		// check; the creation is already sent and cannot be cancelled.
		if s == "esc" && f.checking {
			f.runGen.Next()
			f.checking = false
		}
		return f, nil
	}
	if f.picking != nil {
		return f, f.pickKey(s)
	}
	if f.hintList {
		return f, f.hintListKey(s)
	}
	editing := f.focus == focusVars && f.vt.CapturesInput()
	switch s {
	case "esc":
		if editing {
			_, cmd := f.vt.Update(k)
			return f, cmd
		}
		return nil, nil
	case "tab", "shift+tab":
		if editing {
			return f, nil
		}
		step := focus(1)
		if s == "shift+tab" {
			step = focusCount - 1
		}
		return f, f.setFocus((f.focus + step) % focusCount)
	}
	switch f.focus {
	case focusVars:
		return f, f.varsKey(k)
	case focusButton:
		if s == "enter" {
			return f, f.run()
		}
		return f, nil
	}
	return f, f.refKey(k)
}

// setFocus moves the focus; only the ref field holds a text cursor.
func (f *Form) setFocus(to focus) tea.Cmd {
	f.focus = to
	if to == focusRef {
		return f.input.Focus()
	}
	f.input.Blur()
	return nil
}

func (f *Form) refKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "down", "ctrl+n":
		f.want = ""
		f.setCursor(f.candCursor + 1)
		return nil
	case "up", "ctrl+p":
		f.want = ""
		f.setCursor(f.candCursor - 1)
		return nil
	case "pgdown":
		f.want = ""
		f.nextGroup()
		return nil
	case "pgup":
		f.want = ""
		f.prevGroup()
		return nil
	case "enter":
		if f.candCursor < len(f.cands) {
			f.input.SetValue(f.cands[f.candCursor].name)
			f.input.CursorEnd()
			f.prefilled = false
			f.inputGen.Next() // drop a pending debounce
			return f.loadHints(0)
		}
		return nil
	}
	if f.prefilled {
		switch k.String() {
		case "left", "right", "home", "end":
			f.prefilled = false // keep the text, edit it
		case "backspace":
			f.input.SetValue("")
			f.prefilled = false
			return f.edited(nil)
		default:
			if k.Key().Text != "" {
				f.input.SetValue("") // typing replaces the selected text
				f.prefilled = false
			}
		}
	}
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(k)
	if f.input.Value() == before {
		return cmd
	}
	f.prefilled = false
	return f.edited(cmd)
}

// edited starts the debounce after a change of the ref: only the tick of
// the last keystroke starts a search.
func (f *Form) edited(cmd tea.Cmd) tea.Cmd {
	f.want = ""
	tok := f.inputGen.Next()
	return tea.Batch(cmd, tea.Tick(debounce, func(time.Time) tea.Msg { return searchTickMsg{tok: tok} }))
}

func (f *Form) varsKey(k tea.KeyPressMsg) tea.Cmd {
	if !f.vt.CapturesInput() {
		s := k.String()
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			if i := int(s[0] - '1'); i < len(f.hints) {
				return f.addHint(f.hints[i])
			}
			return nil
		}
		if s == "s" {
			if len(f.hints) > 0 {
				f.hintList, f.hintIdx, f.hintOff = true, 0, 0
			}
			return nil
		}
	}
	_, cmd := f.vt.Update(k)
	return cmd
}

func (f *Form) hintListKey(s string) tea.Cmd {
	switch s {
	case "esc":
		f.hintList = false
	case "j", "down":
		f.hintIdx = min(f.hintIdx+1, len(f.hints)-1)
	case "k", "up":
		f.hintIdx = max(f.hintIdx-1, 0)
	case "enter":
		f.hintList = false
		if f.hintIdx < len(f.hints) {
			return f.addHint(f.hints[f.hintIdx])
		}
	}
	f.hintOff = window(f.hintIdx, f.hintOff, len(f.hints), f.hintShown)
	return nil
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

// run handles enter on the Run button: it takes the ref and the variables
// and locks the form. Whether the ref is a tag is decided only by an exact
// tag search; a tag is first checked for an existing pipeline, anything
// else is created right away (SPEC §3.9).
func (f *Form) run() tea.Cmd {
	env := f.env
	if f.checking || f.creating {
		return nil
	}
	ref := f.Ref()
	if ref == "" {
		return app.Toast(common.ToastWarn, env.Msg.Form.RefRequired, "")
	}
	run := launch{ref: ref, vars: f.vt.Vars()}
	f.checking = true
	tok := f.runGen.Next()
	client, project := env.Client, f.ref
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		page, err := client.Tags().List(ctx, project, "^"+ref+"$", "", searchLimit)
		if err != nil {
			return tagCheckMsg{tok: tok, run: run, tag: true, err: err}
		}
		if !slices.ContainsFunc(page.Items, func(t gitlab.Tag) bool { return t.Name == ref }) {
			return tagCheckMsg{tok: tok, run: run}
		}
		pipes, err := client.Pipelines().List(ctx, project, gitlab.PipelineFilter{Ref: ref, Scope: "tags"}, "", 1)
		if err != nil {
			return tagCheckMsg{tok: tok, run: run, tag: true, err: err}
		}
		msg := tagCheckMsg{tok: tok, run: run, tag: true}
		if len(pipes.Items) > 0 {
			msg.last = &pipes.Items[0]
		}
		return msg
	}
}

func (f *Form) onTagCheck(msg tagCheckMsg) tea.Cmd {
	if !f.runGen.Valid(msg.tok) || !f.checking {
		return nil // cancelled with esc, or another form's answer
	}
	f.checking = false
	m := f.env.Msg.Form
	var body string
	switch {
	case !msg.tag:
		return f.create(msg.run)
	case msg.err != nil:
		body = m.TagCheckFailed(app.ErrorText(f.env, msg.err))
	case msg.last != nil:
		body = m.TagRerunBody(msg.last.ID, string(msg.last.Status))
	default:
		return f.create(msg.run)
	}
	tok, run := msg.tok, msg.run
	f.confirm = app.NewConfirm(f.env, m.TagRerunTitle(run.ref), body, func() tea.Msg { return confirmedMsg{tok: tok, run: run} })
	return nil
}

func (f *Form) create(run launch) tea.Cmd {
	if f.creating {
		return nil
	}
	f.creating = true
	tok := f.runGen.Next()
	client, project := f.env.Client, f.ref
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		p, err := client.Pipelines().Create(ctx, project, run.ref, run.vars)
		return createdMsg{tok: tok, p: p, err: err}
	}
}

// ---- view ----

// View renders the form. The suggestion and hint lists show up to 6 and 8
// rows; on a short terminal they shrink so the whole form stays on screen.
func (f *Form) View(width, height int) string {
	if f.confirm != nil {
		return f.confirm.View(width, height)
	}
	base := lipgloss.Height(f.render(width, 1, 1))
	extra := max(0, height-base)
	f.candShown, f.hintShown = min(candRows, 1+extra), min(hintRows, 1+extra)
	f.candOff = window(f.candCursor, f.candOff, len(f.cands), f.candShown)
	f.hintOff = window(f.hintIdx, f.hintOff, len(f.hints), f.hintShown)
	return f.render(width, f.candShown, f.hintShown)
}

// render draws the form with at most candN suggestions and hintN rows of
// the hints list.
func (f *Form) render(width, candN, hintN int) string {
	s, m := f.env.Styles, f.env.Msg
	w := min(76, width-6)
	var b strings.Builder
	b.WriteString(s.ModalTitle.Render(m.Form.RunTitle) + "  " + s.Muted.Render(f.ref.FullPath) + "\n\n")

	refLabel := m.Form.RefLabel
	varsLabel := m.Form.VarsLabel
	refKind, varsKind := common.BoxNormal, common.BoxNormal
	switch f.focus {
	case focusRef:
		refKind = common.BoxActive
		refLabel = s.Accent.Render(refLabel)
	case focusVars:
		varsKind = common.BoxActive
		varsLabel = s.Accent.Render(varsLabel)
	}
	b.WriteString(refLabel + "\n")
	b.WriteString(common.Box(s, " "+f.refView(), w, 3, refKind) + "\n")
	if list := f.candidatesView(w, candN); list != "" {
		b.WriteString(list + "\n")
	}
	b.WriteString(varsLabel + "\n")
	table := f.vt.View(w - 4)
	rows := strings.Split(table, "\n")
	for i := range rows {
		rows[i] = " " + rows[i]
	}
	b.WriteString(common.Box(s, strings.Join(rows, "\n"), w, len(rows)+2, varsKind) + "\n")
	if hints := f.hintsView(w); hints != "" {
		b.WriteString(hints + "\n")
	}
	if f.hintList {
		b.WriteString("\n" + f.hintListView(w, hintN) + "\n")
	}
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
	b.WriteString(f.bottomView(w))
	return s.Modal.Width(w + 4).Render(b.String())
}

// refView is the ref field; the prefilled value is shown as selected text.
func (f *Form) refView() string {
	if f.prefilled && f.input.Focused() {
		return f.env.Styles.RowSelected.Render(f.input.Value())
	}
	return f.input.View()
}

// keysLine is the key hint for the current focus.
func (f *Form) keysLine() string {
	m := f.env.Msg.Form
	switch {
	case f.checking:
		return m.CheckingKeys
	case f.creating:
		return m.CreatingKeys
	case f.picking != nil || f.hintList:
		return m.PickKeys
	case f.focus == focusVars && f.vt.CapturesInput():
		return m.EditKeys
	case f.focus == focusVars:
		return m.VarsKeys
	case f.focus == focusButton:
		return m.ButtonKeys
	}
	return m.RefKeys
}

// bottomView puts the key line (wrapped between its items) left of the
// run button.
func (f *Form) bottomView(w int) string {
	s, m := f.env.Styles, f.env.Msg.Form
	button := s.Button.Render("enter " + m.RunButton)
	if f.focus == focusButton {
		button = s.ButtonActive.Render("enter " + m.RunButton)
	}
	keysW := max(10, w-lipgloss.Width(button)-2)
	var lines []string
	line := ""
	for _, item := range strings.Split(f.keysLine(), " · ") {
		switch {
		case line == "":
			line = item
		case ansi.StringWidth(line+" · "+item) <= keysW:
			line += " · " + item
		default:
			lines = append(lines, line)
			line = item
		}
	}
	lines = append(lines, line)
	for i := range lines {
		lines[i] = render.Pad(s.Muted.Render(lines[i]), keysW)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(lines, "\n"), "  ", button)
}

func (f *Form) candidatesView(w, rows int) string {
	s, m := f.env.Styles, f.env.Msg.Form
	if f.focus != focusRef {
		return ""
	}
	if len(f.cands) == 0 {
		switch {
		case f.searching():
			return "  " + s.Muted.Render(m.RefSearching)
		case f.branchErr != nil && f.tagErr != nil:
			return "  " + s.Muted.Render(m.RefsFailed)
		}
		return "  " + s.Muted.Render(m.NoRefs)
	}
	var lines []string
	off := window(f.candCursor, f.candOff, len(f.cands), rows)
	for i := off; i < min(len(f.cands), off+rows); i++ {
		c := f.cands[i]
		tag := ""
		switch {
		case c.current:
			tag = "  " + s.Success.Render("("+m.RefCurrent+")")
		case c.kind == kindTag:
			tag = "  " + s.Muted.Render("("+m.RefTag+")")
		}
		mark := "  "
		if i == f.candCursor {
			mark = s.Accent.Render("› ")
		}
		lines = append(lines, render.Clip(mark+f.highlight(c.name)+tag, w))
	}
	lines = append(lines, "  "+s.Muted.Render(f.summary()))
	return strings.Join(lines, "\n")
}

// summary is "branches 20+ · tags 7 · 3/27" (+ "searching…" while an
// answer of the latest search is still to come).
func (f *Form) summary() string {
	m := f.env.Msg.Form
	count := func(n int) string {
		if n >= searchLimit {
			return fmt.Sprintf("%d+", n)
		}
		return fmt.Sprint(n)
	}
	groups := []string{m.RefBranches + " " + count(len(f.branches)), m.RefTags + " " + count(len(f.tags))}
	if f.env.Settings.RefsTagsFirst {
		groups[0], groups[1] = groups[1], groups[0]
	}
	out := strings.Join(groups, " · ") + fmt.Sprintf(" · %d/%d", f.candCursor+1, len(f.cands))
	if f.searching() {
		out += "  " + m.RefSearching
	}
	return out
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
			p := h.Key
			if i < numberedHints {
				p = fmt.Sprintf("%d %s", i+1, h.Key)
			}
			if len(h.ValueOptions) > 0 {
				p += " [" + strings.Join(h.ValueOptions, ", ") + "]"
			}
			parts[i] = p
		}
		line := s.Muted.Render(m.HintsLabel) + " " + strings.Join(parts, ", ")
		if ansi.StringWidth(line) <= w {
			return line
		}
		all := "  " + m.HintsAll(len(f.hints))
		return render.Clip(line, w-ansi.StringWidth(all)) + s.Muted.Render(all)
	}
	return ""
}

// hintListView lists every hint with its options and description.
func (f *Form) hintListView(w, rows int) string {
	s, m := f.env.Styles, f.env.Msg.Form
	lines := []string{s.Bold.Render(m.HintsLabel)}
	off := window(f.hintIdx, f.hintOff, len(f.hints), rows)
	for i := off; i < min(len(f.hints), off+rows); i++ {
		h := f.hints[i]
		text := h.Key
		if len(h.ValueOptions) > 0 {
			text += " [" + strings.Join(h.ValueOptions, ", ") + "]"
		}
		if h.Description != "" {
			text += "  " + s.Muted.Render(h.Description)
		}
		mark := "  "
		if i == f.hintIdx {
			mark = s.Accent.Render("› ")
		}
		lines = append(lines, render.Clip(mark+text, w))
	}
	return strings.Join(lines, "\n")
}

// highlight marks the part of a candidate that matches the typed ref.
func (f *Form) highlight(name string) string {
	q := f.filter()
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
