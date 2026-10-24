package newmr

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
)

// TestFitsLongContent is the SPEC §12.3 invariant for the form with long
// values: chips, an error line, suggestions, the confirmations and a
// description taller than the third of the screen fit 80×24 and 120×40 in
// en and ru.
func TestFitsLongContent(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		c := newFake()
		c.AddMR(path, gitlab.MergeRequest{IID: 482, Title: "Fix the slot allocator race that shows up under load in the courier pool", State: gitlab.MRStateOpened, SourceBranch: "feat/surge", TargetBranch: "main"})
		o := open(t, c, lang, "feat/surge")
		o.d.RequireFitsSizes()

		o.focusOn(fTitle)
		o.typeText(" and a very long tail of the title that does not fit the value column of the form at eighty columns")
		o.d.RequireFitsSizes()

		o.focusOn(fDesc)
		o.typeText("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve")
		o.d.RequireFitsSizes()

		for _, id := range []fieldID{fAssignees, fReviewers, fLabels} {
			o.focusOn(id)
			o.d.Key("enter", "enter")
			o.d.RequireFitsSizes()
		}
		o.focusOn(fDraft)
		o.d.Key("space")
		o.focusOn(fTemplate)
		o.d.Key("down", "enter") // asks to replace the typed description
		o.d.RequireFitsSizes()
		o.d.Key("esc")

		o.focusOn(fButton)
		o.d.RequireFitsSizes()
		o.d.Key("enter") // the open MR: confirm
		o.d.RequireFitsSizes()
		o.d.Key("n")

		o.setBranches("", "main")
		o.d.Key("enter")
		o.d.RequireFitsSizes()
	}
}
