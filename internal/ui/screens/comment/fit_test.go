package comment

import (
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/i18n"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the comment overlay: an
// empty form, a long multi-line text and the send error fit 80×24 and
// 120×40 in en and ru.
func TestFitsTerminal(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		s := start(t, newFake(), lang)
		s.open(issueTarget)
		s.d.RequireFits()
		s.text(strings.Repeat("a long line of text without any breaks at all ", 6))
		s.d.RequireFits()
		for range 30 {
			s.d.Key("enter") // more lines than the overlay can show
		}
		s.d.RequireFits()
		s.d.Key("esc")

		s = start(t, newFake(), lang)
		s.open(mrTarget)
		s.d.RequireFits()
	}
}
