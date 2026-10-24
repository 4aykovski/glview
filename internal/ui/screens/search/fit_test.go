package search

import (
	"testing"

	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/screens/project"
	"github.com/4aykovski/glview/internal/ui/screens/start"
	wsscreen "github.com/4aykovski/glview/internal/ui/screens/workspace"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the p picker, the F
// search and the start screen (80×24 and 120×40, en and ru).
func TestFitsTerminal(t *testing.T) {
	t.Run("picker and global search", func(t *testing.T) {
		client := newFake()
		env, last := setup(t, client)
		env.Catalog.SetCurrent("platform/route-service", repo("/src/route-service"))
		d := uitest.New(t, app.New(env, wsscreen.New(env, "~/src", wsScope()), app.Options{}))
		d.Key("p")
		d.RequireFits()
		d.Key("r", "o", "u")
		d.RequireFits()
		d.Key("x", "x", "x", "x") // nothing found
		d.RequireFits()
		d.Key("esc", "F")
		d.RequireFits()
		d.Key("p", "l", "a")
		d.Send((*last).DebounceMsg())
		d.RequireFits()
	})
	t.Run("picker above a project", func(t *testing.T) {
		client := newFake()
		env, _ := setup(t, client)
		env.Catalog.SetCurrent("platform/route-service", repo("/src/route-service"))
		d := uitest.New(t, app.New(env, project.New(env, project.Options{Path: "platform/route-service"}), app.Options{}))
		d.Key("p")
		d.RequireFits()
	})
	t.Run("not on the instance", func(t *testing.T) {
		env, _ := setup(t, newFake())
		scr := start.NewNotOnInstance(env, start.NotOnInstanceInfo{
			Reason:     env.Msg.Startup.NotOnInstance("gitlab.example.com"),
			RemoteURL:  "git@github.com:me/dotfiles.git",
			RemoteHost: "github.com",
		}, nil)
		d := uitest.New(t, app.New(env, scr, app.Options{}))
		d.RequireFits()
		d.Key("?")
		d.RequireFits()
	})
}
