// Package newmr is the New MR screen: a form on the navigation stack with
// every field of POST /merge_requests except forks, checks of the branches
// before the MR is created, and the MR detail of the new MR afterwards
// (SPEC §3.18).
package newmr

import (
	"context"
	"errors"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

const (
	debounce       = 300 * time.Millisecond
	requestTimeout = 15 * time.Second
	suggestTTL     = 5 * time.Minute
	maxHints       = 5 // suggestions under a field
	descMinRows    = 3
)

// CreatedMsg reports a created MR; the MR lists of the project reload.
type CreatedMsg struct {
	Project gitlab.ProjectRef
	MR      gitlab.MergeRequest
}

// Every request carries a token of one of the form's generation counters:
// answers of a closed form or of an earlier request are dropped.
type (
	projectMsg struct {
		tok common.Token
		p   gitlab.Project
		err error
	}
	templatesMsg struct {
		tok   common.Token
		names []string
		err   error
	}
	templateMsg struct {
		tok     common.Token
		name    string
		content string
		force   bool // the user chose it: replaces the description whatever it holds
		err     error
	}
	// tickMsg ends the pause after an edit of a branch or of a suggesting
	// field: the suggestions and the branch check are asked.
	tickMsg  struct{ tok common.Token }
	hintsMsg struct {
		id    fieldID
		tok   common.Token
		items []item
		err   error
	}
	checkMsg struct {
		tok common.Token
		st  checkState
	}
	// replaceMsg is the "Yes" of the replace-description confirm.
	replaceMsg struct {
		tok common.Token
		sel int
	}
	// openDupMsg is the "Yes" of the open-the-existing-MR confirm.
	openDupMsg struct {
		tok common.Token
		mr  gitlab.MergeRequest
	}
	createdMsg struct {
		tok common.Token
		mr  gitlab.MergeRequest
		err error
	}
)

// Options say what the form is opened for.
type Options struct {
	Project       gitlab.ProjectRef
	Repo          *gitremote.Repo // the local folder of the project, nil without one
	DefaultBranch string          // known default branch, "" when not yet
}

// tplState is the description template field.
type tplState struct {
	loaded, failed bool
	names          []string
	sel            int    // 0 = no template, i = names[i-1]
	hi             int    // highlighted option while the list is open
	subst          string // the text the description got from a template
	gen            common.Gen
}

// Form is the screen.
type Form struct {
	env      *app.Env
	project  gitlab.ProjectRef
	repo     *gitremote.Repo
	draftKey string

	src, dst, assignees, reviewers, labels, milestone *hintInput

	title        textinput.Model
	titleFollows bool // the title follows the branches until edited
	desc         textarea.Model

	draft, remove, squash bool
	// removeTouched and squashTouched stop the project settings from
	// overriding what the user chose; squashLocked is squash_option
	// always/never.
	removeTouched, squashTouched, squashLocked bool
	defaultBranch                              string
	localBranch                                string

	tpl         tplState
	restoredTpl string // template name of a restored draft, resolved when the list arrives
	keepDesc    bool

	focus fieldID

	check checkState
	snap  *snapshot // what enter on the button took from the fields
	// waiting: the snapshot waits for the check of its branches.
	waiting  bool
	creating bool
	errLine  string

	w, h, scroll int

	tickGen, checkGen, projGen, listGen, confirmGen, runGen common.Gen
}

// New creates the form for o.Project; a draft of an earlier form of the
// same project is restored.
func New(env *app.Env, o Options) *Form {
	f := &Form{
		env: env, project: o.Project, repo: o.Repo, defaultBranch: o.DefaultBranch,
		draftKey:     draftKey(env, o.Project),
		titleFollows: true, remove: true,
		tickGen: common.NewGen(), checkGen: common.NewGen(), projGen: common.NewGen(),
		listGen: common.NewGen(), confirmGen: common.NewGen(), runGen: common.NewGen(),
	}
	f.tpl.gen = common.NewGen()
	s := env.Styles
	f.src = newHintInput(s, fSrc, false, 0)
	f.dst = newHintInput(s, fDst, false, 0)
	f.assignees = newHintInput(s, fAssignees, true, 0)
	f.reviewers = newHintInput(s, fReviewers, true, 0)
	f.labels = newHintInput(s, fLabels, true, 0)
	f.milestone = newHintInput(s, fMilestone, true, 1)
	f.title = common.NewInput(s)
	f.desc = common.NewTextArea(s)
	f.desc.DynamicHeight = true
	f.desc.MinHeight = descMinRows
	f.desc.ShowLineNumbers = false

	f.localBranch = localBranch(o.Repo, o.DefaultBranch)
	f.src.in.SetValue(f.localBranch)
	f.dst.in.SetValue(o.DefaultBranch)
	if d, ok := env.Stash(f.draftKey).(*formState); ok {
		f.restore(d)
	}
	for _, h := range f.inputs() {
		h.in.CursorEnd()
	}
	f.title.CursorEnd()
	f.markPair()
	f.focusCmd()
	return f
}

// localBranch is the branch of the local folder worth proposing: not a
// detached HEAD and not the default branch.
func localBranch(repo *gitremote.Repo, defaultBranch string) string {
	if repo == nil || repo.Detached || repo.Branch == "" || repo.Branch == defaultBranch {
		return ""
	}
	return repo.Branch
}

func (f *Form) inputs() []*hintInput {
	return []*hintInput{f.src, f.dst, f.assignees, f.reviewers, f.labels, f.milestone}
}

func (f *Form) hintOf(id fieldID) *hintInput {
	for _, h := range f.inputs() {
		if h.id == id {
			return h
		}
	}
	return nil
}

// Branches returns the typed branches (for tests).
func (f *Form) Branches() (src, dst string) { return f.src.text(), f.dst.text() }

// TitleText returns the title as it stands (for tests).
func (f *Form) TitleText() string { return strings.TrimSpace(f.title.Value()) }

// Description returns the description as it stands (for tests).
func (f *Form) Description() string { return f.desc.Value() }

// Focus returns the focused field (for tests).
func (f *Form) Focus() fieldID { return f.focus }

// Init asks for the project settings and the templates and checks the
// branches.
func (f *Form) Init() tea.Cmd {
	return tea.Batch(f.projectCmd(), f.templatesCmd(), f.startCheck(), f.focusCmd())
}

func (f *Form) projectCmd() tea.Cmd {
	tok := f.projGen.Next()
	client, p := f.env.Client, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		pr, err := client.Projects().Get(ctx, p)
		return projectMsg{tok: tok, p: pr, err: err}
	}
}

func (f *Form) templatesCmd() tea.Cmd {
	tok := f.listGen.Next()
	env, p := f.env, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		names, _, err := common.Fetch(ctx, env.Cache, "mrtpl:"+p.FullPath, suggestTTL, false, func(ctx context.Context) ([]string, error) {
			return env.Client.Projects().MRTemplates(ctx, p)
		})
		return templatesMsg{tok: tok, names: names, err: err}
	}
}

func (f *Form) templateCmd(name string, force bool) tea.Cmd {
	tok := f.tpl.gen.Next()
	env, p := f.env, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		content, err := env.Client.Projects().MRTemplate(ctx, p, name)
		return templateMsg{tok: tok, name: name, content: content, force: force, err: err}
	}
}

// ---- screen ----

func (f *Form) Title() string { return f.project.FullPath + " › " + f.env.Msg.NewMR.Crumb }

func (f *Form) KeyMap() common.KeyMap {
	m := f.env.Msg.NewMR
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"tab"}, "tab", m.KeyField, common.PriorityHigh),
		common.NewBinding([]string{"esc"}, "esc", m.KeyClose, common.PriorityHigh),
	}}
}

func (f *Form) SetSize(w, h int) { f.w, f.h = w, h }

// CapturesInput: every key belongs to the form, the global shortcuts (p, F,
// T, ?, q, r) do not act while it is open.
func (f *Form) CapturesInput() bool { return true }

// ---- update ----

func (f *Form) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case projectMsg:
		return f, f.onProject(msg)
	case templatesMsg:
		return f, f.onTemplates(msg)
	case templateMsg:
		return f, f.onTemplate(msg)
	case tickMsg:
		if !f.tickGen.Valid(msg.tok) {
			return f, nil // a newer keystroke is pending
		}
		return f, f.onTick()
	case hintsMsg:
		return f, f.onHints(msg)
	case checkMsg:
		return f, f.onCheck(msg)
	case replaceMsg:
		if !f.confirmGen.Valid(msg.tok) {
			return f, nil
		}
		f.tpl.sel = msg.sel
		return f, f.templateCmd(f.tpl.names[msg.sel-1], true)
	case openDupMsg:
		if !f.confirmGen.Valid(msg.tok) {
			return f, nil
		}
		f.saveDraft()
		return f, f.openMR(msg.mr)
	case createdMsg:
		return f, f.onCreated(msg)
	case app.ThemeChangedMsg:
		for _, h := range f.inputs() {
			h.in.SetStyles(msg.Styles.Input)
		}
		f.title.SetStyles(msg.Styles.Input)
		return f, nil
	case tea.KeyPressMsg:
		return f, f.key(msg)
	case tea.PasteMsg:
		return f, f.edit(msg)
	}
	return f, nil
}

// onProject takes the settings of the project: the default branch and the
// defaults of the flags.
func (f *Form) onProject(msg projectMsg) tea.Cmd {
	if !f.projGen.Valid(msg.tok) {
		return nil
	}
	if msg.err != nil {
		return f.env.Fail(msg.err)
	}
	p := msg.p
	f.defaultBranch = p.DefaultBranch
	changed := false
	if f.dst.text() == "" && p.DefaultBranch != "" {
		f.dst.in.SetValue(p.DefaultBranch)
		f.dst.in.CursorEnd()
		changed = true
	}
	if f.src.text() != "" && f.src.text() == p.DefaultBranch && f.localBranch == p.DefaultBranch {
		f.src.in.SetValue("") // the folder sits on the default branch
		changed = true
	}
	if !f.removeTouched {
		f.remove = p.RemoveSourceBranchAfterMerge
	}
	switch p.SquashOption {
	case "always":
		f.squash, f.squashLocked = true, true
	case "never":
		f.squash, f.squashLocked = false, true
	case "default_on":
		if !f.squashTouched {
			f.squash = true
		}
	case "default_off":
		if !f.squashTouched {
			f.squash = false
		}
	}
	if changed {
		f.markPair()
		return f.startCheck()
	}
	return nil
}

func (f *Form) onTemplates(msg templatesMsg) tea.Cmd {
	if !f.listGen.Valid(msg.tok) {
		return nil
	}
	if msg.err != nil {
		f.tpl.failed = true
		return nil
	}
	f.tpl.loaded, f.tpl.names = true, msg.names
	if f.restoredTpl != "" || f.keepDesc {
		for i, n := range msg.names {
			if n == f.restoredTpl {
				f.tpl.sel = i + 1
			}
		}
		f.restoredTpl, f.keepDesc = "", false
		return nil
	}
	if strings.TrimSpace(f.desc.Value()) != "" {
		return nil
	}
	for i, n := range msg.names {
		if strings.EqualFold(n, "default") {
			f.tpl.sel, f.tpl.hi = i+1, i+1
			return f.templateCmd(n, false)
		}
	}
	return nil
}

func (f *Form) onTemplate(msg templateMsg) tea.Cmd {
	if !f.tpl.gen.Valid(msg.tok) {
		return nil
	}
	if msg.err != nil {
		return f.env.Fail(msg.err)
	}
	if !msg.force && strings.TrimSpace(f.desc.Value()) != "" {
		return nil // typed meanwhile: the template does not overwrite it
	}
	f.desc.SetValue(msg.content)
	f.tpl.subst = msg.content
	return nil
}

// onTick asks the suggestions of the fields edited and the check of the
// branches.
func (f *Form) onTick() tea.Cmd {
	var cmds []tea.Cmd
	for _, h := range f.inputs() {
		if h.dirty && f.focus == h.id {
			cmds = append(cmds, f.fetchHints(h))
		}
	}
	if !f.check.started {
		cmds = append(cmds, f.startCheck())
	}
	return tea.Batch(cmds...)
}

func (f *Form) onHints(msg hintsMsg) tea.Cmd {
	h := f.hintOf(msg.id)
	if h == nil || !h.gen.Valid(msg.tok) {
		return nil
	}
	if msg.err != nil {
		h.hints = nil
		return f.env.Fail(msg.err)
	}
	h.hints = f.filterHints(h, msg.items)
	h.idx = 0
	return nil
}

// filterHints drops the chosen values, puts "@me" first in the person
// fields and keeps the first maxHints.
func (f *Form) filterHints(h *hintInput, items []item) []item {
	var out []item
	if (h.id == fAssignees || h.id == fReviewers) && f.env.User.ID != 0 { // "@me" needs the user loaded
		q := strings.ToLower(h.text())
		me := item{id: f.env.User.ID, name: f.env.User.Username, me: true}
		if !h.has(me) && (q == "" || strings.Contains("@me", q) || strings.Contains(strings.ToLower(me.name), q)) {
			out = append(out, me)
		}
	}
	for _, it := range items {
		if !h.has(it) && (len(out) == 0 || !out[0].me || !out[0].same(it)) {
			out = append(out, it)
		}
	}
	return out[:min(len(out), maxHints)]
}

// fetchHints asks the suggestions of h for its text.
func (f *Form) fetchHints(h *hintInput) tea.Cmd {
	tok := h.gen.Next()
	h.dirty = false
	q := h.text()
	id := h.id
	env, p := f.env, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		cached := func(kind string, fn func(context.Context) ([]item, error)) ([]item, error) {
			key := "mrsugg:" + p.FullPath + ":" + kind + ":" + strings.ToLower(q)
			items, _, err := common.Fetch(ctx, env.Cache, key, suggestTTL, false, fn)
			return items, err
		}
		var (
			items []item
			err   error
		)
		switch id {
		case fSrc, fDst:
			var names []string
			names, err = env.Client.Projects().Branches(ctx, p, q)
			for _, n := range names {
				items = append(items, item{name: n})
			}
		case fAssignees, fReviewers:
			items, err = cached("members", func(ctx context.Context) ([]item, error) {
				users, err := env.Client.Projects().Members(ctx, p, q)
				out := make([]item, len(users))
				for i, u := range users {
					out[i] = item{id: u.ID, name: u.Username, note: u.Name}
				}
				return out, err
			})
		case fLabels:
			items, err = cached("labels", func(ctx context.Context) ([]item, error) {
				names, err := env.Client.Projects().Labels(ctx, p, q)
				out := make([]item, len(names))
				for i, n := range names {
					out[i] = item{name: n}
				}
				return out, err
			})
		case fMilestone:
			items, err = cached("milestones", func(ctx context.Context) ([]item, error) {
				ms, err := env.Client.Projects().Milestones(ctx, p, q)
				out := make([]item, len(ms))
				for i, m := range ms {
					out[i] = item{id: m.ID, name: m.Title}
				}
				return out, err
			})
		}
		return hintsMsg{id: id, tok: tok, items: items, err: err}
	}
}

// ---- keys ----

func (f *Form) key(k tea.KeyPressMsg) tea.Cmd {
	if f.creating {
		return nil // the request is sent and cannot be taken back
	}
	s := k.String()
	if f.waiting {
		if s == "esc" {
			f.waiting, f.snap = false, nil
		}
		return nil
	}
	f.errLine = ""
	switch s {
	case "esc":
		f.saveDraft()
		return app.Pop()
	case "tab":
		return f.setFocus((f.focus + 1) % fieldCount)
	case "shift+tab":
		return f.setFocus((f.focus + fieldCount - 1) % fieldCount)
	}
	switch f.focus {
	case fSrc, fDst, fAssignees, fReviewers, fLabels, fMilestone:
		return f.hintKey(f.hintOf(f.focus), k)
	case fTitle:
		if s == "enter" {
			return f.setFocus(fDraft)
		}
		return f.edit(k)
	case fDraft, fRemove, fSquash:
		switch s {
		case "space", "x":
			f.toggle(f.focus)
		case "enter":
			return f.setFocus(f.focus + 1)
		}
		return nil
	case fTemplate:
		return f.templateKey(s)
	case fDesc:
		return f.edit(k)
	}
	if s == "enter" { // the button
		return f.submit()
	}
	return nil
}

func (f *Form) toggle(id fieldID) {
	switch id {
	case fDraft:
		f.draft = !f.draft
	case fRemove:
		f.remove, f.removeTouched = !f.remove, true
	case fSquash:
		if f.squashLocked {
			return
		}
		f.squash, f.squashTouched = !f.squash, true
	}
}

// blurAll drops the focus of every control.
func (f *Form) blurAll() {
	for _, h := range f.inputs() {
		h.in.Blur()
	}
	f.title.Blur()
	f.desc.Blur()
}

// focusCmd focuses the control of the focused field.
func (f *Form) focusCmd() tea.Cmd {
	f.blurAll()
	switch f.focus {
	case fTitle:
		return f.title.Focus()
	case fDesc:
		return f.desc.Focus()
	}
	if h := f.hintOf(f.focus); h != nil && !h.full() {
		return h.in.Focus()
	}
	return nil
}

func (f *Form) setFocus(to fieldID) tea.Cmd {
	f.focus = to
	cmds := []tea.Cmd{f.focusCmd()}
	switch h := f.hintOf(to); {
	case h != nil && h.chip:
		h.hide = false
		cmds = append(cmds, f.fetchHints(h))
	case to == fTemplate:
		f.tpl.hi = f.tpl.sel
	}
	return tea.Batch(cmds...)
}

// edit passes a key or a paste to the focused text control and reacts to a
// changed value.
func (f *Form) edit(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch f.focus {
	case fTitle:
		before := f.title.Value()
		f.title, cmd = f.title.Update(msg)
		if f.title.Value() != before {
			f.titleFollows = false
		}
	case fDesc:
		f.desc, cmd = f.desc.Update(msg)
	default:
		h := f.hintOf(f.focus)
		if h == nil || h.full() {
			return nil
		}
		before := h.in.Value()
		h.in, cmd = h.in.Update(msg)
		if h.in.Value() != before {
			return tea.Batch(cmd, f.edited(h))
		}
	}
	return cmd
}

// edited reacts to a changed text of h: suggestions and, for a branch, the
// check are asked after a pause.
func (f *Form) edited(h *hintInput) tea.Cmd {
	h.hide, h.dirty = false, true
	if h == f.src || h == f.dst {
		f.markPair()
	}
	tok := f.tickGen.Next()
	return tea.Tick(debounce, func(time.Time) tea.Msg { return tickMsg{tok: tok} })
}

func (f *Form) hintsShown(h *hintInput) bool {
	return f.focus == h.id && !h.hide && len(h.hints) > 0 && !h.full()
}

func (f *Form) hintKey(h *hintInput, k tea.KeyPressMsg) tea.Cmd {
	shown := f.hintsShown(h)
	switch k.String() {
	case "down":
		if shown {
			h.idx = min(h.idx+1, len(h.hints)-1)
		}
		return nil
	case "up":
		if shown {
			h.idx = max(h.idx-1, 0)
		}
		return nil
	case "enter":
		if shown {
			return f.pick(h)
		}
		return f.setFocus(f.focus + 1)
	case "backspace":
		if h.chip && len(h.chips) > 0 && (h.in.Value() == "" || h.full()) {
			h.chips = h.chips[:len(h.chips)-1]
			h.hide = false
			return tea.Batch(f.focusCmd(), f.fetchHints(h))
		}
	}
	return f.edit(k)
}

// pick takes the highlighted suggestion.
func (f *Form) pick(h *hintInput) tea.Cmd {
	it := h.hints[h.idx]
	if h.chip {
		h.chips = append(h.chips, it)
		h.in.SetValue("")
		h.hide = false
		if h.full() {
			h.in.Blur()
			h.hints = nil
			return nil
		}
		return f.fetchHints(h)
	}
	before := h.in.Value()
	h.in.SetValue(it.name)
	h.in.CursorEnd()
	h.hide = true
	if h.in.Value() == before {
		return nil
	}
	f.tickGen.Next() // drop a pending pause
	f.markPair()
	return f.startCheck()
}

func (f *Form) templateKey(s string) tea.Cmd {
	n := len(f.tpl.names) + 1
	if !f.tpl.loaded || n == 1 {
		if s == "enter" {
			return f.setFocus(fDesc)
		}
		return nil
	}
	switch s {
	case "down":
		f.tpl.hi = min(f.tpl.hi+1, n-1)
	case "up":
		f.tpl.hi = max(f.tpl.hi-1, 0)
	case "enter":
		if f.tpl.hi == f.tpl.sel {
			return f.setFocus(fDesc)
		}
		return f.chooseTemplate(f.tpl.hi)
	}
	return nil
}

// chooseTemplate selects option i (0 = no template). A description edited
// after the last substitution is replaced only after a confirmation.
func (f *Form) chooseTemplate(i int) tea.Cmd {
	if i == 0 {
		f.tpl.sel = 0
		return nil
	}
	d := f.desc.Value()
	if strings.TrimSpace(d) != "" && d != f.tpl.subst {
		m := f.env.Msg.NewMR
		tok := f.confirmGen.Next()
		return app.OpenOverlay(app.NewConfirm(f.env, m.ConfirmReplaceTitle(f.tpl.names[i-1]), m.ConfirmReplaceBody,
			func() tea.Msg { return replaceMsg{tok: tok, sel: i} }))
	}
	f.tpl.sel = i
	return f.templateCmd(f.tpl.names[i-1], true)
}

// fail is the toast of a failed creation: GitLab's own text for 400, 409
// and 422.
func (f *Form) fail(err error) tea.Cmd {
	var ge *gitlab.Error
	if errors.As(err, &ge) && ge.Message != "" && (ge.Status == 400 || ge.Status == 409 || ge.Status == 422) {
		return app.Toast(common.ToastError, ge.Message, "")
	}
	return f.env.Fail(err)
}
