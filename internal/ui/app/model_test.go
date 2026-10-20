package app

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

type confirmedMsg struct{ name string }
type userLoadedMsg struct{ tok common.Token }

// dummy is a minimal screen: a cursor, polling while active, a confirm and a push.
type dummy struct {
	env    *Env
	name   string
	cursor int
	w, h   int
	active bool
	poll   common.Gen
	themes int
	langs  int
	conf   []string
}

func newDummy(env *Env, name string) *dummy {
	return &dummy{env: env, name: name, poll: common.NewGen()}
}

func (d *dummy) Init() tea.Cmd { return nil }
func (d *dummy) Title() string { return d.name }
func (d *dummy) SetSize(w, h int) {
	d.w, d.h = w, h
}

func (d *dummy) KeyMap() common.KeyMap {
	k := d.env.Msg.Keys
	return common.KeyMap{Short: []common.Binding{
		common.NewBinding([]string{"j"}, "j/k", k.Down, common.PriorityHigh),
		common.NewBinding([]string{"enter"}, "", k.Open, common.PriorityNormal),
		common.NewBinding([]string{"o"}, "", k.Browser, common.PriorityLow),
	}}
}

func (d *dummy) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case ActivatedMsg:
		d.active = true
		return d, common.PollAfter(time.Hour, d.poll.Next())
	case DeactivatedMsg:
		d.active = false
		d.poll.Next()
	case common.PollTickMsg:
		if !d.poll.Valid(msg.Token) || !d.active {
			return d, nil
		}
		tok := msg.Token
		return d, func() tea.Msg {
			_, _ = d.env.Client.Users().Current(context.Background())
			return userLoadedMsg{tok: tok}
		}
	case ThemeChangedMsg:
		d.themes++
	case LangChangedMsg:
		d.langs++
	case confirmedMsg:
		if msg.name == d.name {
			d.conf = append(d.conf, msg.name)
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j":
			d.cursor++
		case "n":
			return d, Push(newDummy(d.env, d.name+"-child"))
		case "x":
			name := d.name
			return d, OpenOverlay(NewConfirm(d.env, "Delete tag v1.0?", "This cannot be undone.",
				func() tea.Msg { return confirmedMsg{name: name} }))
		}
	}
	return d, nil
}

func (d *dummy) View() string {
	return fmt.Sprintf("screen=%s cursor=%d size=%dx%d", d.name, d.cursor, d.w, d.h)
}

func setup(t *testing.T) (*uitest.Driver, *Model, *fake.Client, *[]i18n.Lang) {
	t.Helper()
	client := fake.New()
	env := NewEnv(i18n.EN)
	env.Client = client
	var saved []i18n.Lang
	root := New(env, newDummy(env, "A"), Options{SaveLanguage: func(l i18n.Lang) error {
		saved = append(saved, l)
		return nil
	}})
	d := uitest.New(t, root)
	return d, root, client, &saved
}

func TestPushPopKeepsState(t *testing.T) {
	d, root, _, _ := setup(t)
	d.Key("j", "j")
	d.Key("n")
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.RequireView("screen=A-child cursor=0")
	d.Key("esc")
	d.RequireView("screen=A cursor=2", fmt.Sprintf("size=%dx%d", uitest.Width, uitest.Height-2))
	d.Key("esc") // no-op on root
	if root.Depth() != 1 || d.Quit {
		t.Fatal("esc on root must do nothing")
	}
}

func TestQuitOnlyFromRoot(t *testing.T) {
	d, _, _, _ := setup(t)
	d.Key("n", "q")
	if d.Quit {
		t.Fatal("q must not quit from a nested screen")
	}
	d.Key("esc", "q")
	if !d.Quit {
		t.Fatal("q on root must quit")
	}
}

func TestCtrlCQuitsFromOverlay(t *testing.T) {
	d, _, _, _ := setup(t)
	d.Key("x", "ctrl+c")
	if !d.Quit {
		t.Fatal("ctrl+c must quit from anywhere")
	}
}

func TestConfirmDefaultsToCancel(t *testing.T) {
	d, root, _, _ := setup(t)
	d.Key("x")
	d.RequireView("Delete tag v1.0?", "Cancel", "Yes")
	d.Key("enter")
	d.RequireNoView("Delete tag v1.0?")
	if got := root.Top().(*dummy).conf; len(got) != 0 {
		t.Fatalf("enter on default must cancel, confirmed=%v", got)
	}
	d.Key("x", "y")
	if got := root.Top().(*dummy).conf; len(got) != 1 {
		t.Fatalf("y must confirm, confirmed=%v", got)
	}
	d.Key("x", "right", "enter")
	if got := root.Top().(*dummy).conf; len(got) != 2 {
		t.Fatalf("moving to Yes then enter must confirm, confirmed=%v", got)
	}
}

func TestLanguageToggle(t *testing.T) {
	d, root, _, saved := setup(t)
	d.RequireView("down", "help")
	d.Key("n") // a second screen must also get LangChangedMsg
	d.Key("ctrl+l")
	d.RequireView("вниз", "хелп", "Язык: русский")
	if root.Env().Lang != i18n.RU || len(*saved) != 1 || (*saved)[0] != i18n.RU {
		t.Fatalf("lang=%s saved=%v", root.Env().Lang, *saved)
	}
	for i, s := range root.stack {
		if s.(*dummy).langs != 1 {
			t.Fatalf("screen %d got %d LangChangedMsg", i, s.(*dummy).langs)
		}
	}
	d.Key("ctrl+l")
	d.RequireView("down")
}

func TestHelpOverlay(t *testing.T) {
	d, _, _, _ := setup(t)
	d.Key("?")
	d.RequireView("Keys", "THIS SCREEN", "EVERYWHERE", "ctrl+l")
	d.Key("j") // keys go to the overlay, not the screen
	d.Key("esc")
	d.RequireNoView("EVERYWHERE")
	d.RequireView("cursor=0")
}

func TestPollingOnlyOnTop(t *testing.T) {
	d, root, client, _ := setup(t)
	a := root.Top().(*dummy)
	tokA := a.poll.Current()

	d.Key("n")
	b := root.Top().(*dummy)
	d.Send(common.PollTickMsg{Token: tokA})
	if n := client.CallCount("Users.Current"); n != 0 {
		t.Fatalf("covered screen polled: %d calls", n)
	}
	d.Send(common.PollTickMsg{Token: b.poll.Current()})
	if n := client.CallCount("Users.Current"); n != 1 {
		t.Fatalf("top screen did not poll: %d calls", n)
	}

	d.Key("esc")
	d.Send(common.PollTickMsg{Token: tokA}) // stale token from before the push
	if n := client.CallCount("Users.Current"); n != 1 {
		t.Fatalf("stale tick was honoured: %d calls", n)
	}
	d.Send(common.PollTickMsg{Token: a.poll.Current()})
	if n := client.CallCount("Users.Current"); n != 2 {
		t.Fatalf("reactivated screen did not poll: %d calls", n)
	}
}

func TestTooSmall(t *testing.T) {
	d, _, _, _ := setup(t)
	d.Send(tea.WindowSizeMsg{Width: 70, Height: 20})
	d.RequireView("80×24")
	d.RequireNoView("screen=A")
}

func TestLightBackgroundSwitchesTheme(t *testing.T) {
	d, root, _, _ := setup(t)
	d.Send(tea.BackgroundColorMsg{Color: color.White})
	if root.Env().Styles.Theme.IsDark {
		t.Fatal("theme must switch to light")
	}
	if root.Top().(*dummy).themes != 1 {
		t.Fatal("screens must receive ThemeChangedMsg")
	}
	d.Send(tea.BackgroundColorMsg{Color: color.White})
	if root.Top().(*dummy).themes != 1 {
		t.Fatal("same background must not rebroadcast")
	}
}

func TestToastLifecycle(t *testing.T) {
	d, root, _, _ := setup(t)
	d.Send(ToastMsg{Level: common.ToastError, Text: "Token rejected", Hint: "run: glab auth login"})
	d.RequireView("Token rejected — run: glab auth login")
	d.Send(ToastMsg{Level: common.ToastSuccess, Text: "approved !479"})
	d.RequireView("approved !479")
	d.RequireNoView("Token rejected")
	d.Send(toastExpiredMsg{id: root.toastID - 1}) // stale expiry must not clear the new toast
	d.RequireView("approved !479")
	d.Send(toastExpiredMsg{id: root.toastID})
	d.RequireNoView("approved !479")
}

func TestReplaceRoot(t *testing.T) {
	d, root, _, _ := setup(t)
	d.Key("n", "n")
	d.Send(ReplaceRootMsg{Screen: newDummy(root.Env(), "R")})
	if root.Depth() != 1 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.RequireView("screen=R")
}

// TestProgramSmoke runs the root inside a real tea.Program.
func TestProgramSmoke(t *testing.T) {
	env := NewEnv(i18n.EN)
	env.Client = fake.New()
	tm := uitest.Program(t, New(env, newDummy(env, "A"), Options{}))
	tm.Send(uitest.KeyPress("j"))
	tm.Send(uitest.KeyPress("ctrl+c"))
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	final := tm.FinalModel(t).(*Model)
	if !strings.Contains(final.Top().View(), "cursor=1") {
		t.Fatalf("final view: %s", final.Top().View())
	}
}
