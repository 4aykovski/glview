package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/cache"
	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

// probe records every message type it receives.
type probe struct {
	env  *Env
	seen []string
}

func (p *probe) Init() tea.Cmd         { return nil }
func (p *probe) Title() string         { return "probe" }
func (p *probe) SetSize(int, int)      {}
func (p *probe) KeyMap() common.KeyMap { return common.KeyMap{} }
func (p *probe) View() string          { return strings.Join(p.seen, ",") }
func (p *probe) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
	case tea.KeyPressMsg:
		p.seen = append(p.seen, "key:"+m.String())
	default:
		p.seen = append(p.seen, fmt.Sprintf("%T", msg))
	}
	return p, nil
}

func (p *probe) saw(prefix string) int {
	n := 0
	for _, s := range p.seen {
		if strings.Contains(s, prefix) {
			n++
		}
	}
	return n
}

func newProbeRoot(t *testing.T) (*uitest.Driver, *Model, *probe, *fake.Client) {
	t.Helper()
	client := fake.New()
	env := NewEnv(i18n.EN)
	env.Client, env.User, env.Host = client, client.User, "https://gitlab.example.com"
	p := &probe{env: env}
	root := New(env, p, Options{})
	return uitest.New(t, root), root, p, client
}

func TestCacheRevalidatedIsBroadcastAndRelistened(t *testing.T) {
	_, root, p, _ := newProbeRoot(t)
	root.env.Cache = cache.New()

	if cmd := root.listenCache(); cmd == nil {
		t.Fatal("a cache in the env must be listened to")
	}
	_, cmd := root.Update(CacheRevalidatedMsg{Key: "mrs:a/b:x"})
	if p.saw("CacheRevalidatedMsg") != 1 {
		t.Fatalf("screen did not get the message: %v", p.seen)
	}
	if cmd == nil {
		t.Fatal("the root must listen for the next refresh")
	}
}

func TestListenCacheDeliversKey(t *testing.T) {
	env := NewEnv(i18n.EN)
	env.Cache = cache.New()
	m := New(env, &probe{env: env}, Options{})
	fetch := func(context.Context) (int, error) { return 1, nil }

	if _, _, err := cache.Get(t.Context(), env.Cache, "k", time.Millisecond, fetch); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	// The entry is stale now: the call returns it and refreshes in the background.
	if _, stale, _ := cache.Get(t.Context(), env.Cache, "k", time.Millisecond, fetch); !stale {
		t.Fatal("expected a stale hit")
	}
	got := make(chan tea.Msg, 1)
	go func() { got <- m.listenCache()() }()
	select {
	case msg := <-got:
		if r, ok := msg.(CacheRevalidatedMsg); !ok || r.Key != "k" {
			t.Fatalf("msg = %#v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no revalidation announced")
	}
}

func TestAuthLostAndRestored(t *testing.T) {
	d, root, p, client := newProbeRoot(t)
	root.env.TokenSource = config.SourceGlab

	d.Send(AuthLostMsg{Err: &gitlab.Error{Kind: gitlab.ErrUnauthorized, Status: 401}})
	if !root.env.AuthLost || p.saw("AuthLostMsg") != 1 {
		t.Fatalf("AuthLost = %v, seen = %v", root.env.AuthLost, p.seen)
	}
	d.RequireView("") // root renders
	d.RequireView("Token rejected", "glab auth login --hostname gitlab.example.com")

	// A second 401 does not repeat the toast.
	d.Send(AuthLostMsg{})
	if p.saw("AuthLostMsg") != 2 {
		t.Fatalf("screens must still be told: %v", p.seen)
	}

	// r re-checks the token and the screen does not get the key.
	client.ResetCalls()
	client.FailNext("Users.Current", &gitlab.Error{Kind: gitlab.ErrUnauthorized, Status: 401})
	d.Key("r")
	if client.CallCount("Users.Current") != 1 || !root.env.AuthLost {
		t.Fatalf("failed re-check must keep the mark: calls=%d lost=%v", client.CallCount("Users.Current"), root.env.AuthLost)
	}
	d.Key("r")
	if client.CallCount("Users.Current") != 2 || root.env.AuthLost {
		t.Fatalf("successful re-check must clear the mark: calls=%d lost=%v", client.CallCount("Users.Current"), root.env.AuthLost)
	}
	if p.saw("AuthRestoredMsg") != 1 {
		t.Fatalf("screens must reload: %v", p.seen)
	}
	if p.saw("key:r") != 0 {
		t.Fatalf("r belongs to the root while auth is lost: %v", p.seen)
	}
	d.RequireView("Token accepted")
	d.Key("r")
	if p.saw("key:r") != 1 {
		t.Fatal("after recovery r goes to the screen again")
	}
}

func TestAuthHintBySource(t *testing.T) {
	for src, want := range map[config.Source]string{
		config.SourceConfig: "update token in config.yaml",
		config.SourceEnv:    "check GITLAB_TOKEN",
		config.SourceGlab:   "glab auth login --hostname gitlab.example.com",
	} {
		d, root, _, _ := newProbeRoot(t)
		root.env.TokenSource = src
		d.Send(AuthLostMsg{})
		d.RequireView(want)
	}
}

func TestFailMapsUnauthorized(t *testing.T) {
	env := NewEnv(i18n.EN)
	if cmd := env.Fail(&gitlab.Error{Kind: gitlab.ErrUnauthorized}); cmd == nil {
		t.Fatal("401 must produce a message")
	} else if _, ok := cmd().(AuthLostMsg); !ok {
		t.Fatalf("401 -> %T", cmd())
	}
	if m, ok := env.Fail(&gitlab.Error{Kind: gitlab.ErrNotFound})().(ToastMsg); !ok || m.Text != "Not found or no access" {
		t.Fatalf("toast = %+v", m)
	}
	if env.Fail(nil) != nil || env.Fail(errors.New("x")) == nil {
		t.Fatal("nil error is a no-op, unknown error is a toast")
	}
}

func TestFocusSlowsPolling(t *testing.T) {
	d, root, p, _ := newProbeRoot(t)
	env := root.env
	env.Settings = Settings{PollActive: 3 * time.Second, PollIdle: 30 * time.Second}
	if env.PollInterval(true) != 3*time.Second || env.PollInterval(false) != 30*time.Second {
		t.Fatal("focused intervals")
	}
	d.Send(tea.BlurMsg{})
	if env.Focused || env.PollInterval(true) != 15*time.Second || p.saw("FocusChangedMsg") != 1 {
		t.Fatalf("blurred: focused=%v interval=%v seen=%v", env.Focused, env.PollInterval(true), p.seen)
	}
	d.Send(tea.FocusMsg{})
	if !env.Focused || env.PollInterval(true) != 3*time.Second || p.saw("FocusChangedMsg") != 2 {
		t.Fatalf("focused again: %v", p.seen)
	}
}

func TestHeaderRight(t *testing.T) {
	env := NewEnv(i18n.EN)
	env.User = gitlab.User{Username: "egor"}
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	if got := env.HeaderRight(now.Add(-14 * time.Second)); got != "@egor · ⟳ <1m" {
		t.Fatalf("got %q", got)
	}
	env.AuthLost = true
	if got := env.HeaderRight(time.Time{}); !strings.Contains(got, "@egor") || !strings.Contains(got, "✗ auth") {
		t.Fatalf("got %q", got)
	}
}

type statusProbe struct{ probe }

func (s *statusProbe) StatusHint() string { return "poll 3s while running" }

func (s *statusProbe) Update(msg tea.Msg) (Screen, tea.Cmd) {
	_, cmd := s.probe.Update(msg)
	return s, cmd
}

func TestFooterShowsStatusUntilToast(t *testing.T) {
	client := fake.New()
	env := NewEnv(i18n.EN)
	env.Client = client
	root := New(env, &statusProbe{probe{env: env}}, Options{})
	d := uitest.New(t, root)
	d.RequireView("poll 3s while running", "────")
	d.Send(ToastMsg{Level: common.ToastSuccess, Text: "approved !1"})
	d.RequireView("approved !1")
	d.RequireNoView("poll 3s while running")
}

// toastOf runs the command Fail returned and returns its toast.
func toastOf(t *testing.T, cmd tea.Cmd) ToastMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no toast")
	}
	m, ok := cmd().(ToastMsg)
	if !ok {
		t.Fatalf("not a toast: %T", cmd())
	}
	return m
}

// TestFailToastsFollowTheErrorTable is SPEC §11.1, one row per case, in en
// and ru: the toast text, the hint, the level.
func TestFailToastsFollowTheErrorTable(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		en, ru   string
		hintEN   string
		hintRU   string
		wantAuth bool
	}{
		{name: "403", err: &gitlab.Error{Kind: gitlab.ErrForbidden, Status: 403}, en: "Not enough permissions", ru: "Недостаточно прав"},
		{name: "403 scope", err: &gitlab.Error{Kind: gitlab.ErrInsufficientScope, Status: 403}, en: "The token lacks the api scope (read-only)", ru: "У токена нет scope api (только чтение)"},
		{name: "404", err: &gitlab.Error{Kind: gitlab.ErrNotFound, Status: 404}, en: "Not found or no access", ru: "Не найдено или нет доступа"},
		{name: "409", err: &gitlab.Error{Kind: gitlab.ErrConflict, Status: 409}, en: "Conflict: the object changed, refresh (r)", ru: "Конфликт: объект изменился, обнови (r)"},
		{name: "422", err: &gitlab.Error{Kind: gitlab.ErrNotPossible, Status: 422}, en: "Operation not possible", ru: "Операция невозможна"},
		{name: "429 with Retry-After", err: &gitlab.Error{Kind: gitlab.ErrRateLimited, Status: 429, RetryAfter: 120 * time.Second}, en: "GitLab rate limit, retry in 120s", ru: "GitLab ограничил запросы, повтор через 120 с"},
		{name: "429 fractional Retry-After rounds up", err: &gitlab.Error{Kind: gitlab.ErrRateLimited, Status: 429, RetryAfter: 1500 * time.Millisecond}, en: "GitLab rate limit, retry in 2s", ru: "GitLab ограничил запросы, повтор через 2 с"},
		{name: "429 without Retry-After", err: &gitlab.Error{Kind: gitlab.ErrRateLimited, Status: 429}, en: "GitLab rate limit, retry in 30s", ru: "GitLab ограничил запросы, повтор через 30 с"},
		{name: "network", err: &gitlab.Error{Kind: gitlab.ErrNetwork, Err: errors.New("dial tcp: no such host")}, en: "Host unreachable", ru: "Хост недоступен"},
		{name: "tls", err: &gitlab.Error{Kind: gitlab.ErrTLS, Err: errors.New("x509: unknown authority")}, en: "TLS error", ru: "Ошибка TLS", hintEN: "set tls.ca_file in config.yaml", hintRU: "укажи tls.ca_file в config.yaml"},
		{name: "5xx", err: &gitlab.Error{Kind: gitlab.ErrServer, Status: 502}, en: "GitLab server error", ru: "Ошибка сервера GitLab"},
	}
	for _, c := range cases {
		for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
			env := NewEnv(lang)
			got := toastOf(t, env.Fail(c.err))
			want, hint := c.en, c.hintEN
			if lang == i18n.RU {
				want, hint = c.ru, c.hintRU
			}
			if got.Text != want || got.Hint != hint || got.Level != common.ToastError {
				t.Errorf("%s/%s: toast = %+v, want %q hint %q", c.name, lang, got, want, hint)
			}
		}
	}
}

func TestOfflineHeaderBackoffAndRecovery(t *testing.T) {
	client := fake.New()
	env := NewEnv(i18n.EN)
	env.Client, env.User = client, client.User
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	env.Net = gitlab.NewMonitor()
	env.Settings = Settings{PollActive: 3 * time.Second, PollIdle: 30 * time.Second, PollWorkspace: 60 * time.Second}
	p := &probe{env: env}
	root := New(env, p, Options{})
	d := uitest.New(t, root)

	updated := now.Add(-5 * time.Minute)
	if got := env.HeaderRight(updated); got != "@"+env.User.Username+" · ⟳ 5m" {
		t.Fatalf("online header = %q", got)
	}

	netErr := &gitlab.Error{Kind: gitlab.ErrNetwork, Err: errors.New("dial tcp: no route to host")}
	env.Net.Report(netErr)
	d.Send(NetChangedMsg{Online: false})
	if got := ansi.Strip(env.HeaderRight(updated)); !strings.HasSuffix(got, "⟳ 5m (offline)") {
		t.Fatalf("offline header = %q", got)
	}
	if got := ansi.Strip(env.HeaderRight(time.Time{})); !strings.HasSuffix(got, "(offline)") {
		t.Fatalf("offline header without a load = %q", got)
	}
	d.Key("ctrl+l")
	if got := ansi.Strip(env.HeaderRight(updated)); !strings.HasSuffix(got, "⟳ 5м (офлайн)") {
		t.Fatalf("offline header in ru = %q", got)
	}
	d.Key("ctrl+l")

	// The delay grows exponentially: 3s → 6, 12, 24, 48, then 60 s at most.
	var got []time.Duration
	got = append(got, env.PollInterval(true)) // one failure: ×2
	for range 5 {
		env.Net.Report(netErr)
		got = append(got, env.PollInterval(true))
	}
	want := []time.Duration{6, 12, 24, 48, 60, 60}
	for i, w := range want {
		if got[i] != w*time.Second {
			t.Fatalf("active delays = %v", got)
		}
	}
	if env.PollInterval(false) != 60*time.Second || env.WorkspacePoll() != 60*time.Second {
		t.Fatalf("idle = %v, workspace = %v", env.PollInterval(false), env.WorkspacePoll())
	}

	// The first answer ends the outage: one toast, normal delays again.
	env.Net.Report(nil)
	d.Send(NetChangedMsg{Online: true})
	d.RequireView("Connection restored")
	if env.Net.Offline() || env.PollInterval(true) != 3*time.Second {
		t.Fatalf("still offline: fails = %d, interval = %v", env.Net.Fails(), env.PollInterval(true))
	}
	if got := env.HeaderRight(updated); strings.Contains(got, "offline") {
		t.Fatalf("header = %q", got)
	}
}

func TestListenNetDeliversChanges(t *testing.T) {
	env := NewEnv(i18n.EN)
	env.Client = fake.New()
	env.Net = gitlab.NewMonitor()
	root := New(env, &probe{env: env}, Options{})
	cmd := root.listenNet()
	if cmd == nil {
		t.Fatal("no listener")
	}
	env.Net.Report(&gitlab.Error{Kind: gitlab.ErrNetwork})
	msg := make(chan tea.Msg, 1)
	go func() { msg <- cmd() }()
	select {
	case m := <-msg:
		if n, ok := m.(NetChangedMsg); !ok || n.Online {
			t.Fatalf("msg = %#v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message")
	}
	if New(NewEnv(i18n.EN), &probe{}, Options{}).listenNet() != nil {
		t.Fatal("no monitor, no listener")
	}
}

// Toasts live 4 s, errors 8 s (SPEC §11.1).
func TestToastLifetimes(t *testing.T) {
	if toastTTL != 4*time.Second || toastErrorTTL != 8*time.Second {
		t.Fatalf("ttl = %v / %v", toastTTL, toastErrorTTL)
	}
}
