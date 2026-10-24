package setup

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the setup screen: the
// URL step, the token step and the error fit 80×24 and 120×40 in en and ru.
func TestFitsTerminal(t *testing.T) {
	d, _, _, _ := newRoot(t, Options{Host: "https://gitlab.example.com"})
	d.RequireFits()
	d.Key("enter")
	d.RequireFits()
	typeText(d, "wrong")
	d.Key("enter")
	d.RequireFits() // the invalid token error

	d, _, _, _ = newRoot(t, Options{Host: "https://h.example.com", Err: &gitlab.Error{Kind: gitlab.ErrUnauthorized}})
	d.RequireFits()
}
