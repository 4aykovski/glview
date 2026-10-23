package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
)

func keyPress(s string) tea.KeyPressMsg {
	if s == "enter" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	if s == "esc" {
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func TestLinksOverlayOpenAndCopy(t *testing.T) {
	env := NewEnv(i18n.EN)
	var opened []string
	env.Browser = func(u string) error { opened = append(opened, u); return nil }
	links := []gitlab.Link{
		{Text: "ABC-123", URL: "https://tracker.example.com/browse/ABC-123"},
		{Text: "#12", URL: "https://gitlab.example.com/platform/route-service/-/issues/12"},
	}
	o := NewLinksOverlay(env, "k", "!479", links)
	next, _ := o.Update(keyPress("j"))
	next, cmd := next.Update(keyPress("enter"))
	if next != nil || cmd == nil {
		t.Fatalf("enter must close and open: %v %v", next, cmd)
	}
	cmd()
	if len(opened) != 1 || opened[0] != links[1].URL {
		t.Fatalf("opened = %v", opened)
	}

	o = NewLinksOverlay(env, "k", "!479", links)
	next, cmd = o.Update(keyPress("y"))
	if next != nil || cmd == nil {
		t.Fatalf("y must close and copy")
	}
	if !strings.Contains(o.View(100, 30), "Links · !479") {
		t.Errorf("title missing")
	}
}

func TestLinksOverlayRefreshKeepsSelection(t *testing.T) {
	env := NewEnv(i18n.EN)
	a := gitlab.Link{Text: "a", URL: "https://x.example.com/a"}
	b := gitlab.Link{Text: "b", URL: "https://x.example.com/b"}
	o := NewLinksOverlay(env, "k", "!1", []gitlab.Link{a, b})
	o.Update(keyPress("j"))
	o.Update(LinksRefreshMsg{Key: "other", Links: []gitlab.Link{a}})
	o.Update(LinksRefreshMsg{Key: "k", Links: []gitlab.Link{{Text: "n", URL: "https://x.example.com/n"}, b}})
	if l, _ := o.modal.Selected(); l.Text != "b" {
		t.Fatalf("selected = %+v", l)
	}
	if next, _ := o.Update(keyPress("esc")); next != nil {
		t.Fatal("esc closes")
	}
}
