package threads

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
)

func newModel() *Model {
	env := app.NewEnv(i18n.EN)
	env.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	return New(env, func(d gitlab.Discussion) string {
		return fmt.Sprintf("https://gitlab.example.com/x#note_%d", d.Notes[0].ID)
	})
}

func note(id int64, user, body string) gitlab.Note {
	return gitlab.Note{ID: id, Author: gitlab.User{Username: user}, Body: body, CreatedAt: time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)}
}

func thread(id string, notes ...gitlab.Note) gitlab.Discussion {
	return gitlab.Discussion{ID: id, Notes: notes}
}

func longBody() string {
	return strings.Repeat("line\n\n", 30)
}

func TestSystemNotesDropped(t *testing.T) {
	m := newModel()
	sys := note(1, "egor", "assigned to @anna.k")
	sys.System = true
	m.SetDiscussions([]gitlab.Discussion{thread("s", sys), thread("a", note(2, "anna.k", "hi"))})
	if m.Count() != 1 {
		t.Fatalf("count = %d", m.Count())
	}
}

func TestFoldedByDefaultRules(t *testing.T) {
	m := newModel()
	resolved := thread("r", note(1, "anna.k", "done"))
	resolved.Resolvable, resolved.Resolved = true, true
	m.SetDiscussions([]gitlab.Discussion{
		resolved,
		thread("long", note(2, "anna.k", longBody())),
		thread("short", note(3, "egor", "short one"), note(4, "anna.k", "reply")),
	})
	lines, cards := m.Layout(100)
	if len(cards) != 3 {
		t.Fatalf("cards = %d", len(cards))
	}
	if cards[0].H != 3 || cards[1].H != 3 {
		t.Fatalf("resolved and long threads must be folded: %+v", cards)
	}
	if cards[2].H <= 3 {
		t.Fatalf("short thread must be expanded: %+v", cards)
	}
	if !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "short one") {
		t.Fatal("expanded text missing")
	}
	if cards[2].Link != "https://gitlab.example.com/x#note_3" {
		t.Fatalf("link = %q", cards[2].Link)
	}
}

func TestNextPrev(t *testing.T) {
	m := newModel()
	m.SetDiscussions([]gitlab.Discussion{thread("a", note(1, "x", "one")), thread("b", note(2, "y", "two")), thread("c", note(3, "z", "three"))})
	_, cards := m.Layout(100)
	if c, _ := m.Current(); c.ID != "a" {
		t.Fatalf("first current = %q", c.ID)
	}
	if got := m.Next(); got != cards[1].Start {
		t.Fatalf("Next returned %d, want %d", got, cards[1].Start)
	}
	m.Next()
	m.Next() // stays on the last one
	if c, _ := m.Current(); c.ID != "c" {
		t.Fatalf("current = %q", c.ID)
	}
	m.Prev()
	m.Prev()
	m.Prev()
	if c, _ := m.Current(); c.ID != "a" {
		t.Fatalf("current = %q", c.ID)
	}
	m.Repick(cards[2].Start)
	if c, _ := m.Current(); c.ID != "c" {
		t.Fatalf("repick: current = %q", c.ID)
	}
	m.Repick(1000)
	if c, _ := m.Current(); c.ID != "c" {
		t.Fatalf("repick past the end: current = %q", c.ID)
	}
}

func TestFoldAndCurrentSurviveSetDiscussions(t *testing.T) {
	m := newModel()
	ds := []gitlab.Discussion{thread("a", note(1, "x", "one")), thread("b", note(2, "y", "two"), note(3, "x", "reply"))}
	m.SetDiscussions(ds)
	m.Layout(100)
	m.Next()
	before, _ := m.Current()
	if before.H <= 3 {
		t.Fatalf("thread b should be expanded: %+v", before)
	}
	m.ToggleCurrent()
	if c, _ := m.Current(); c.ID != "b" || c.H != 3 {
		t.Fatalf("toggle must fold b: %+v", c)
	}
	// A reload brings a new reply; the fold and the current thread stay.
	ds[1].Notes = append(ds[1].Notes, note(4, "y", "one more"))
	m.SetDiscussions(ds)
	if c, _ := m.Current(); c.ID != "b" || c.H != 3 {
		t.Fatalf("after SetDiscussions: %+v", c)
	}
	m.ToggleCurrent()
	if c, _ := m.Current(); c.H <= 3 {
		t.Fatalf("second toggle must unfold: %+v", c)
	}
}

func TestEmpty(t *testing.T) {
	m := newModel()
	if _, ok := m.Current(); ok {
		t.Fatal("no current thread expected")
	}
	if m.Next() != 0 || m.ToggleCurrent() != 0 {
		t.Fatal("empty model must not move")
	}
	m.Repick(5)
	lines, cards := m.Layout(80)
	if len(lines) != 0 || len(cards) != 0 {
		t.Fatal("empty layout")
	}
}
