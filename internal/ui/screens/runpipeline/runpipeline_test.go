package runpipeline

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

const path = "platform/route-service"

func TestMain(m *testing.M) {
	uitest.CmdTimeout = 40 * time.Millisecond
	os.Exit(m.Run())
}

type host struct{}

func (host) Init() tea.Cmd                          { return nil }
func (h host) Update(tea.Msg) (app.Screen, tea.Cmd) { return h, nil }
func (host) View() string                           { return "host" }
func (host) Title() string                          { return "host" }
func (host) KeyMap() common.KeyMap                  { return common.KeyMap{} }
func (host) SetSize(int, int)                       {}

func open(t *testing.T, c *fake.Client, initial, local string) (*uitest.Driver, *app.Model, *Form) {
	t.Helper()
	return openWith(t, c, initial, local, nil)
}

// openWith opens the form after adjusting the env (settings).
func openWith(t *testing.T, c *fake.Client, initial, local string, setup func(*app.Env)) (*uitest.Driver, *app.Model, *Form) {
	t.Helper()
	env := app.NewEnv(i18n.EN)
	if setup != nil {
		setup(env)
	}
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	root := app.New(env, host{}, app.Options{})
	d := uitest.New(t, root)
	f := New(env, gitlab.ProjectRef{ID: 42, FullPath: path}, initial, local)
	d.Send(app.OpenOverlayMsg{Overlay: f})
	d.Run(f.Init())
	return d, root, f
}

func newFake() *fake.Client {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	c.SetBranches(path, "main", "feat/surge", "feat/slots", "fix/race")
	c.SetTags(path, gitlab.Tag{Name: "v1.14.2"}, gitlab.Tag{Name: "v1.15.0"})
	return c
}

func typeText(d *uitest.Driver, s string) {
	for _, r := range s {
		d.Key(string(r))
	}
}

func TestLocalBranchComesFirst(t *testing.T) {
	d, _, f := open(t, newFake(), "feat/surge", "feat/surge")
	if f.Ref() != "feat/surge" {
		t.Fatalf("ref = %q", f.Ref())
	}
	d.RequireView("Run pipeline", "Ref", "› feat/surge  (current in git)", "will be created: ref=feat/surge, variables=0")
	if len(f.cands) == 0 || !f.cands[0].current {
		t.Fatalf("cands = %+v", f.cands)
	}
}

func TestDebounceOneSearchPerPause(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "main", "")
	for range 10 {
		d.Key("backspace")
	}
	c.ResetCalls()
	typeText(d, "feat/s")
	if c.CallCount("Projects.Branches") != 0 {
		t.Fatal("typing alone must not search")
	}
	// Ticks of the earlier keystrokes are stale; only the last one fires.
	d.Send(searchTickMsg{tok: stale(f.inputGen)})
	if c.CallCount("Projects.Branches") != 0 {
		t.Fatal("a stale tick must be ignored")
	}
	d.Send(f.DebounceMsg())
	if c.CallCount("Projects.Branches") != 1 || c.CallCount("Tags.List") != 1 {
		t.Fatalf("branches=%d tags=%d", c.CallCount("Projects.Branches"), c.CallCount("Tags.List"))
	}
	for _, call := range c.Calls() {
		if call.Method == "Projects.Branches" && call.Args[1] != "feat/s" {
			t.Fatalf("search term = %v", call.Args)
		}
	}
	d.RequireView("feat/surge", "feat/slots")
	d.RequireNoView("fix/race")
}

func TestPickCandidateWithArrowsAndEnter(t *testing.T) {
	d, _, f := open(t, newFake(), "", "")
	d.Key("down", "down", "enter")
	if f.Ref() != f.cands[2].name {
		t.Fatalf("ref = %q, cands = %+v", f.Ref(), f.cands)
	}
}

func TestHintsPickValueAndCreate(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", gitlab.CIConfigVariable{Key: "DEPLOY_ENV", Value: "staging", ValueOptions: []string{"staging", "prod-canary"}}, gitlab.CIConfigVariable{Key: "SKIP_E2E"})
	d, root, f := open(t, c, "main", "")
	d.RequireView("From .gitlab-ci.yml:", "1 DEPLOY_ENV [staging, prod-canary]", "2 SKIP_E2E")
	d.Key("tab", "1")
	d.RequireView("Value for DEPLOY_ENV", "› staging")
	d.Key("enter")
	if v := f.Vars(); len(v) != 1 || v[0].Key != "DEPLOY_ENV" || v[0].Value != "staging" {
		t.Fatalf("vars = %+v", v)
	}
	d.RequireView("variables=1")
	d.Key("tab", "enter") // from the table to the button
	var call fake.Call
	for _, cl := range c.Calls() {
		if cl.Method == "Pipelines.Create" {
			call = cl
		}
	}
	vars, _ := call.Args[2].([]gitlab.CIVariable)
	if call.Args[1] != "main" || len(vars) != 1 || vars[0].Key != "DEPLOY_ENV" || vars[0].Value != "staging" {
		t.Fatalf("create call = %+v", call)
	}
	if root.Depth() != 2 {
		t.Fatalf("the new pipeline screen must be on top, depth = %d", root.Depth())
	}
	d.RequireView(path+" › Pipelines › #", "pipeline #")
}

func TestHintWithoutOptionsAddsDefaultValue(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", gitlab.CIConfigVariable{Key: "SKIP_E2E", Value: "false"})
	d, _, f := open(t, c, "main", "")
	d.Key("tab", "1")
	if v := f.Vars(); len(v) != 1 || v[0].Key != "SKIP_E2E" || v[0].Value != "false" {
		t.Fatalf("vars = %+v", v)
	}
}

func TestHintsNotReadyRetriesThenGivesUp(t *testing.T) {
	c := newFake()
	for range 4 {
		c.FailNext("Projects.CIConfigVariables", gitlab.ErrNotReady)
	}
	d, _, f := open(t, c, "main", "")
	d.RequireView("loading hints")
	for attempt := 1; attempt <= 3; attempt++ {
		d.Send(hintsRetryMsg{tok: f.hintGen.Current(), attempt: attempt})
	}
	if c.CallCount("Projects.CIConfigVariables") != 4 {
		t.Fatalf("calls = %d, want 1 + retries after 1s, 2s, 4s", c.CallCount("Projects.CIConfigVariables"))
	}
	d.RequireView("hints unavailable")
	// The form still works.
	d.Key("tab", "tab", "enter")
	if c.CallCount("Pipelines.Create") != 1 {
		t.Fatal("the form must stay usable without hints")
	}
}

func TestHintsRecoverAfterRetry(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", gitlab.CIConfigVariable{Key: "A"})
	c.FailNext("Projects.CIConfigVariables", gitlab.ErrNotReady)
	d, _, f := open(t, c, "main", "")
	d.Send(hintsRetryMsg{tok: f.hintGen.Current(), attempt: 1})
	d.RequireView("1 A")
}

func TestEditVariablesAndRefRequired(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "", "")
	d.Key("tab", "tab", "enter")
	d.RequireView("Enter a branch or tag")
	if c.CallCount("Pipelines.Create") != 0 {
		t.Fatal("empty ref must not create")
	}
	d.Key("tab") // the button → back to the ref
	typeText(d, "main")
	d.Key("tab", "a")
	typeText(d, "FOO")
	d.Key("enter")
	typeText(d, "bar")
	d.Key("enter", "tab", "enter")
	var vars []gitlab.CIVariable
	for _, cl := range c.Calls() {
		if cl.Method == "Pipelines.Create" {
			vars, _ = cl.Args[2].([]gitlab.CIVariable)
		}
	}
	if len(vars) != 1 || vars[0].Key != "FOO" || vars[0].Value != "bar" || f.Ref() != "main" {
		t.Fatalf("vars = %+v", vars)
	}
}

func TestEscClosesWithoutRunning(t *testing.T) {
	c := newFake()
	d, root, _ := open(t, c, "main", "")
	d.Key("esc")
	d.RequireNoView("Run pipeline")
	if c.CallCount("Pipelines.Create") != 0 || root.Depth() != 1 {
		t.Fatal("esc must not run anything")
	}
}

// stale returns the token of the generation before the current one.
func stale(g common.Gen) common.Token {
	t := g.Current()
	t.N--
	return t
}

func names(f *Form) []string {
	out := make([]string, len(f.cands))
	for i, c := range f.cands {
		out[i] = c.name
	}
	return out
}

func toasts(d *uitest.Driver) []app.ToastMsg {
	var out []app.ToastMsg
	for _, m := range d.Msgs {
		if t, ok := m.(app.ToastMsg); ok {
			out = append(out, t)
		}
	}
	return out
}

func calls(c *fake.Client, method string) []fake.Call {
	var out []fake.Call
	for _, cl := range c.Calls() {
		if cl.Method == method {
			out = append(out, cl)
		}
	}
	return out
}

// ---- ref: prefill, order, window, summary ----

func TestPrefilledValueDoesNotFilter(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "feat/surge", "feat/surge")
	for _, cl := range calls(c, "Projects.Branches") {
		if cl.Args[1] != "" {
			t.Fatalf("the first search must run with an empty query: %v", cl.Args)
		}
	}
	d.RequireView("› feat/surge  (current in git)", "main", "fix/race", "v1.14.2  (tag)", "branches 4 · tags 2 · 1/6")
	if f.candCursor != 0 || f.cands[0].name != "feat/surge" {
		t.Fatalf("cursor = %d, cands = %v", f.candCursor, names(f))
	}
}

func TestPrefilledDefaultBranchGetsTheCursor(t *testing.T) {
	// No local branch: the field holds the default branch, the order is
	// refs_first and the cursor stands on the prefilled ref.
	d, _, f := open(t, newFake(), "fix/race", "")
	if f.cands[0].name != "main" || f.cands[f.candCursor].name != "fix/race" {
		t.Fatalf("cursor = %d, cands = %v", f.candCursor, names(f))
	}
	d.RequireView("› fix/race", "4/6")
}

func TestPrefilledTypingReplacesTheValue(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "feat/surge", "feat/surge")
	typeText(d, "v1")
	if f.Ref() != "v1" {
		t.Fatalf("the first character must replace the prefilled ref, got %q", f.Ref())
	}
	c.ResetCalls()
	d.Send(f.DebounceMsg())
	if cl := calls(c, "Tags.List"); len(cl) != 1 || cl[0].Args[1] != "v1" {
		t.Fatalf("after an edit the field filters: %+v", cl)
	}
	d.RequireView("v1.14.2", "v1.15.0")
	d.RequireNoView("fix/race")
}

func TestPrefilledBackspaceClears(t *testing.T) {
	d, _, f := open(t, newFake(), "feat/surge", "feat/surge")
	d.Key("backspace")
	if f.Ref() != "" || f.prefilled {
		t.Fatalf("backspace must clear the prefilled ref, got %q", f.Ref())
	}
}

func TestPrefilledArrowsDeselectForEditing(t *testing.T) {
	d, _, f := open(t, newFake(), "feat/surge", "feat/surge")
	d.Key("left")
	if f.Ref() != "feat/surge" || f.prefilled {
		t.Fatalf("left keeps the text and deselects it: %q prefilled=%v", f.Ref(), f.prefilled)
	}
	d.Key("x")
	if f.Ref() != "feat/surgxe" {
		t.Fatalf("ref = %q", f.Ref())
	}
	d, _, f = open(t, newFake(), "main", "")
	d.Key("home", "x")
	if f.Ref() != "xmain" {
		t.Fatalf("ref = %q", f.Ref())
	}
}

func TestRefsTagsFirstKeepsLocalBranchOnTop(t *testing.T) {
	d, _, f := openWith(t, newFake(), "feat/surge", "feat/surge", func(e *app.Env) { e.Settings.RefsTagsFirst = true })
	want := []string{"feat/surge", "v1.14.2", "v1.15.0", "main", "feat/slots", "fix/race"}
	if got := names(f); !slices.Equal(got, want) {
		t.Fatalf("cands = %v, want %v", got, want)
	}
	d.RequireView("tags 2 · branches 4 · 1/6")
}

func TestRefsTagsFirstWithoutLocalBranch(t *testing.T) {
	_, _, f := openWith(t, newFake(), "main", "", func(e *app.Env) { e.Settings.RefsTagsFirst = true })
	if got := names(f); got[0] != "v1.14.2" || f.cands[f.candCursor].name != "main" {
		t.Fatalf("cands = %v, cursor = %d", got, f.candCursor)
	}
}

func TestPrefillOffLeavesFieldEmptyAndLocalBranchInItsGroup(t *testing.T) {
	d, _, f := openWith(t, newFake(), "", "feat/surge", func(e *app.Env) { e.Settings.PrefillLocalBranch = false })
	if f.Ref() != "" {
		t.Fatalf("ref = %q", f.Ref())
	}
	want := []string{"main", "feat/surge", "feat/slots", "fix/race", "v1.14.2", "v1.15.0"}
	if got := names(f); !slices.Equal(got, want) {
		t.Fatalf("cands = %v, want %v", got, want)
	}
	d.RequireView("› main", "  feat/surge  (current in git)")
}

func manyRefs() *fake.Client {
	c := newFake()
	bs := make([]string, 25)
	for i := range bs {
		bs[i] = fmt.Sprintf("feat/b%02d", i)
	}
	c.SetBranches(path, bs...)
	return c
}

func TestSuggestionWindowFollowsCursor(t *testing.T) {
	d, _, f := open(t, manyRefs(), "", "")
	d.RequireView("feat/b00", "feat/b05", "branches 20+ · tags 2 · 1/22")
	d.RequireNoView("feat/b06")
	d.Key("down", "down", "down", "down", "down", "down", "down")
	d.RequireView("› feat/b07", "8/22")
	d.RequireNoView("feat/b00", "feat/b01")
	d.Key("enter")
	if f.Ref() != "feat/b07" {
		t.Fatalf("enter must take the visible selected row, got %q", f.Ref())
	}
}

func TestPageKeysJumpBetweenGroups(t *testing.T) {
	d, _, f := open(t, manyRefs(), "", "")
	d.Key("pgdown")
	if f.cands[f.candCursor].name != "v1.14.2" {
		t.Fatalf("pgdown must go to the first tag, cursor = %d", f.candCursor)
	}
	d.RequireView("› v1.14.2  (tag)", "21/22")
	d.Key("pgdown") // no next group
	if f.candCursor != 20 {
		t.Fatalf("cursor = %d", f.candCursor)
	}
	d.Key("down", "pgup")
	if f.candCursor != 0 {
		t.Fatalf("pgup must go to the first branch, cursor = %d", f.candCursor)
	}
	d.RequireView("› feat/b00")
}

func TestSearchingUntilBothAnswersArrive(t *testing.T) {
	d, _, f := open(t, newFake(), "", "")
	d.RequireNoView("searching…")
	f.search() // the answers are delivered by hand below
	d.Send(branchesMsg{tok: f.searchGen.Current(), names: []string{"main"}})
	d.RequireView("searching…")
	d.Send(branchesMsg{tok: stale(f.searchGen), names: []string{"stale"}})
	d.RequireNoView("stale")
	d.Send(tagsMsg{tok: f.searchGen.Current(), names: []string{"v1.14.2"}})
	d.RequireNoView("searching…")
	d.RequireView("branches 1 · tags 1 · 1/2")
}

func TestFailedBranchesEmptyTheirGroupAndToast(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "", "")
	d.RequireView("main")
	c.FailNext("Projects.Branches", errors.New("boom"))
	typeText(d, "v")
	d.Send(f.DebounceMsg())
	d.RequireNoView("main")
	d.RequireView("v1.14.2", "branches 0 · tags 2")
	if ts := toasts(d); len(ts) != 1 || ts[0].Level != common.ToastError {
		t.Fatalf("toasts = %+v", ts)
	}
}

func TestBothRefRequestsFailedOneToast(t *testing.T) {
	c := newFake()
	c.FailNext("Projects.Branches", errors.New("boom"))
	c.FailNext("Tags.List", errors.New("bang"))
	d, _, _ := open(t, c, "", "")
	d.RequireView("no refs: request failed")
	if ts := toasts(d); len(ts) != 1 {
		t.Fatalf("one toast for both failures, got %+v", ts)
	}
}

// ---- variables, hints, key lines ----

func TestEmptyTableTellsHowToAdd(t *testing.T) {
	d, _, _ := open(t, newFake(), "main", "")
	d.RequireView("no variables — a to add")
}

func TestKeyLineFollowsFocus(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", gitlab.CIConfigVariable{Key: "DEPLOY_ENV", ValueOptions: []string{"staging", "prod-canary"}})
	d, _, _ := open(t, c, "main", "")
	d.RequireView("↑/↓ ref · pgdn/pgup group", "tab next")
	d.RequireNoView("ctrl+s")
	d.Key("tab")
	d.RequireView("a add · d delete · enter edit", "1-9/s hint")
	d.Key("a")
	d.RequireView("enter save · esc discard input")
	d.Key("esc", "1")
	d.RequireView("j/k choose · enter take · esc close")
	d.Key("esc", "tab")
	d.RequireView("enter run · tab next · esc cancel", "enter Run")
}

// buttonFocused reports whether the Run button is drawn with the active
// (reverse) button style.
func buttonFocused(d *uitest.Driver) bool {
	return strings.Contains(d.Model.View().Content, "7m \x1b[1menter Run")
}

func TestTabCyclesRefVariablesButton(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "main", "")
	order := []focus{focusVars, focusButton, focusRef, focusVars}
	for _, want := range order {
		d.Key("tab")
		if f.focus != want {
			t.Fatalf("tab: focus = %d, want %d", f.focus, want)
		}
	}
	for _, want := range []focus{focusRef, focusButton, focusVars} {
		d.Key("shift+tab")
		if f.focus != want {
			t.Fatalf("shift+tab: focus = %d, want %d", f.focus, want)
		}
	}
	if buttonFocused(d) {
		t.Fatal("the button is not focused on the table")
	}
	d.Key("tab")
	if !buttonFocused(d) {
		t.Fatalf("the focused button must use the active style:\n%q", d.Model.View().Content)
	}
}

func TestTabDoesNothingWhileEditingOrPicking(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", gitlab.CIConfigVariable{Key: "DEPLOY_ENV", ValueOptions: []string{"staging", "prod-canary"}})
	d, _, f := open(t, c, "main", "")
	d.Key("tab", "a", "tab")
	if f.focus != focusVars || !f.vt.CapturesInput() {
		t.Fatalf("tab while editing a cell: focus = %d", f.focus)
	}
	d.Key("esc", "1", "tab")
	if f.focus != focusVars || f.picking == nil {
		t.Fatal("tab while the value picker is open must do nothing")
	}
	d.Key("esc", "s", "shift+tab")
	if f.focus != focusVars || !f.hintList {
		t.Fatal("shift+tab while the hints list is open must do nothing")
	}
}

func TestEnterRunsOnlyOnTheButton(t *testing.T) {
	c := newFake()
	d, _, _ := open(t, c, "main", "")
	d.Key("enter")               // ref: picks the suggestion
	d.Key("tab", "enter", "esc") // table: starts editing nothing (no rows), esc leaves
	if c.CallCount("Pipelines.Create") != 0 {
		t.Fatal("enter outside the button must not run")
	}
	d, root, _ := open(t, c, "main", "")
	d.Key("tab", "tab", "esc")
	if c.CallCount("Pipelines.Create") != 0 || root.Depth() != 1 {
		t.Fatal("esc on the button cancels")
	}
	d.RequireNoView("Run pipeline")
	d, root, _ = open(t, c, "main", "")
	d.Key("shift+tab", "enter")
	if c.CallCount("Pipelines.Create") != 1 || root.Depth() != 2 {
		t.Fatalf("enter on the button must run, creates = %d", c.CallCount("Pipelines.Create"))
	}
}

func manyHints() []gitlab.CIConfigVariable {
	hs := make([]gitlab.CIConfigVariable, 12)
	for i := range hs {
		hs[i] = gitlab.CIConfigVariable{Key: fmt.Sprintf("FLAG_%02d", i+1), Value: "on", Description: fmt.Sprintf("toggle %d", i+1)}
	}
	hs[11].ValueOptions = []string{"blue", "green"}
	return hs
}

func TestLongHintsLineIsCutWithShowAll(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", manyHints()...)
	d, _, _ := open(t, c, "main", "")
	d.RequireView("From .gitlab-ci.yml: 1 FLAG_01, 2 FLAG_02", "…  s all (12)")
	d.RequireNoView("FLAG_12")
}

func TestHintsListAddsAnyHint(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", manyHints()...)
	d, _, f := open(t, c, "main", "")
	d.Key("tab", "s")
	d.RequireView("› FLAG_01  toggle 1", "FLAG_08")
	d.RequireNoView("FLAG_09  toggle")
	for range 9 {
		d.Key("j")
	}
	d.RequireView("› FLAG_10  toggle 10")
	d.Key("enter")
	if v := f.Vars(); len(v) != 1 || v[0].Key != "FLAG_10" || v[0].Value != "on" {
		t.Fatalf("vars = %+v", v)
	}
	// A hint with options goes through the value picker.
	d.Key("s", "k", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "enter")
	d.RequireView("Value for FLAG_12")
	d.Key("j", "enter")
	if v := f.Vars(); len(v) != 2 || v[1].Key != "FLAG_12" || v[1].Value != "green" {
		t.Fatalf("vars = %+v", v)
	}
	d.Key("s", "esc")
	d.RequireNoView("toggle 1")
}

// ---- run on a tag ----

func tagWithPipeline() *fake.Client {
	c := newFake()
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91761, Ref: "v1.14.2", Status: gitlab.StatusSuccess}})
	return c
}

func TestTagWithPipelineAsksFirst(t *testing.T) {
	c := tagWithPipeline()
	d, root, f := open(t, c, "", "")
	typeText(d, "v1.14.2")
	d.Send(f.DebounceMsg())
	d.Key("tab", "tab", "enter")
	if cl := calls(c, "Pipelines.List"); len(cl) != 1 {
		t.Fatalf("pipelines check = %+v", cl)
	} else if fl, _ := cl[0].Args[1].(gitlab.PipelineFilter); fl.Ref != "v1.14.2" || fl.Scope != "tags" || cl[0].Args[3] != 1 {
		t.Fatalf("pipelines check args = %+v", cl[0].Args)
	}
	d.RequireView("Run another pipeline on tag v1.14.2?", "pipeline #91761 (success)", "build, publish, deploy")
	if c.CallCount("Pipelines.Create") != 0 {
		t.Fatal("must not create before the confirmation")
	}
	d.Key("enter") // "Cancel" is the default
	if c.CallCount("Pipelines.Create") != 0 {
		t.Fatal("cancel must not create")
	}
	d.RequireView("Run pipeline", "will be created: ref=v1.14.2")
	d.Key("enter", "y") // the focus stays on the button
	if cl := calls(c, "Pipelines.Create"); len(cl) != 1 || cl[0].Args[1] != "v1.14.2" {
		t.Fatalf("create = %+v", cl)
	}
	if root.Depth() != 2 {
		t.Fatalf("the new pipeline screen must be on top, depth = %d", root.Depth())
	}
}

func TestTagWithoutPipelineCreatesRightAway(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "", "")
	typeText(d, "v1.15.0")
	d.Send(f.DebounceMsg())
	d.Key("tab", "tab", "enter")
	if c.CallCount("Pipelines.List") != 1 || c.CallCount("Pipelines.Create") != 1 {
		t.Fatalf("list=%d create=%d", c.CallCount("Pipelines.List"), c.CallCount("Pipelines.Create"))
	}
	d.RequireNoView("Run another pipeline")
}

func TestBranchIsNotChecked(t *testing.T) {
	c := tagWithPipeline()
	d, _, _ := open(t, c, "main", "")
	d.Key("tab", "tab", "enter")
	if c.CallCount("Pipelines.List") != 0 || c.CallCount("Pipelines.Create") != 1 {
		t.Fatalf("list=%d create=%d", c.CallCount("Pipelines.List"), c.CallCount("Pipelines.Create"))
	}
}

func TestUnknownRefIsLookedUpAsTag(t *testing.T) {
	c := newFake()
	d, _, _ := open(t, c, "", "")
	// A tag the loaded suggestions do not know yet.
	c.SetTags(path, gitlab.Tag{Name: "v1.14.2"}, gitlab.Tag{Name: "v1.15.0"}, gitlab.Tag{Name: "v1.16.0"})
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91761, Ref: "v1.16.0", Status: gitlab.StatusFailed}})
	typeText(d, "v1.16.0") // run before the debounced search
	c.ResetCalls()
	d.Key("tab", "tab", "enter")
	if cl := calls(c, "Tags.List"); len(cl) != 1 || cl[0].Args[1] != "^v1.16.0$" {
		t.Fatalf("exact tag search = %+v", cl)
	}
	d.RequireView("Run another pipeline on tag v1.16.0?", "#91761 (failed)")

	// Not a tag and not a known branch: created without a pipelines check.
	c = newFake()
	d, _, _ = open(t, c, "", "")
	typeText(d, "feat/new")
	d.Key("tab", "tab", "enter")
	if c.CallCount("Pipelines.List") != 0 || c.CallCount("Pipelines.Create") != 1 {
		t.Fatalf("list=%d create=%d", c.CallCount("Pipelines.List"), c.CallCount("Pipelines.Create"))
	}
}

func TestTagCheckFailureAsksWithTheError(t *testing.T) {
	c := tagWithPipeline()
	c.FailNext("Pipelines.List", errors.New("boom"))
	d, _, f := open(t, c, "", "")
	typeText(d, "v1.14.2")
	d.Send(f.DebounceMsg())
	d.Key("tab", "tab", "enter")
	d.RequireView("Run another pipeline on tag v1.14.2?", "Could not check whether the tag already has a pipeline")
	d.Key("y")
	if c.CallCount("Pipelines.Create") != 1 {
		t.Fatal("yes must create")
	}
}

func TestRepeatedRunIgnoredWhileChecking(t *testing.T) {
	c := tagWithPipeline()
	d, _, f := open(t, c, "", "")
	typeText(d, "v1.14.2")
	d.Send(f.DebounceMsg())
	c.SetDelay("Pipelines.List", 200*time.Millisecond) // the check outlives the test driver's wait
	d.Key("tab", "tab", "enter", "enter", "enter")
	if n := c.CallCount("Pipelines.List"); n != 1 {
		t.Fatalf("checks = %d, repeated runs must be ignored", n)
	}
	if c.CallCount("Pipelines.Create") != 0 {
		t.Fatal("no create while checking")
	}
}

// ---- answers of a closed form, the run snapshot and the lock ----

// closedForm returns a form whose overlay is gone: its requests may still
// answer while another Run pipeline form is open.
func closedForm(t *testing.T, c *fake.Client) *Form {
	t.Helper()
	d, _, f := open(t, c, "main", "")
	d.Key("esc")
	return f
}

func TestAnswersOfAClosedFormAreDropped(t *testing.T) {
	c := newFake()
	old := closedForm(t, c)
	d, root, f := open(t, c, "main", "")
	c.ResetCalls()
	cands := names(f)
	d.Send(branchesMsg{tok: old.searchGen.Current(), names: []string{"stale-branch"}})
	d.Send(tagsMsg{tok: old.searchGen.Current(), names: []string{"stale-tag"}})
	if !slices.Equal(names(f), cands) || f.searching() {
		t.Fatalf("a closed form's search answers must be dropped: cands = %v, pending = %d", names(f), f.pending)
	}
	d.Send(hintsMsg{tok: old.hintGen.Current(), vars: []gitlab.CIConfigVariable{{Key: "STALE"}}})
	d.Send(searchTickMsg{tok: old.inputGen.Current()})
	if c.CallCount("Projects.Branches") != 0 {
		t.Fatal("a closed form's debounce tick must not search")
	}
	d.RequireNoView("stale-branch", "stale-tag", "STALE")

	// The new form is checking a tag when the old form's answer comes.
	c.SetDelay("Tags.List", 200*time.Millisecond)
	d.Key("shift+tab", "enter")
	if !f.checking {
		t.Fatal("the tag check must be running")
	}
	d.Send(tagCheckMsg{tok: old.runGen.Next(), run: launch{ref: "main"}})
	d.Send(confirmedMsg{tok: old.runGen.Current(), run: launch{ref: "main"}})
	if c.CallCount("Pipelines.Create") != 0 || !f.checking {
		t.Fatalf("a closed form's tag check must be dropped, creates = %d", c.CallCount("Pipelines.Create"))
	}
	d.Send(createdMsg{tok: old.runGen.Current(), p: gitlab.PipelineSummary{ID: 7}})
	if root.Depth() != 1 {
		t.Fatal("a closed form's created pipeline must not be opened")
	}
	d.RequireView("Run pipeline")
}

// createCall returns the only Pipelines.Create call: its ref and variables.
func createCall(t *testing.T, c *fake.Client) (string, []gitlab.CIVariable) {
	t.Helper()
	cl := calls(c, "Pipelines.Create")
	if len(cl) != 1 {
		t.Fatalf("creates = %+v", cl)
	}
	vars, _ := cl[0].Args[2].([]gitlab.CIVariable)
	return cl[0].Args[1].(string), vars
}

func TestRunUsesWhatEnterTook(t *testing.T) {
	// A branch: created right after the tag lookup.
	c := newFake()
	d, _, f := open(t, c, "main", "")
	f.vt.Set("A", "1")
	cmd := f.run()
	f.input.SetValue("feat/surge") // edits that would race with the check
	f.vt.Set("A", "2")
	f.vt.Set("B", "3")
	d.Run(cmd)
	if ref, vars := createCall(t, c); ref != "main" || len(vars) != 1 || vars[0] != (gitlab.CIVariable{Key: "A", Value: "1", Type: gitlab.VarEnv}) {
		t.Fatalf("create ref = %q, vars = %+v", ref, vars)
	}

	// A tag with a pipeline: the confirmation and the creation use it too.
	c = tagWithPipeline()
	d, _, f = open(t, c, "v1.14.2", "")
	f.vt.Set("A", "1")
	cmd = f.run()
	f.input.SetValue("main")
	f.vt.Set("A", "2")
	d.Run(cmd)
	d.RequireView("Run another pipeline on tag v1.14.2?")
	d.Key("y")
	if ref, vars := createCall(t, c); ref != "v1.14.2" || len(vars) != 1 || vars[0].Value != "1" {
		t.Fatalf("create ref = %q, vars = %+v", ref, vars)
	}
}

func TestFormIsLockedWhileChecking(t *testing.T) {
	c := newFake()
	d, _, f := open(t, c, "main", "")
	c.SetDelay("Tags.List", 200*time.Millisecond) // the check outlives the driver's wait
	d.Key("shift+tab", "enter")
	d.RequireView("checking tag… · esc cancel")
	d.Key("tab", "shift+tab", "x", "a", "enter")
	if f.focus != focusButton || f.Ref() != "main" || f.vt.Len() != 0 {
		t.Fatalf("locked form changed: focus = %d, ref = %q, rows = %d", f.focus, f.Ref(), f.vt.Len())
	}
	if n := c.CallCount("Tags.List"); n != 2 { // the first search and the check
		t.Fatalf("tag lookups = %d, a repeated enter must be ignored", n)
	}
	cancelled := f.runGen.Current()
	d.Key("esc")
	if f.checking {
		t.Fatal("esc must cancel the check")
	}
	d.RequireView("Run pipeline", "enter run · tab next · esc cancel")
	d.Send(tagCheckMsg{tok: cancelled, run: launch{ref: "main"}})
	if c.CallCount("Pipelines.Create") != 0 {
		t.Fatal("the answer of a cancelled check must be dropped")
	}
	d.Key("shift+tab")
	if f.focus != focusVars {
		t.Fatalf("the form must be editable again, focus = %d", f.focus)
	}
}

func TestFormIsLockedWhileCreating(t *testing.T) {
	c := newFake()
	d, root, f := open(t, c, "main", "")
	c.SetDelay("Pipelines.Create", 200*time.Millisecond)
	d.Key("shift+tab", "enter")
	if !f.creating {
		t.Fatal("the creation must be running")
	}
	d.RequireView("creating…")
	d.Key("esc", "tab", "enter")
	if !f.creating || f.focus != focusButton || c.CallCount("Pipelines.Create") != 1 {
		t.Fatalf("esc and keys must do nothing while creating: creates = %d", c.CallCount("Pipelines.Create"))
	}
	d.RequireView("Run pipeline")
	d.Send(createdMsg{tok: f.runGen.Current(), p: gitlab.PipelineSummary{ID: 7, Ref: "main"}})
	if root.Depth() != 2 {
		t.Fatalf("the answer opens the new pipeline, depth = %d", root.Depth())
	}
}

func TestBranchThatIsAlsoATagIsChecked(t *testing.T) {
	c := newFake()
	c.SetBranches(path, "main", "release")
	d, _, _ := open(t, c, "", "")
	d.RequireView("release")
	// The tag appears after the suggestions were loaded: they know only the branch.
	c.SetTags(path, gitlab.Tag{Name: "v1.14.2"}, gitlab.Tag{Name: "release"})
	c.AddPipeline(path, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 91761, Ref: "release", Status: gitlab.StatusSuccess}})
	typeText(d, "release")
	d.Key("tab", "tab", "enter")
	d.RequireView("Run another pipeline on tag release?")
	if c.CallCount("Pipelines.Create") != 0 {
		t.Fatal("a tag must be checked before the creation")
	}
}

func TestNewSuggestionsReplaceTheListAtOnce(t *testing.T) {
	d, _, f := open(t, newFake(), "", "")
	f.input.SetValue("feat")
	f.search() // the answers are delivered by hand below
	d.Send(branchesMsg{tok: f.searchGen.Current(), names: []string{"feat/new"}})
	d.RequireView("searching…", "fix/race", "v1.14.2")
	d.RequireNoView("feat/new")
	d.Send(tagsMsg{tok: f.searchGen.Current()})
	d.RequireView("feat/new", "branches 1 · tags 0 · 1/1")
	d.RequireNoView("v1.14.2", "fix/race", "searching…")
}
