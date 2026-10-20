// Package setup is the first-run screen: instance URL and token, verified
// with GET /user, then saved to the config.
package setup

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

// Options wire the screen to the outside world.
type Options struct {
	Host string // prefilled URL (from remote or config)
	Err  error  // error that brought us here (e.g. 401 at start)

	// GlabToken returns glab's token for a hostname; nil when glab is unavailable.
	GlabToken func(ctx context.Context, hostname string) (string, error)
	// Connect builds a client and verifies the token (GET /user).
	Connect func(ctx context.Context, hostURL, token string) (gitlab.Client, gitlab.User, error)
	// Save persists host and token (token is empty when it came from glab).
	Save func(hostURL, token string) error
	// Normalize validates and canonicalises the URL.
	Normalize func(raw string) (string, error)
	// Next builds the screen to open once connected.
	Next func(env *app.Env) app.Screen
}

const (
	sourceManual = "manual"
	sourceGlab   = "glab"
)

type connectedMsg struct {
	client gitlab.Client
	user   gitlab.User
	host   string
	token  string
	glab   bool
	err    error
}

// Model is the setup screen.
type Model struct {
	env  *app.Env
	opts Options

	form    *huh.Form
	url     string
	token   string
	source  string
	busy    bool
	done    bool // connected; the screen is about to be replaced
	errText string
	w, h    int
}

// New creates the setup screen.
func New(env *app.Env, opts Options) *Model {
	m := &Model{env: env, opts: opts, url: opts.Host, source: sourceManual}
	if m.url == "" {
		m.url = "https://gitlab.com"
	}
	if opts.Err != nil {
		m.errText = describe(env, opts.Err)
	}
	m.buildForm()
	return m
}

func (m *Model) buildForm() {
	t := m.env.Msg.Setup
	groups := []*huh.Group{
		huh.NewGroup(
			huh.NewInput().Key("url").Title(t.URL).Placeholder(t.URLHint).Value(&m.url).
				Validate(func(s string) error {
					if m.opts.Normalize == nil {
						return nil
					}
					_, err := m.opts.Normalize(s)
					return err
				}),
		),
	}
	if m.opts.GlabToken != nil {
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[string]().Key("source").Title(t.Token).
				Options(
					huh.NewOption(t.Token, sourceManual),
					huh.NewOption(t.UseGlab(config.Hostname(m.url)), sourceGlab),
				).Value(&m.source),
		))
	}
	groups = append(groups, huh.NewGroup(
		huh.NewInput().Key("token").Title(t.Token).Description(t.TokenHint).
			EchoMode(huh.EchoModePassword).Value(&m.token).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New(t.BadToken)
				}
				return nil
			}),
	).WithHideFunc(func() bool { return m.source == sourceGlab }))

	keymap := huh.NewDefaultKeyMap()
	keymap.Quit.SetEnabled(false) // ctrl+c is handled by the root
	m.form = huh.NewForm(groups...).
		WithTheme(huh.ThemeFunc(huh.ThemeBase16)).
		WithKeyMap(keymap).
		WithShowHelp(true).
		WithWidth(min(72, max(40, m.w-4)))
}

func (m *Model) Init() tea.Cmd { return m.form.Init() }

func (m *Model) Title() string { return m.env.Msg.Setup.Title }

func (m *Model) SetSize(w, h int) {
	m.w, m.h = w, h
	m.form.WithWidth(min(72, max(40, w-4)))
}

// CapturesInput: the form owns the keyboard (q, esc, ? are typed into fields).
func (m *Model) CapturesInput() bool { return !m.busy && !m.done }

func (m *Model) KeyMap() common.KeyMap { return common.KeyMap{} }

func (m *Model) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case app.LangChangedMsg:
		m.buildForm()
		return m, m.form.Init()
	case connectedMsg:
		m.busy = false
		if msg.err != nil {
			m.errText = describe(m.env, msg.err)
			m.token = ""
			m.buildForm()
			return m, m.form.Init()
		}
		m.done = true
		m.env.Client, m.env.User, m.env.Host = msg.client, msg.user, msg.host
		var cmds []tea.Cmd
		if m.opts.Save != nil {
			saveToken := msg.token
			if msg.glab {
				saveToken = "" // a glab token is never copied into our config
			}
			if err := m.opts.Save(msg.host, saveToken); err != nil {
				cmds = append(cmds, app.Toast(common.ToastError, m.env.Msg.Toast.ConfigSaveFailed(err.Error()), ""))
			}
		}
		cmds = append(cmds, app.Toast(common.ToastSuccess, m.env.Msg.Setup.LoggedInAs(msg.user.Username), ""))
		if m.opts.Next != nil {
			cmds = append(cmds, app.ReplaceRoot(m.opts.Next(m.env)))
		}
		return m, tea.Batch(cmds...)
	}

	if m.busy || m.done {
		return m, nil
	}
	f, cmd := m.form.Update(msg)
	if form, ok := f.(*huh.Form); ok {
		m.form = form
	}
	if m.form.State == huh.StateCompleted {
		m.busy = true
		m.errText = ""
		return m, tea.Batch(cmd, m.connect())
	}
	return m, cmd
}

func (m *Model) connect() tea.Cmd {
	rawURL, token, useGlab := m.url, strings.TrimSpace(m.token), m.source == sourceGlab
	opts := m.opts
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		host := rawURL
		if opts.Normalize != nil {
			h, err := opts.Normalize(rawURL)
			if err != nil {
				return connectedMsg{err: err}
			}
			host = h
		}
		if useGlab {
			t, err := opts.GlabToken(ctx, config.Hostname(host))
			if err != nil || t == "" {
				return connectedMsg{err: &gitlab.Error{Kind: gitlab.ErrUnauthorized}}
			}
			token = t
		}
		client, user, err := opts.Connect(ctx, host, token)
		return connectedMsg{client: client, user: user, host: host, token: token, glab: useGlab, err: err}
	}
}

func (m *Model) View() string {
	s := m.env.Styles
	t := m.env.Msg.Setup
	var b strings.Builder
	b.WriteString(common.Header(s, s.Bold.Render(t.Title), "", m.w) + "\n\n")
	b.WriteString("  " + s.Muted.Render(t.Intro) + "\n\n")
	for _, line := range strings.Split(m.form.View(), "\n") {
		b.WriteString("  " + line + "\n")
	}
	switch {
	case m.busy:
		b.WriteString("\n  " + s.StatusStyle(gitlab.StatusRunning).Render("◐ "+t.Checking))
	case m.errText != "":
		b.WriteString("\n  " + s.Error.Render("✗ "+m.errText))
	}
	return b.String()
}

func describe(env *app.Env, err error) string {
	t := env.Msg.Setup
	switch {
	case errors.Is(err, gitlab.ErrUnauthorized), errors.Is(err, gitlab.ErrForbidden):
		return t.BadToken
	case errors.Is(err, gitlab.ErrTLS):
		return t.TLSFailed
	case errors.Is(err, gitlab.ErrNetwork), errors.Is(err, context.DeadlineExceeded):
		return t.Unreachable
	}
	return err.Error()
}
