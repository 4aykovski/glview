package newtag

import (
	"testing"

	"github.com/4aykovski/glview/internal/i18n"
)

// TestFitsTerminal is the SPEC §12.3 invariant for the New tag form: the
// version variants, the commit list, the hints and the confirmation fit
// 80×24 and 120×40 in en and ru.
func TestFitsTerminal(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		d, _, _ := open(t, newFake(), lang)
		d.RequireFits()
		d.Key("tab")
		d.RequireFits()
		d.Key("tab", "tab")
		d.RequireFits()
		typeText(d, "feat/s")
		d.RequireFits()
		d.Key("tab", "tab", "tab", "enter")
		d.RequireFits()
	}
}
