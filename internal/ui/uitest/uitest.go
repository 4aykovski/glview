// Package uitest drives Bubble Tea models in tests.
//
// Driver runs Update and the returned commands synchronously and lets tests
// read the rendered view as plain text. Commands that do not return within
// a short time (timers, tea.Tick) are dropped, so tests deliver ticks
// explicitly with Send. For an end-to-end check through a real
// tea.Program use Program (teatest).
package uitest

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// Default terminal size for UI tests: 120×40, or WxH from the UITEST_SIZE
// environment variable. UITEST_SIZE=80x24 go test ./... sweeps the whole
// suite at the minimum size to look for overflow; the failures that are not
// about overflow are expected there (texts are cut).
var Width, Height = defaultSize()

func defaultSize() (int, int) {
	var w, h int
	if _, err := fmt.Sscanf(os.Getenv("UITEST_SIZE"), "%dx%d", &w, &h); err == nil && w > 0 && h > 0 {
		return w, h
	}
	return 120, 40
}

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

	// width and height are the last window size sent to the model.
	width, height int
	// NoFitCheck turns off the check of View against the terminal size.
	NoFitCheck bool
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
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		d.width, d.height = ws.Width, ws.Height
	}
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

// View returns the rendered view without ANSI sequences. Unless NoFitCheck
// is set, it fails the test when the view does not fit the terminal (SPEC
// §12.3): every assertion on a screen therefore also checks 80×24 and
// 120×40 overflow.
func (d *Driver) View() string {
	d.t.Helper()
	v := ansi.Strip(d.Model.View().Content)
	if !d.NoFitCheck {
		if err := FitError(v, d.width, d.height); err != "" {
			d.t.Fatalf("%s:\n%s", err, v)
		}
	}
	return v
}

// FitError reports the first way a plain-text view does not fit a w×h
// terminal: a line wider than w or more than h lines. Empty when it fits or
// when the terminal is smaller than the supported minimum (the "too small"
// message is not required to fit). A zero size is not checked.
func FitError(view string, w, h int) string {
	if w == 0 || h == 0 || w < 80 || h < 24 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) > h {
		return fmt.Sprintf("view has %d lines, terminal %dx%d", len(lines), w, h)
	}
	for i, l := range lines {
		if lw := ansi.StringWidth(l); lw > w {
			return fmt.Sprintf("line %d is %d cells wide, terminal %dx%d: %q", i+1, lw, w, h, l)
		}
	}
	return ""
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
	if rest, ok := strings.CutPrefix(k, "alt+"); ok {
		key := KeyPress(rest).Key()
		key.Mod |= tea.ModAlt
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

// RequireFitsSizes renders the current state at 80×24 and 120×40 and fails
// if either view overflows the terminal (the check of View), then restores
// the default size.
func (d *Driver) RequireFitsSizes() {
	d.t.Helper()
	for _, sz := range [][2]int{{80, 24}, {120, 40}} {
		d.Send(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		d.View()
	}
	d.Send(tea.WindowSizeMsg{Width: Width, Height: Height})
}

// RequireFits checks the current state at 80×24 and 120×40 in the current
// language and, switching with ctrl+l, in the other one (the state is not
// changed otherwise). It is the SPEC §12.3 invariant: no line wider than
// the terminal, no more lines than its height.
func (d *Driver) RequireFits() {
	d.t.Helper()
	d.RequireFitsSizes()
	d.Key("ctrl+l")
	d.RequireFitsSizes()
	d.Key("ctrl+l")
}
