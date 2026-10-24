// Package comment is the comment form: an overlay with a multi-line field
// for a note on an issue or a merge request (SPEC §3.16). The text of a form
// closed with esc is kept as a draft in memory for the session.
package comment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/issueactions"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	requestTimeout = 15 * time.Second
	minRows        = 6  // the field is never shorter
	frameRows      = 14 // rows of the modal around the field
)

// Kinds of a Target.
const (
	KindIssue = "issue"
	KindMR    = "mr"
)

// Target is the object a comment is written to.
type Target struct {
	Project gitlab.ProjectRef
	Kind    string // KindIssue | KindMR
	IID     int64
	Title   string
}

// Ref is how the object is named: "#1318" for an issue, "!479" for an MR.
func (t Target) Ref() string {
	if t.Kind == KindMR {
		return fmt.Sprintf("!%d", t.IID)
	}
	return fmt.Sprintf("#%d", t.IID)
}

// SentMsg reports a comment that GitLab accepted; the screen showing the
// object reloads its comments.
type SentMsg struct {
	Target Target
	Note   gitlab.Note
}

// sentMsg is the answer of the request of one form.
type sentMsg struct {
	tok  common.Token
	note gitlab.Note
	err  error
}

// Form is the overlay.
type Form struct {
	env      *app.Env
	t        Target
	draftKey string // key of the draft

	ta     textarea.Model
	button bool // the focus is on the button
	// sending: the request is in flight, the form is locked.
	sending bool
	closed  bool
	errLine string

	runGen common.Gen
}

// New creates the form for the target, with the stored draft in the field.
func New(env *app.Env, t Target) *Form {
	f := &Form{env: env, t: t, draftKey: env.DraftKey(t.Project.FullPath, t.Kind, t.IID), runGen: common.NewGen()}
	f.ta = common.NewTextArea(env.Styles)
	f.ta.DynamicHeight = true
	f.ta.MinHeight = minRows
	f.ta.SetValue(env.Draft(f.draftKey))
	f.ta.CursorEnd()
	return f
}

// Open shows the form for t.
func Open(env *app.Env, t Target) tea.Cmd {
	f := New(env, t)
	return tea.Sequence(app.OpenOverlay(f), f.Init())
}

// Init focuses the field; the opener runs it right after OpenOverlay.
func (f *Form) Init() tea.Cmd { return f.ta.Focus() }

// Text returns the text of the field (for tests).
func (f *Form) Text() string { return f.ta.Value() }

// Target returns the target of the form (for tests).
func (f *Form) Target() Target { return f.t }

// Answer returns the message the request of the form is answered with, so
// tests can deliver it explicitly (to this form or to another one).
func (f *Form) Answer(note gitlab.Note, err error) tea.Msg {
	return sentMsg{tok: f.runGen.Current(), note: note, err: err}
}

func (f *Form) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	switch msg := msg.(type) {
	case sentMsg:
		// Answers of other forms are not for this one; the answer of this
		// form is handled even when the form is closed: the comment exists.
		if !f.runGen.Valid(msg.tok) || !f.sending {
			return f, nil
		}
		return f.onSent(msg)
	case tea.KeyPressMsg:
		if f.closed {
			return nil, nil
		}
		return f.key(msg)
	}
	if f.closed || f.sending {
		return f, nil
	}
	var cmd tea.Cmd
	f.ta, cmd = f.ta.Update(msg) // paste, cursor blink
	return f, cmd
}

func (f *Form) key(k tea.KeyPressMsg) (app.Overlay, tea.Cmd) {
	if f.sending {
		return f, nil // the request is sent and cannot be taken back
	}
	switch s := k.String(); s {
	case "esc":
		f.env.SetDraft(f.draftKey, f.ta.Value())
		f.closed = true
		return nil, nil
	case "tab", "shift+tab":
		f.errLine = ""
		f.button = !f.button
		if f.button {
			f.ta.Blur()
			return f, nil
		}
		return f, f.ta.Focus()
	case "enter":
		if f.button {
			return f, f.submit()
		}
	case "alt+enter":
		return f, f.submit()
	}
	if f.button {
		return f, nil
	}
	before := f.ta.Value()
	var cmd tea.Cmd
	f.ta, cmd = f.ta.Update(k)
	if f.ta.Value() != before {
		f.errLine = ""
	}
	return f, cmd
}

// submit sends the text as it stands now.
func (f *Form) submit() tea.Cmd {
	body := strings.TrimRight(strings.TrimLeft(f.ta.Value(), "\r\n"), " \t\r\n")
	if strings.TrimSpace(body) == "" {
		f.errLine = f.env.Msg.Comment.Empty
		return nil
	}
	f.errLine = ""
	f.sending = true
	f.ta.Blur()
	tok := f.runGen.Next()
	client, t := f.env.Client, f.t
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		var (
			note gitlab.Note
			err  error
		)
		if t.Kind == KindMR {
			note, err = client.MergeRequests().Comment(ctx, t.Project, t.IID, body)
		} else {
			note, err = client.Issues().Comment(ctx, t.Project, t.IID, body)
		}
		return sentMsg{tok: tok, note: note, err: err}
	}
}

func (f *Form) onSent(msg sentMsg) (app.Overlay, tea.Cmd) {
	f.sending = false
	if msg.err != nil {
		var focus tea.Cmd
		if !f.button {
			focus = f.ta.Focus()
		}
		return f, tea.Batch(f.env.Fail(msg.err), focus)
	}
	f.env.SetDraft(f.draftKey, "")
	Invalidate(f.env, f.t)
	f.closed = true
	t, note := f.t, msg.note
	return nil, tea.Batch(
		app.Toast(common.ToastSuccess, f.env.Msg.Comment.Added, ""),
		func() tea.Msg { return SentMsg{Target: t, Note: note} },
	)
}

// Invalidate drops the cached comments of the target: GitLab has a new
// note, the screens reload them.
func Invalidate(env *app.Env, t Target) {
	if t.Kind == KindMR {
		if env.Cache != nil {
			env.Cache.Invalidate(fmt.Sprintf("mrdisc:%s:%d", t.Project.FullPath, t.IID))
			env.Cache.Invalidate(fmt.Sprintf("mr:%s:%d", t.Project.FullPath, t.IID))
		}
		return
	}
	issueactions.Invalidate(env, t.Project, t.IID)
}

// ---- view ----

func (f *Form) View(width, height int) string {
	s, m := f.env.Styles, f.env.Msg.Comment
	w := min(76, width-6)
	title := render.Clip(s.ModalTitle.Render(m.Title(f.t.Ref(), f.t.Title)), w)

	maxRows := max(minRows, min(height/2, height-frameRows))
	f.ta.MaxHeight = maxRows
	f.ta.SetWidth(w - 4)
	rows := min(max(f.ta.Height(), minRows), maxRows)
	f.ta.SetHeight(rows)
	kind := common.BoxNormal
	if !f.button && !f.sending {
		kind = common.BoxActive
	}
	field := common.Box(s, " "+strings.ReplaceAll(f.ta.View(), "\n", "\n "), w, rows+2, kind)

	hint := s.Muted.Render(m.Hint)
	switch {
	case f.sending:
		hint = s.Muted.Render(m.Sending)
	case f.errLine != "":
		hint = s.Error.Render(f.errLine)
	}

	button := s.Button.Render("enter " + m.Send)
	if f.button && !f.sending {
		button = s.ButtonActive.Render("enter " + m.Send)
	}
	left := s.Muted.Render(m.Close)
	gap := max(1, w-lipgloss.Width(left)-lipgloss.Width(button))
	bottom := left + strings.Repeat(" ", gap) + button

	return s.Modal.Width(w + 4).Render(strings.Join([]string{title, "", field, render.Clip(hint, w), bottom}, "\n"))
}
