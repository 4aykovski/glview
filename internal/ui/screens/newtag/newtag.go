// Package newtag is the "new tag" overlay: a name chosen from the semver
// suggestions or typed, a ref with branch autocompletion, an optional
// message, and the commits the tag will include (SPEC §3.10).
package newtag

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/semver"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	debounce       = 300 * time.Millisecond
	requestTimeout = 15 * time.Second
	tagsLimit      = 100 // tags read for the suggestions and the duplicate check
	maxHints       = 5   // branch suggestions under the ref field
	maxCommits     = 10  // commits listed under "will include"
)

// CreatedMsg reports a created tag; the Tags tab reloads and puts the
// cursor on it.
type CreatedMsg struct {
	Owner common.Token // the tab that opened the form: it alone finishes the work
	Ref   gitlab.ProjectRef
	Tag   gitlab.Tag
}

type focus int

const (
	focusName focus = iota
	focusRef
	focusMessage
	focusButton
	focusCount
)

type cmpState int

const (
	cmpNone cmpState = iota // nothing to compare (no tags, empty ref)
	cmpLoading
	cmpReady
	cmpFailed
)

// Every request carries a token of one of the form's generation counters:
// answers of a closed form or of an earlier request are dropped.
type (
	tagsMsg struct {
		tok  common.Token
		tags []gitlab.Tag
		err  error
	}
	refTickMsg  struct{ tok common.Token }
	branchesMsg struct {
		tok   common.Token
		names []string
		err   error
	}
	compareMsg struct {
		tok common.Token
		ref string
		cmp gitlab.Comparison
		err error
	}
	confirmedMsg struct {
		tok  common.Token
		snap snapshot
	}
	createdMsg struct {
		tok common.Token
		tag gitlab.Tag
		err error
	}
)

// snapshot is what enter on the button took from the form: the confirm and
// the creation work with it, not with the fields.
type snapshot struct {
	name, ref, message, head string
}

// Form is the overlay.
type Form struct {
	env           *app.Env
	project       gitlab.ProjectRef
	defaultBranch string
	owner         common.Token

	name, ref, msg textinput.Model
	focus          focus

	// Tags of the project: names for the duplicate check, the newest one
	// as the base of the comparison, the suggestions.
	tagsLoaded, tagsFailed bool
	tagNames               []string
	last                   string
	sugg                   semver.Suggestion
	sel                    int // chosen option; len(options) is "custom…"
	customSeen             bool
	msgFollows             bool // the message follows the name until edited

	hints     []string
	hintIdx   int
	hintsHide bool // a hint was just used: hidden until the next edit

	cmp    cmpState
	cmpRef string // ref of the comparison shown
	cmpRes gitlab.Comparison

	errLine string
	confirm *app.Confirm
	// creating: the creation request is in flight, the form is locked.
	creating bool
	closed   bool
	compact  bool // a short terminal: no blank line under the title

	tagsGen, tickGen, hintGen, cmpGen, runGen common.Gen
}

// New creates the form for the project; the ref starts as defaultBranch.
// owner is put into CreatedMsg.
func New(env *app.Env, project gitlab.ProjectRef, defaultBranch string, owner common.Token) *Form {
	mk := func(v string) textinput.Model {
		in := common.NewInput(env.Styles)
		in.SetValue(v)
		in.CursorEnd()
		return in
	}
	f := &Form{
		env: env, project: project, defaultBranch: defaultBranch, owner: owner,
		name: mk(""), ref: mk(defaultBranch), msg: mk(""), msgFollows: true,
		tagsGen: common.NewGen(), tickGen: common.NewGen(), hintGen: common.NewGen(),
		cmpGen: common.NewGen(), runGen: common.NewGen(),
	}
	f.name.Placeholder = ""
	return f
}

// Init loads the tags; the opener runs it right after OpenOverlay.
func (f *Form) Init() tea.Cmd {
	tok := f.tagsGen.Next()
	client, p := f.env.Client, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		page, err := client.Tags().List(ctx, p, "", "", tagsLimit)
		return tagsMsg{tok: tok, tags: page.Items, err: err}
	}
}

// Name returns the tag name as it stands (for tests).
func (f *Form) Name() string { return strings.TrimSpace(f.nameValue()) }

// Ref returns the typed ref (for tests).
func (f *Form) Ref() string { return strings.TrimSpace(f.ref.Value()) }

// Message returns the message as it stands (for tests).
func (f *Form) Message() string { return strings.TrimSpace(f.msg.Value()) }

// DebounceMsg returns the pending ref debounce tick, so tests can deliver
// timers explicitly.
func (f *Form) DebounceMsg() tea.Msg { return refTickMsg{tok: f.tickGen.Current()} }

func (f *Form) loading() bool { return !f.tagsLoaded }

func (f *Form) options() []semver.Option { return f.sugg.Options }

// custom reports whether the name is typed by hand.
func (f *Form) custom() bool { return f.tagsLoaded && f.sel >= len(f.options()) }

func (f *Form) nameValue() string {
	switch {
	case f.custom():
		return f.name.Value()
	case f.tagsLoaded:
		return f.options()[f.sel].Name
	}
	return ""
}

// followMessage keeps the default message in step with the name.
func (f *Form) followMessage() {
	if !f.msgFollows {
		return
	}
	if name := strings.TrimSpace(f.nameValue()); name != "" {
		f.msg.SetValue("release: " + name)
	} else {
		f.msg.SetValue("")
	}
	f.msg.CursorEnd()
}

// ---- update ----

func (f *Form) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	if f.closed {
		return nil, nil
	}
	switch msg := msg.(type) {
	case tagsMsg:
		return f, f.onTags(msg)
	case refTickMsg:
		if !f.tickGen.Valid(msg.tok) {
			return f, nil // a newer keystroke is pending
		}
		return f, tea.Batch(f.searchBranches(), f.compare())
	case branchesMsg:
		if !f.hintGen.Valid(msg.tok) {
			return f, nil
		}
		if msg.err != nil {
			f.hints = nil
			return f, f.env.Fail(msg.err)
		}
		f.hints = msg.names[:min(len(msg.names), maxHints)]
		f.hintIdx = 0
		return f, nil
	case compareMsg:
		return f, f.onCompare(msg)
	case confirmedMsg:
		if !f.runGen.Valid(msg.tok) {
			return f, nil
		}
		return f, f.create(msg.snap)
	case createdMsg:
		return f.onCreated(msg)
	case tea.KeyPressMsg:
		return f.key(msg)
	}
	return f, nil
}

func (f *Form) onTags(msg tagsMsg) tea.Cmd {
	if !f.tagsGen.Valid(msg.tok) {
		return nil
	}
	f.tagsLoaded = true
	var cmds []tea.Cmd
	if msg.err != nil {
		f.tagsFailed = true
		cmds = append(cmds, f.env.Fail(msg.err))
	} else {
		f.tagNames = make([]string, len(msg.tags))
		for i, t := range msg.tags {
			f.tagNames[i] = t.Name
		}
		if len(f.tagNames) > 0 {
			f.last = f.tagNames[0]
		}
		f.sugg = semver.Suggest(f.tagNames)
		f.sel = f.sugg.Default
	}
	if f.custom() { // no variants: the name is typed
		f.name.SetValue("")
		f.customSeen = true
	}
	f.followMessage()
	if f.focus == focusName {
		cmds = append(cmds, f.focusCmd())
	}
	cmds = append(cmds, f.compare())
	return tea.Batch(cmds...)
}

// compare asks for the commits between the newest tag and the ref.
func (f *Form) compare() tea.Cmd {
	ref := f.Ref()
	f.cmpRes, f.cmpRef = gitlab.Comparison{}, ref
	if !f.tagsLoaded || f.last == "" || ref == "" {
		f.cmp = cmpNone
		f.cmpGen.Next() // drop an answer still in flight
		return nil
	}
	f.cmp = cmpLoading
	tok := f.cmpGen.Next()
	client, p, from := f.env.Client, f.project, f.last
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		cmp, err := client.Tags().Compare(ctx, p, from, ref)
		return compareMsg{tok: tok, ref: ref, cmp: cmp, err: err}
	}
}

func (f *Form) onCompare(msg compareMsg) tea.Cmd {
	if !f.cmpGen.Valid(msg.tok) {
		return nil
	}
	if msg.err != nil {
		f.cmp = cmpFailed
		return nil
	}
	f.cmp, f.cmpRes = cmpReady, msg.cmp
	return nil
}

func (f *Form) searchBranches() tea.Cmd {
	tok := f.hintGen.Next()
	q := f.Ref()
	client, p := f.env.Client, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		names, err := client.Projects().Branches(ctx, p, q)
		return branchesMsg{tok: tok, names: names, err: err}
	}
}

func (f *Form) key(k tea.KeyPressMsg) (app.Overlay, tea.Cmd) {
	if f.confirm != nil {
		o, cmd := f.confirm.Update(k)
		if o == nil {
			f.confirm = nil
		}
		return f, cmd
	}
	if f.creating {
		return f, nil // the request is sent and cannot be taken back
	}
	f.errLine = ""
	s := k.String()
	switch s {
	case "esc":
		f.closed = true
		return nil, nil
	case "tab":
		return f, f.setFocus((f.focus + 1) % focusCount)
	case "shift+tab":
		return f, f.setFocus((f.focus + focusCount - 1) % focusCount)
	}
	switch f.focus {
	case focusName:
		return f, f.nameKey(k)
	case focusRef:
		return f, f.refKey(k)
	case focusMessage:
		if s == "enter" {
			return f, f.setFocus(focusButton)
		}
		before := f.msg.Value()
		var cmd tea.Cmd
		f.msg, cmd = f.msg.Update(k)
		if f.msg.Value() != before {
			f.msgFollows = false
		}
		return f, cmd
	}
	if s == "enter" {
		return f, f.submit()
	}
	return f, nil
}

// focusCmd blurs or focuses the text input of the focused field.
func (f *Form) focusCmd() tea.Cmd {
	f.name.Blur()
	f.ref.Blur()
	f.msg.Blur()
	switch f.focus {
	case focusName:
		if f.custom() {
			return f.name.Focus()
		}
	case focusRef:
		return f.ref.Focus()
	case focusMessage:
		return f.msg.Focus()
	}
	return nil
}

func (f *Form) setFocus(to focus) tea.Cmd {
	f.focus = to
	return f.focusCmd()
}

func (f *Form) nameKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "up":
		f.moveVariant(-1)
		return f.focusCmd()
	case "down":
		f.moveVariant(1)
		return f.focusCmd()
	case "enter":
		return f.setFocus(focusRef)
	}
	if !f.custom() {
		return nil
	}
	before := f.name.Value()
	var cmd tea.Cmd
	f.name, cmd = f.name.Update(k)
	if f.name.Value() != before {
		f.followMessage()
	}
	return cmd
}

// moveVariant selects the previous or next variant; "custom…" is last.
func (f *Form) moveVariant(d int) {
	if !f.tagsLoaded {
		return
	}
	n := len(f.options()) + 1
	to := max(0, min(f.sel+d, n-1))
	if to == f.sel {
		return
	}
	f.sel = to
	if f.custom() && !f.customSeen {
		f.name.SetValue(f.options()[f.sugg.Default].Name)
		f.name.CursorEnd()
		f.customSeen = true
	}
	f.followMessage()
}

func (f *Form) refKey(k tea.KeyPressMsg) tea.Cmd {
	listed := f.hintsShown()
	switch k.String() {
	case "down":
		if listed {
			f.hintIdx = min(f.hintIdx+1, len(f.hints)-1)
		}
		return nil
	case "up":
		if listed {
			f.hintIdx = max(f.hintIdx-1, 0)
		}
		return nil
	case "enter":
		if listed {
			f.ref.SetValue(f.hints[f.hintIdx])
			f.ref.CursorEnd()
			f.hintsHide = true
			f.tickGen.Next() // drop a pending debounce
			return f.compare()
		}
		return f.setFocus(focusMessage)
	}
	before := f.ref.Value()
	var cmd tea.Cmd
	f.ref, cmd = f.ref.Update(k)
	if f.ref.Value() == before {
		return cmd
	}
	f.hintsHide = false
	tok := f.tickGen.Next()
	return tea.Batch(cmd, tea.Tick(debounce, func(time.Time) tea.Msg { return refTickMsg{tok: tok} }))
}

func (f *Form) hintsShown() bool {
	return f.focus == focusRef && !f.hintsHide && len(f.hints) > 0
}

// submit validates the form and asks for confirmation.
func (f *Form) submit() tea.Cmd {
	m := f.env.Msg.Tag
	name, ref := strings.TrimSpace(f.nameValue()), f.Ref()
	switch {
	case name == "":
		f.errLine = m.ErrNoName
		return f.setFocus(focusName)
	case slices.Contains(f.tagNames, name):
		f.errLine = m.ErrExists(name)
		return f.setFocus(focusName)
	case ref == "":
		f.errLine = m.ErrNoRef
		return f.setFocus(focusRef)
	}
	snap := snapshot{name: name, ref: ref, message: f.Message()}
	if f.cmp == cmpReady && f.cmpRef == ref && f.cmpRes.Head != nil {
		snap.head = f.cmpRes.Head.ShortSHA
	}
	body := m.ConfirmLightweight
	if snap.message != "" {
		body = m.ConfirmAnnotated(snap.message)
	}
	tok := f.runGen.Next()
	f.confirm = app.NewConfirm(f.env, m.ConfirmTitle(snap.name, snap.ref, snap.head), body,
		func() tea.Msg { return confirmedMsg{tok: tok, snap: snap} })
	return nil
}

func (f *Form) create(snap snapshot) tea.Cmd {
	if f.creating {
		return nil
	}
	f.creating = true
	tok := f.runGen.Next()
	client, p := f.env.Client, f.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		tag, err := client.Tags().Create(ctx, p, snap.name, snap.ref, snap.message)
		return createdMsg{tok: tok, tag: tag, err: err}
	}
}

func (f *Form) onCreated(msg createdMsg) (app.Overlay, tea.Cmd) {
	if !f.runGen.Valid(msg.tok) || !f.creating {
		return f, nil
	}
	f.creating = false
	if msg.err != nil {
		return f, f.fail(msg.err)
	}
	f.closed = true
	p, tag, owner := f.project, msg.tag, f.owner
	return nil, tea.Batch(
		app.Toast(common.ToastSuccess, f.env.Msg.Tag.Created(tag.Name), ""),
		func() tea.Msg { return CreatedMsg{Owner: owner, Ref: p, Tag: tag} },
	)
}

// fail shows the error of the creation; a 400 carries GitLab's own text.
func (f *Form) fail(err error) tea.Cmd {
	var ge *gitlab.Error
	if errors.As(err, &ge) && ge.Status == 400 && ge.Message != "" {
		return app.Toast(common.ToastError, ge.Message, "")
	}
	return f.env.Fail(err)
}

// ---- view ----

// View renders the form. The commit list takes the rows the rest leaves.
func (f *Form) View(width, height int) string {
	if f.confirm != nil {
		return f.confirm.View(width, height)
	}
	s := f.env.Styles
	w := min(76, width-6)
	f.compact = height < 30
	head := f.headView(w)
	ref := f.refView(w)
	msg := f.messageView(w)
	foot := f.errView(w) + f.bottomView(w)
	rest := s.Modal.GetVerticalFrameSize() + lines(head) + lines(ref) + lines(msg) + lines(foot)
	avail := max(0, height-2-rest) // rows for the commit list with its caption
	var parts []string
	for _, p := range []string{head, ref, msg, f.commitsView(w, avail), foot} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return s.Modal.Width(w + 4).Render(strings.Join(parts, "\n"))
}

func lines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func (f *Form) field(label string, which focus, inner string, w int) string {
	s := f.env.Styles
	kind := common.BoxNormal
	if f.focus == which {
		label, kind = s.Accent.Render(label), common.BoxActive
	}
	return label + "\n" + common.Box(s, " "+inner, w, 3, kind)
}

// headView is the title and the name field with the variants.
func (f *Form) headView(w int) string {
	s, m := f.env.Styles, f.env.Msg.Tag
	title := render.Clip(s.ModalTitle.Render(m.FormTitle)+s.Muted.Render(" · "+f.project.FullPath), w)
	var name string
	switch {
	case f.custom():
		name = f.name.View()
	default:
		name = s.Text.Render(f.nameValue())
	}
	gap := "\n\n"
	if f.compact {
		gap = "\n"
	}
	out := title + gap + f.field(m.LblName, focusName, name, w)
	if f.loading() {
		return out + "\n" + s.Muted.Render(m.LoadingTags)
	}
	if len(f.options()) == 0 {
		return out + "\n" + render.Clip(f.variantsLine(), w)
	}
	return out + "\n" + render.Clip(s.Muted.Render(f.suggestedLine()), w) + "\n" + render.Clip(f.variantsLine(), w)
}

// suggestedLine is "suggested: patch from v1.14.2".
func (f *Form) suggestedLine() string {
	m := f.env.Msg.Tag
	def := f.options()[f.sugg.Default]
	if f.sugg.Base == "" {
		return m.SuggestedNoBase(def.Name)
	}
	return m.SuggestedFrom(label(def), f.sugg.Base)
}

// label is the variant's kind, "minor · release" for a release match.
func label(o semver.Option) string {
	if o.Release {
		return string(o.Kind) + " · release"
	}
	return string(o.Kind)
}

// variantsLine is "patch v1.14.3 · minor v1.15.0 · major v2.0.0 · custom…",
// the chosen variant in the accent color.
func (f *Form) variantsLine() string {
	s, m := f.env.Styles, f.env.Msg.Tag
	var parts []string
	for i, o := range f.options() {
		text := string(o.Kind) + " " + o.Name
		if o.Release {
			text += " · release"
		}
		parts = append(parts, f.mark(text, i == f.sel))
	}
	parts = append(parts, f.mark(m.Custom, f.custom()))
	return strings.Join(parts, s.Muted.Render(" · "))
}

func (f *Form) mark(text string, on bool) string {
	if on {
		return f.env.Styles.Accent.Bold(true).Render(text)
	}
	return f.env.Styles.Muted.Render(text)
}

// refView is the ref field, the branch suggestions and the comparison
// caption.
func (f *Form) refView(w int) string {
	s, m := f.env.Styles, f.env.Msg.Tag
	out := f.field(m.LblRef, focusRef, f.ref.View(), w)
	if f.hintsShown() {
		for i, h := range f.hints {
			mark := "  "
			if i == f.hintIdx {
				mark = s.Accent.Render("› ")
			}
			out += "\n" + render.Clip(mark+h, w)
		}
	}
	return out + "\n" + render.Clip(f.captionLine(), w)
}

// captionLine is "main @ 0b7d3e9 · 2 commits after v1.14.2".
func (f *Form) captionLine() string {
	s, m := f.env.Styles, f.env.Msg.Tag
	ref := f.Ref()
	switch {
	case ref == "" || f.loading() || f.tagsFailed:
		return " "
	case f.last == "":
		return s.Muted.Render(m.RefFirstTag(ref))
	}
	switch f.cmp {
	case cmpLoading:
		return s.Muted.Render(m.RefComparing)
	case cmpFailed:
		return s.Muted.Render(m.RefCompareErr)
	case cmpReady:
		sha := ""
		if h := f.cmpRes.Head; h != nil {
			sha = h.ShortSHA
		}
		return s.Muted.Render(m.RefCaption(f.cmpRef, sha, len(f.cmpRes.Commits), f.last))
	}
	return " "
}

func (f *Form) messageView(w int) string {
	return f.field(f.env.Msg.Tag.LblMessage, focusMessage, f.msg.View(), w)
}

// commitsView lists up to ten commits of the comparison, newest first, in
// at most avail rows (the caption and the "more" line included).
func (f *Form) commitsView(w, avail int) string {
	s, m := f.env.Styles, f.env.Msg.Tag
	if f.cmp != cmpReady || f.cmpRef != f.Ref() || avail < 1 {
		return ""
	}
	cs := f.cmpRes.Commits
	if len(cs) == 0 {
		return s.Muted.Render(m.NoNewCommits(f.last))
	}
	room := avail - 1 // the caption takes a row
	if room < 1 {
		return ""
	}
	rows := min(len(cs), maxCommits, room)
	if more := len(cs) - rows; more > 0 && rows+1 > room { // "more" needs a row too
		rows = room - 1 // none when only one row is left: the "more" line says it all
	}
	out := []string{s.Muted.Render(m.LblCommits)}
	for _, c := range cs[:rows] {
		out = append(out, render.Clip(s.Accent.Render(c.ShortSHA)+" "+c.Title, w))
	}
	if more := len(cs) - rows; more > 0 {
		out = append(out, s.Muted.Render(m.CommitsMore(more)))
	}
	return strings.Join(out, "\n")
}

func (f *Form) errView(w int) string {
	if f.errLine == "" {
		return ""
	}
	return render.Clip(f.env.Styles.Error.Render(f.errLine), w) + "\n"
}

// keysLine is the key hint for the current focus.
func (f *Form) keysLine() string {
	m := f.env.Msg.Tag
	switch {
	case f.creating:
		return m.KeysCreating
	case f.focus == focusName && f.custom():
		return m.KeysCustom
	case f.focus == focusName:
		return m.KeysName
	case f.focus == focusRef && f.hintsShown():
		return m.KeysRefList
	case f.focus == focusRef:
		return m.KeysRef
	case f.focus == focusMessage:
		return m.KeysMessage
	}
	return m.KeysButton
}

// bottomView puts the key line (wrapped between its items) left of the
// create button.
func (f *Form) bottomView(w int) string {
	s, m := f.env.Styles, f.env.Msg.Tag
	button := s.Button.Render("enter " + m.ButtonCreate)
	if f.focus == focusButton && !f.creating {
		button = s.ButtonActive.Render("enter " + m.ButtonCreate)
	}
	keysW := max(10, w-lipgloss.Width(button)-2)
	var out []string
	line := ""
	for _, item := range strings.Split(f.keysLine(), " · ") {
		switch {
		case line == "":
			line = item
		case ansi.StringWidth(line+" · "+item) <= keysW:
			line += " · " + item
		default:
			out = append(out, line)
			line = item
		}
	}
	out = append(out, line)
	for i := range out {
		out[i] = render.Pad(s.Muted.Render(out[i]), keysW)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(out, "\n"), "  ", button)
}
