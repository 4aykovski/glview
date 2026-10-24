package issuedetail

import (
	"testing"

	"github.com/4aykovski/glview/internal/i18n"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the issue screen: the
// threads, a folded thread, the help and the confirm dialog fit 80×24 and
// 120×40 in en and ru.
func TestFitsTerminal(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		s := open(t, newFake(), 1318, false, lang)
		s.d.RequireFits()
		s.d.Key("j", "space")
		s.d.RequireFits()
		s.d.Key("?")
		s.d.RequireFits()
		s.d.Key("esc", "x")
		s.d.RequireFits()
		s.d.Key("esc", "c")
		s.d.RequireFits()

		s = open(t, newFake(), 1300, true, lang) // no description, no comments, archived
		s.d.RequireFits()
	}
}
