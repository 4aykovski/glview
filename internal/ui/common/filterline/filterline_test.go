package filterline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

var mrKeys = []Key{
	{Name: "state", Values: []string{"opened", "merged", "closed", "all"}},
	{Name: "scope", Values: []string{"all", "created_by_me", "assigned_to_me"}},
	{Name: "reviewer"},
	{Name: "draft", Values: []string{"any", "yes", "no"}},
	{Name: "labels", Multi: true},
}

func TestParse(t *testing.T) {
	vals, free, err := Parse(mrKeys, "state:merged surge reviewer:@me pricing")
	if err != nil || vals["state"] != "merged" || vals["reviewer"] != "@me" || free != "surge pricing" || len(vals) != 2 {
		t.Fatalf("%v %q %v", vals, free, err)
	}
	if vals, free, err := Parse(mrKeys, ""); err != nil || len(vals) != 0 || free != "" {
		t.Fatalf("empty: %v %q %v", vals, free, err)
	}
	if vals, _, _ := Parse(mrKeys, "labels:bug,backend"); vals["labels"] != "bug,backend" {
		t.Fatalf("labels: %v", vals)
	}
	if vals, _, _ := Parse(mrKeys, "state:opened state:closed"); vals["state"] != "closed" {
		t.Fatalf("last duplicate wins: %v", vals)
	}
	// A token with a colon and an unknown key is a typo, not text.
	if _, free, err := Parse(mrKeys, "http://x"); err == nil {
		t.Fatalf("a token with an unknown key must be rejected, got free=%q", free)
	}
}

func TestParseErrors(t *testing.T) {
	var pe *ParseError
	_, _, err := Parse(mrKeys, "foo:bar")
	if !errors.As(err, &pe) || pe.Kind != UnknownKey || pe.Key != "foo" {
		t.Fatalf("unknown key: %v", err)
	}
	_, _, err = Parse(mrKeys, "state:bogus")
	if !errors.As(err, &pe) || pe.Kind != BadValue || pe.Key != "state" || pe.Value != "bogus" || len(pe.Allowed) != 4 {
		t.Fatalf("bad value: %v", err)
	}
	if _, _, err = Parse(mrKeys, "state:"); err == nil {
		t.Fatal("empty value must be rejected")
	}
}

func TestFormatRoundTrip(t *testing.T) {
	in := "state:merged reviewer:@me surge"
	vals, free, _ := Parse(mrKeys, in)
	if got := Format(mrKeys, vals, free); got != in {
		t.Fatalf("got %q", got)
	}
}

func TestComplete(t *testing.T) {
	cases := map[string]string{
		"st":                 "state:",
		"sc":                 "scope:",
		"s":                  "s", // state and scope: no common extension beyond "s"
		"state:me":           "state:merged ",
		"state:":             "state:", // nothing typed yet: ambiguous, common prefix is empty
		"state:o":            "state:opened ",
		"state:merged re":    "state:merged reviewer:",
		"scope:a":            "scope:a", // all, assigned_to_me: only the common prefix
		"draft:y":            "draft:yes ",
		"foo":                "foo",
		"":                   "",
		"labels:bug re":      "labels:bug reviewer:",
		"reviewer:":          "reviewer:",
		"state:merged surge": "state:merged surge",
	}
	for in, want := range cases {
		if got := Complete(mrKeys, in); got != want {
			t.Errorf("Complete(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Complete(mrKeys, "scope:as"); got != "scope:assigned_to_me " {
		t.Errorf("got %q", got)
	}
}

func TestCompleteListElement(t *testing.T) {
	keys := []Key{{Name: "labels", Multi: true, Values: []string{"bug", "backend", "docs"}}}
	if got := Complete(keys, "labels:bug,d"); got != "labels:bug,docs " {
		t.Errorf("got %q", got)
	}
	if got := Complete(keys, "labels:b"); got != "labels:b" { // bug, backend: common prefix "b"
		t.Errorf("got %q", got)
	}
}

func TestSuggest(t *testing.T) {
	got := Suggest(mrKeys, "state:merged s")
	if len(got) != 2 || got[0] != "state:" || got[1] != "scope:" {
		t.Fatalf("got %v", got)
	}
}

func TestWidgetKeys(t *testing.T) {
	m := New(common.NewStyles(common.TerminalTheme(true)), mrKeys)
	m.Open("state:opened")
	if !m.Focused() {
		t.Fatal("must be focused after Open")
	}
	// Typing q does not leave the widget.
	if res, _ := m.Update(uitest.KeyPress("q")); res != Editing || m.Value() != "state:openedq" {
		t.Fatalf("typing: %v %q", res, m.Value())
	}
	if res, _ := m.Update(uitest.KeyPress("enter")); res != Editing || m.Err() == nil || !m.Focused() {
		t.Fatalf("bad input must stay open with an error: %v %v", res, m.Err())
	}
	m.Open("sta")
	m.Update(uitest.KeyPress("tab"))
	if m.Value() != "state:" {
		t.Fatalf("tab: %q", m.Value())
	}
	m.Update(uitest.KeyPress("m"))
	m.Update(uitest.KeyPress("tab"))
	if m.Value() != "state:merged " {
		t.Fatalf("tab value: %q", m.Value())
	}
	if res, _ := m.Update(uitest.KeyPress("enter")); res != Applied || m.Focused() {
		t.Fatalf("enter: %v", res)
	}
	vals, _, err := m.Parsed()
	if err != nil || vals["state"] != "merged" {
		t.Fatalf("%v %v", vals, err)
	}
	m.Open("x")
	if res, _ := m.Update(uitest.KeyPress("esc")); res != Cancelled || m.Focused() {
		t.Fatalf("esc: %v", res)
	}
}

func newSuggestModel(src func(ctx context.Context, prefix string) ([]Suggestion, error)) *Model {
	keys := []Key{
		{Name: "state", Desc: "MR state", Values: []string{"opened", "merged", "closed", "all"}},
		{Name: "scope", Desc: "whose MRs", Values: []string{"all", "created_by_me", "assigned_to_me"}},
		{Name: "reviewer", Desc: "reviewer", Values: []string{"@me", "any"}, Source: src},
		{Name: "labels", Desc: "labels", Multi: true, Source: src},
	}
	m := New(common.NewStyles(common.TerminalTheme(true)), keys)
	m.SetCurrent(map[string]string{"state": "opened"})
	return m
}

func typeKeys(m *Model, s string) {
	for _, r := range s {
		m.Update(uitest.KeyPress(string(r)))
	}
}

func values(m *Model) []string {
	var out []string
	for _, it := range m.Items() {
		out = append(out, it.Value)
	}
	return out
}

func TestSuggestKeysWithDescriptionAndCurrentValue(t *testing.T) {
	m := newSuggestModel(nil)
	m.Open("")
	if got := values(m); len(got) != 4 || got[0] != "state:" {
		t.Fatalf("items = %v", got)
	}
	if note := m.Items()[0].Note; note != "MR state  opened" {
		t.Fatalf("note = %q", note)
	}
	if !strings.Contains(m.View(), "MR state  opened") {
		t.Fatalf("view lacks the note:\n%s", m.View())
	}
}

func TestSuggestValuesDownTabAndEnter(t *testing.T) {
	m := newSuggestModel(nil)
	m.Open("")
	typeKeys(m, "state:")
	if got := values(m); strings.Join(got, ",") != "opened,merged,closed,all" {
		t.Fatalf("values = %v", got)
	}
	m.Update(uitest.KeyPress("down"))
	m.Update(uitest.KeyPress("tab"))
	if m.Value() != "state:merged " {
		t.Fatalf("value = %q", m.Value())
	}
	// With the list open, enter completes instead of applying.
	m.Open("")
	typeKeys(m, "sta")
	if res, _ := m.Update(uitest.KeyPress("enter")); res != Editing || m.Value() != "state:" {
		t.Fatalf("enter must complete: %v %q", res, m.Value())
	}
	// A fully typed value closes the list: enter applies.
	m.Open("state:merged")
	if res, _ := m.Update(uitest.KeyPress("enter")); res != Applied {
		t.Fatalf("enter must apply: %v", res)
	}
}

func TestSuggestPrefixMatchesFirst(t *testing.T) {
	m := newSuggestModel(nil)
	m.Open("scope:e") // "created_by_me" contains e after c; no prefix match
	m.Open("scope:a")
	if got := values(m); got[0] != "all" || got[1] != "assigned_to_me" {
		t.Fatalf("prefix matches first: %v", got)
	}
	m.Open("scope:me")
	if got := values(m); len(got) != 2 || got[0] != "created_by_me" {
		t.Fatalf("substring matches follow: %v", got)
	}
}

func TestSourceDebouncedStaleAndErrors(t *testing.T) {
	var calls []string
	m := newSuggestModel(func(ctx context.Context, prefix string) ([]Suggestion, error) {
		calls = append(calls, prefix)
		return []Suggestion{{Value: "anna.k", Note: "Anna"}, {Value: "bob", Note: "Bob"}}, nil
	})
	m.Open("")
	typeKeys(m, "reviewer:an")
	// static items are there at once, nothing fetched before the pause
	if got := values(m); len(got) != 1 || got[0] != "any" {
		t.Fatalf("static items for 'an': %v", got)
	}
	if len(calls) != 0 {
		t.Fatal("typing alone must not call the source")
	}
	stale := m.DebounceMsg()
	typeKeys(m, "n")
	if cmd := m.HandleMsg(stale); cmd != nil {
		t.Fatal("a stale debounce tick must be ignored")
	}
	cmd := m.HandleMsg(m.DebounceMsg())
	if cmd == nil {
		t.Fatal("the last tick must fetch")
	}
	msg := cmd().(SuggestionsMsg)
	if len(calls) != 1 || calls[0] != "ann" {
		t.Fatalf("calls = %v", calls)
	}
	// An answer for an older token is dropped.
	m.HandleMsg(SuggestionsMsg{Token: msg.Token - 1, Items: []Suggestion{{Value: "zzz"}}})
	if len(m.Items()) != 0 {
		t.Fatal("stale answer must not show")
	}
	m.HandleMsg(msg)
	if got := values(m); len(got) != 1 || got[0] != "anna.k" {
		t.Fatalf("answer filtered by prefix: %v", got)
	}
	// An error neither shows nor breaks typing.
	m.HandleMsg(SuggestionsMsg{Token: m.tok, Err: errors.New("boom")})
	typeKeys(m, "x")
	if m.Value() != "reviewer:annx" || m.Err() != nil {
		t.Fatalf("typing after an error: %q %v", m.Value(), m.Err())
	}
}

func TestSourceMergedWithStaticAndMulti(t *testing.T) {
	m := newSuggestModel(func(ctx context.Context, prefix string) ([]Suggestion, error) {
		return []Suggestion{{Value: "any"}, {Value: "anna.k"}}, nil
	})
	m.Open("reviewer:a")
	m.HandleMsg(m.fetch()())
	if got := values(m); strings.Join(got, ",") != "any,anna.k" {
		t.Fatalf("static first, no duplicates: %v", got)
	}
	m.Open("labels:bug,ba")
	m.HandleMsg(m.fetch()())
	m.Items() // list is for the last element only
	m.Update(uitest.KeyPress("tab"))
	if !strings.HasPrefix(m.Value(), "labels:bug,") {
		t.Fatalf("tab must replace only the last list element: %q", m.Value())
	}
}

func TestHeightAndEscClearsList(t *testing.T) {
	m := newSuggestModel(nil)
	m.Open("")
	if m.Height() != 5 {
		t.Fatalf("height = %d", m.Height())
	}
	m.Update(uitest.KeyPress("esc"))
	if m.Focused() || len(m.Items()) != 0 {
		t.Fatal("esc closes everything")
	}
}
