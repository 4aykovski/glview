package start

import (
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/gitremote"
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
	}, func() app.Screen { opened = true; return NewEmptyWorkspace(env, "/src", 1, 1, 0) })
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	d.RequireView("This project is not on gitlab.example.com", "github.com", "git@github.com:me/dotfiles.git", "pick project", "workspace of parent folder")
	d.Key("p")
	d.RequireView("coming in the next milestone")
	d.Key("w")
	if !opened {
		t.Fatal("w must open the parent workspace")
	}
	d.RequireView("No GitLab projects in /src")
}

func TestEmptyWorkspace(t *testing.T) {
	env := newEnv(fake.New())
	d := uitest.New(t, app.New(env, NewEmptyWorkspace(env, "/tmp/ws", 3, 3, 0), app.Options{}))
	d.RequireView("No GitLab projects in /tmp/ws", "scanned 3 folders: 3 with another remote, 0 without .git")
	d.Key("ctrl+l")
	d.RequireView("В /tmp/ws нет GitLab-проектов", "просмотрено 3 папки: с чужим remote 3, без .git 0")
}

func TestWorkspacePlaceholder(t *testing.T) {
	env := newEnv(fake.New())
	entries := []gitremote.LocalProject{
		{Name: "dotfiles", Kind: gitremote.KindForeign, Remote: &gitremote.Remote{Host: "github.com", Path: "egor/dotfiles"}, Repo: &gitremote.Repo{Branch: "main"}},
		{Name: "notes", Kind: gitremote.KindNoGit},
		{Name: "route-service", Kind: gitremote.KindGitLab, ProjectPath: "platform/route-service", Repo: &gitremote.Repo{Branch: "fix/slot-race"}},
	}
	ws := NewWorkspace(env, "/tmp/src", entries)
	d := uitest.New(t, app.New(env, ws, app.Options{}))
	d.RequireView("1 GitLab project", "platform/route-service", "fix/slot-race",
		"github.com/egor/dotfiles · not GitLab", "skipped without .git: notes")
	if ws.entries[0].Name != "route-service" {
		t.Fatal("GitLab projects must come first")
	}
	d.Key("j", "j", "j")
	if ws.cursor != 1 {
		t.Fatalf("cursor = %d", ws.cursor)
	}
}
