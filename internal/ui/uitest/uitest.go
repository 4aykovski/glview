// Package uitest drives Bubble Tea models in tests.
//
// Driver runs Update and the returned commands synchronously and lets tests
// read the rendered view as plain text. Commands that do not return within
// a short time (timers, tea.Tick) are dropped, so tests deliver ticks
// explicitly with Send. For an end-to-end check through a real
// tea.Program use Program (teatest).
package uitest

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// Default terminal size for UI tests.
const (
	Width  = 120
	Height = 40
)

// CmdTimeout is how long a command may run before it is treated as a timer
// and dropped.
var CmdTimeout = 150 * time.Millisecond

// Driver runs a model synchronously.
type Driver struct {
	t     testing.TB
	Model tea.Model
	Quit  bool
	Msgs  []tea.Msg // every message delivered to Update, for assertions
	depth int
}

// New initialises m, sends a window size and runs the init commands.
func New(t testing.TB, m tea.Model) *Driver {
	t.Helper()
	d := &Driver{t: t, Model: m}
	d.run(m.Init())
	d.Send(tea.WindowSizeMsg{Width: Width, Height: Height})
	return d
}

// Send delivers msg and runs resulting commands to completion.
func (d *Driver) Send(msg tea.Msg) {
	d.t.Helper()
	d.depth++
	defer func() { d.depth-- }()
	if d.depth > 200 {
		d.t.Fatalf("uitest: message loop too deep (last %T)", msg)
	}
	d.Msgs = append(d.Msgs, msg)
	m, cmd := d.Model.Update(msg)
	d.Model = m
	d.run(cmd)
}

// Run executes cmd and delivers its messages, as the runtime would.
func (d *Driver) Run(cmd tea.Cmd) {
	d.t.Helper()
	d.run(cmd)
}

// Key sends key presses: "j", "enter", "ctrl+l", "esc", "?"… Single runes
// and named keys are supported.
func (d *Driver) Key(keys ...string) {
	d.t.Helper()
	for _, k := range keys {
		d.Send(KeyPress(k))
	}
}

// View returns the rendered view without ANSI sequences.
func (d *Driver) View() string {
	return ansi.Strip(d.Model.View().Content)
}

// RequireView fails unless the view contains every substring.
func (d *Driver) RequireView(subs ...string) {
	d.t.Helper()
	v := d.View()
	for _, s := range subs {
		if !strings.Contains(v, s) {
			d.t.Fatalf("view does not contain %q:\n%s", s, v)
		}
	}
}

// RequireNoView fails if the view contains any substring.
func (d *Driver) RequireNoView(subs ...string) {
	d.t.Helper()
	v := d.View()
	for _, s := range subs {
		if strings.Contains(v, s) {
			d.t.Fatalf("view unexpectedly contains %q:\n%s", s, v)
		}
	}
}

func (d *Driver) run(cmd tea.Cmd) {
	if cmd == nil || d.Quit {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(CmdTimeout):
		return // a timer: tests deliver ticks themselves
	}
	if msg == nil {
		return
	}
	if _, ok := msg.(tea.QuitMsg); ok {
		d.Quit = true
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			d.run(c)
		}
		return
	}
	// tea.Sequence produces an unexported slice of commands: run in order.
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
		for i := 0; i < v.Len(); i++ {
			d.run(v.Index(i).Interface().(tea.Cmd))
		}
		return
	}
	d.Send(msg)
}

// KeyPress builds a key press message from a key name.
func KeyPress(k string) tea.KeyPressMsg {
	named := map[string]tea.Key{
		"enter":     {Code: tea.KeyEnter},
		"esc":       {Code: tea.KeyEscape},
		"tab":       {Code: tea.KeyTab},
		"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"space":     {Code: tea.KeySpace, Text: " "},
		"up":        {Code: tea.KeyUp},
		"down":      {Code: tea.KeyDown},
		"left":      {Code: tea.KeyLeft},
		"right":     {Code: tea.KeyRight},
		"backspace": {Code: tea.KeyBackspace},
		"pgup":      {Code: tea.KeyPgUp},
		"pgdown":    {Code: tea.KeyPgDown},
		"home":      {Code: tea.KeyHome},
		"end":       {Code: tea.KeyEnd},
	}
	if key, ok := named[k]; ok {
		return tea.KeyPressMsg(key)
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok && len([]rune(rest)) == 1 {
		return tea.KeyPressMsg(tea.Key{Code: []rune(rest)[0], Mod: tea.ModCtrl})
	}
	r := []rune(k)
	if len(r) == 1 {
		key := tea.Key{Code: r[0], Text: k}
		if r[0] >= 'A' && r[0] <= 'Z' {
			key.Code = r[0] + ('a' - 'A')
			key.ShiftedCode = r[0]
			key.Mod = tea.ModShift
		}
		return tea.KeyPressMsg(key)
	}
	panic("uitest: unknown key " + k)
}

// Program starts m in a real tea.Program at the default size (end-to-end).
func Program(t testing.TB, m tea.Model) *teatest.TestModel {
	t.Helper()
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(Width, Height))
}
