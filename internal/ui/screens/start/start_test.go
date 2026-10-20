package start

import (
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

func newEnv(client *fake.Client) *app.Env {
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = client, gitlab.User{Username: "egor"}, "https://gitlab.example.com"
	env.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	return env
}

func TestNotOnInstance(t *testing.T) {
	env := newEnv(fake.New())
	opened := false
	scr := NewNotOnInstance(env, NotOnInstanceInfo{
		Reason:     env.Msg.Startup.NotOnInstance("gitlab.example.com"),
		RemoteURL:  "git@github.com:me/dotfiles.git",
		RemoteHost: "github.com",
	}, func() app.Screen {
		opened = true
		return NewNotOnInstance(env, NotOnInstanceInfo{Reason: "parent workspace"}, nil)
	})
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	d.RequireView("This project is not on gitlab.example.com", "github.com", "git@github.com:me/dotfiles.git", "pick project", "workspace of parent folder")
	d.Key("p")
	d.RequireView("coming in the next milestone")
	d.Key("w")
	if !opened {
		t.Fatal("w must open the parent workspace")
	}
	d.RequireView("parent workspace")
}
