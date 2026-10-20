package setup

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

type nextScreen struct{ env *app.Env }

func (n *nextScreen) Init() tea.Cmd                        { return nil }
func (n *nextScreen) Update(tea.Msg) (app.Screen, tea.Cmd) { return n, nil }
func (n *nextScreen) View() string                         { return "NEXT for @" + n.env.User.Username }
func (n *nextScreen) Title() string                        { return "next" }
func (n *nextScreen) KeyMap() common.KeyMap                { return common.KeyMap{} }
func (n *nextScreen) SetSize(int, int)                     {}

type saved struct{ host, token string }

func newRoot(t *testing.T, opts Options) (*uitest.Driver, *app.Model, *[]saved, *[]string) {
	t.Helper()
	env := app.NewEnv(i18n.EN)
	var saves []saved
	var tokens []string
	client := fake.New()
	opts.Normalize = config.NormalizeHost
	opts.Connect = func(_ context.Context, host, token string) (gitlab.Client, gitlab.User, error) {
		tokens = append(tokens, token)
		if token != "glpat-good-token" {
			return nil, gitlab.User{}, &gitlab.Error{Kind: gitlab.ErrUnauthorized, Status: 401}
		}
		return client, gitlab.User{Username: "egor"}, nil
	}
	opts.Save = func(host, token string) error {
		saves = append(saves, saved{host, token})
		return nil
	}
	opts.Next = func(env *app.Env) app.Screen { return &nextScreen{env: env} }
	root := app.New(env, New(env, opts), app.Options{})
	return uitest.New(t, root), root, &saves, &tokens
}

func typeText(d *uitest.Driver, s string) {
	for _, r := range s {
		d.Key(string(r))
	}
}

func TestSetupSuccess(t *testing.T) {
	d, root, saves, _ := newRoot(t, Options{Host: "https://gitlab.example.com"})
	d.RequireView("Connect to GitLab", "Instance URL")
	d.Key("enter") // accept prefilled URL
	d.RequireView("Personal access token")
	typeText(d, "glpat-good-token")
	d.Key("enter")

	d.RequireView("NEXT for @egor", "Logged in as @egor")
	if root.Env().Host != "https://gitlab.example.com" || root.Env().Client == nil {
		t.Fatalf("env not filled: %+v", root.Env())
	}
	if len(*saves) != 1 || (*saves)[0] != (saved{"https://gitlab.example.com", "glpat-good-token"}) {
		t.Fatalf("saves = %+v", *saves)
	}
}

func TestSetupBadTokenStaysOpen(t *testing.T) {
	d, _, saves, _ := newRoot(t, Options{Host: "https://gitlab.example.com"})
	d.Key("enter")
	typeText(d, "wrong")
	d.Key("enter")
	d.RequireView("Token is invalid or expired", "Instance URL")
	d.RequireNoView("NEXT")
	if len(*saves) != 0 {
		t.Fatal("nothing must be saved on failure")
	}
}

func TestSetupGlabTokenNotSaved(t *testing.T) {
	d, _, saves, tokens := newRoot(t, Options{
		Host:      "https://gitlab.example.com",
		GlabToken: func(context.Context, string) (string, error) { return "glpat-good-token", nil },
	})
	d.Key("enter")         // URL
	d.Key("down", "enter") // choose "Use token from glab"
	d.RequireView("NEXT for @egor")
	if len(*tokens) != 1 || (*tokens)[0] != "glpat-good-token" {
		t.Fatalf("connect tokens = %v", *tokens)
	}
	if len(*saves) != 1 || (*saves)[0].token != "" {
		t.Fatalf("glab token must not be saved: %+v", *saves)
	}
}

func TestSetupShowsStartError(t *testing.T) {
	d, _, _, _ := newRoot(t, Options{Host: "https://h.example.com", Err: &gitlab.Error{Kind: gitlab.ErrUnauthorized}})
	d.RequireView("Token is invalid or expired")
}

func TestSetupKeysGoToForm(t *testing.T) {
	d, _, _, _ := newRoot(t, Options{Host: ""})
	d.Key("ctrl+a") // ensure no panic on control keys
	d.Key("q", "?") // typed into the URL field, not quit/help
	if d.Quit {
		t.Fatal("q must be typed into the field")
	}
	d.RequireNoView("EVERYWHERE")
}
