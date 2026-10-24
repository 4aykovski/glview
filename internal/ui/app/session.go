package app

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
)

// PollInterval is the delay before the next poll tick: PollActive while
// something on screen is still changing, PollIdle otherwise, slowed down
// while the terminal is not focused.
func (e *Env) PollInterval(active bool) time.Duration {
	base := e.Settings.PollIdle
	if active {
		base = e.Settings.PollActive
	}
	if base <= 0 {
		base = DefaultPollIdle
		if active {
			base = DefaultPollActive
		}
	}
	return common.PollInterval(e.Net.Backoff(base), e.Focused)
}

// WorkspacePoll is the workspace poll interval (polling.workspace), slowed
// down while the terminal is not focused.
func (e *Env) WorkspacePoll() time.Duration {
	base := e.Settings.PollWorkspace
	if base <= 0 {
		base = DefaultPollWorkspace
	}
	return common.PollInterval(e.Net.Backoff(base), e.Focused)
}

// ErrorText turns a gitlab error into a translated message.
func ErrorText(env *Env, err error) string {
	t := env.Msg.Errors
	switch {
	case errors.Is(err, gitlab.ErrUnauthorized):
		return t.Unauthorized
	case errors.Is(err, gitlab.ErrInsufficientScope):
		return t.InsufficientScope
	case errors.Is(err, gitlab.ErrForbidden):
		return t.Forbidden
	case errors.Is(err, gitlab.ErrNotFound):
		return t.NotFound
	case errors.Is(err, gitlab.ErrConflict):
		return t.Conflict
	case errors.Is(err, gitlab.ErrNotPossible):
		return t.NotPossible
	case errors.Is(err, gitlab.ErrRateLimited):
		return t.RateLimited(retryAfterSeconds(err))
	case errors.Is(err, gitlab.ErrTLS):
		return t.TLS
	case errors.Is(err, gitlab.ErrNetwork):
		return t.Network
	case errors.Is(err, gitlab.ErrServer):
		return t.Server
	}
	return t.Unknown
}

// defaultRetryAfter is shown for a 429 without a usable Retry-After.
const defaultRetryAfter = 30

// retryAfterSeconds is the "retry in N s" of a rate-limit error: the
// server's Retry-After, rounded up, else defaultRetryAfter.
func retryAfterSeconds(err error) int {
	var ge *gitlab.Error
	if errors.As(err, &ge) && ge.RetryAfter > 0 {
		return int((ge.RetryAfter + time.Second - 1) / time.Second)
	}
	return defaultRetryAfter
}

// Fail reports a failed request: a 401 becomes AuthLostMsg (the root shows
// the hint and stops polling), cancellations are ignored, everything else
// is an error toast.
func (e *Env) Fail(err error) tea.Cmd {
	switch {
	case err == nil, errors.Is(err, context.Canceled):
		return nil
	case errors.Is(err, gitlab.ErrUnauthorized):
		return msgCmd(AuthLostMsg{Err: err})
	}
	hint := ""
	if errors.Is(err, gitlab.ErrTLS) {
		hint = e.Msg.Errors.HintTLS
	}
	return Toast(common.ToastError, ErrorText(e, err), hint)
}

// authHint is the "where to fix the token" hint for the token's source.
func (e *Env) authHint() string {
	t := e.Msg.Errors
	switch e.TokenSource {
	case config.SourceGlab:
		return t.HintGlabToken(config.Hostname(e.Host))
	case config.SourceEnv:
		return t.HintEnvToken
	}
	return t.HintConfigToken
}

func (m *Model) onAuthLost(msg AuthLostMsg) tea.Cmd {
	first := !m.env.AuthLost
	m.env.AuthLost = true
	cmds := []tea.Cmd{m.broadcast(msg)}
	if first {
		cmds = append(cmds, Toast(common.ToastError, m.env.Msg.Errors.Unauthorized, m.env.authHint()))
	}
	return tea.Batch(cmds...)
}

// checkAuth re-validates the token (GET /user) after 401, on "r".
func (m *Model) checkAuth() tea.Cmd {
	client := m.env.Client
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		u, err := client.Users().Current(ctx)
		return authCheckedMsg{user: u, err: err}
	}
}

func (m *Model) onAuthChecked(msg authCheckedMsg) tea.Cmd {
	if msg.err != nil {
		return Toast(common.ToastError, ErrorText(m.env, msg.err), m.env.authHint())
	}
	m.env.AuthLost = false
	m.env.User = msg.user
	return tea.Batch(
		Toast(common.ToastSuccess, m.env.Msg.Toast.AuthRestored, ""),
		m.broadcast(AuthRestoredMsg{}),
	)
}

// listenNet waits for the next change of the host's reachability.
func (m *Model) listenNet() tea.Cmd {
	ch := m.env.Net.Changes()
	if ch == nil {
		return nil
	}
	return func() tea.Msg { return NetChangedMsg{Online: <-ch} }
}

// onNetChanged re-listens and announces the recovery (an outage is already
// visible: the failed requests toast and the header gets "(offline)").
func (m *Model) onNetChanged(msg NetChangedMsg) tea.Cmd {
	if !msg.Online {
		return m.listenNet()
	}
	return tea.Batch(m.listenNet(), Toast(common.ToastSuccess, m.env.Msg.Toast.Online, ""))
}

// listenCache waits for the next background cache refresh.
func (m *Model) listenCache() tea.Cmd {
	if m.env.Cache == nil {
		return nil
	}
	ch := m.env.Cache.Revalidated()
	return func() tea.Msg { return CacheRevalidatedMsg{Key: <-ch} }
}

// OpenURL opens url in the system browser. A failure is shown as a toast
// with the URL in the hint, so it can still be copied by hand.
func OpenURL(env *Env, url string) tea.Cmd {
	return func() tea.Msg {
		open := env.Browser
		if open == nil {
			return nil
		}
		if err := open(url); err != nil {
			return ToastMsg{Level: common.ToastError, Text: env.Msg.Toast.BrowserFailed, Hint: url}
		}
		return nil
	}
}

// ProjectURL is the project page in the web UI without a trailing slash:
// webURL when known, else the instance URL and the full path (the project
// is not loaded yet). Both modes build links to a project with it.
func (e *Env) ProjectURL(webURL, path string) string {
	base := webURL
	if base == "" {
		base = strings.TrimRight(e.Host, "/") + "/" + path
	}
	return strings.TrimRight(base, "/")
}

// Copy puts text on the terminal clipboard (OSC 52) and confirms with a
// toast naming what was copied.
func Copy(env *Env, text, what string) tea.Cmd {
	return tea.Batch(tea.SetClipboard(text), Toast(common.ToastInfo, env.Msg.Toast.Copied(what), ""))
}

// HeaderRight renders the right side of a screen header: "@user · ⟳ 12s",
// followed by the "✗ auth" mark while the token is rejected. updated is the
// time of the last successful load (zero hides the age).
func (e *Env) HeaderRight(updated time.Time) string {
	s := "@" + e.User.Username
	if p := e.Git.Header(e.Msg); p != "" {
		s = e.Styles.Accent.Render(p) + " · " + s
	}
	age := ""
	if !updated.IsZero() {
		age = e.Msg.Header.Updated(i18n.RelTimeShort(e.Lang, e.Now().Sub(updated)))
	}
	if e.Net.Offline() {
		age = strings.TrimSpace(age + " " + e.Styles.Warning.Render("("+e.Msg.Header.Offline+")"))
	}
	if age != "" {
		s += " · " + age
	}
	if e.AuthLost {
		s += " · " + e.Styles.Error.Render(e.Msg.Header.AuthError)
	}
	return s
}
