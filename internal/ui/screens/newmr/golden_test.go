package newmr

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
)

func TestGoldenFocusTop(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		o := open(t, newFake(), lang, "feat/surge")
		o.d.RequireGoldenSizes("newmr-top-" + string(lang))
	}
}

func TestGoldenFocusMiddle(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		o := open(t, newFake(), lang, "feat/surge")
		o.focusOn(fAssignees)
		o.d.RequireGoldenSizes("newmr-middle-" + string(lang))
	}
}

func TestGoldenFocusBottom(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.RU} {
		c := newFake()
		c.AddMR(path, gitlab.MergeRequest{IID: 482, Title: "Fix slot race", State: gitlab.MRStateOpened, SourceBranch: "feat/surge", TargetBranch: "main"})
		o := open(t, c, lang, "feat/surge")
		o.focusOn(fDraft)
		o.d.Key("space")
		o.focusOn(fReviewers)
		o.d.Key("enter")
		o.focusOn(fLabels)
		o.d.Key("enter")
		o.focusOn(fButton)
		o.d.RequireGoldenSizes("newmr-bottom-" + string(lang))
	}
}

func TestGoldenValidationError(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/slots")
	o.focusOn(fButton)
	o.d.Key("enter")
	o.d.RequireGoldenSizes("newmr-error-en")
}

func TestGoldenBranchSuggestions(t *testing.T) {
	o := open(t, newFake(), i18n.RU, "feat/surge")
	o.d.Key("ctrl+u")
	o.typeText("feat/")
	o.tick()
	o.d.RequireGoldenSizes("newmr-hints-ru")
}

func TestGoldenTemplateList(t *testing.T) {
	o := open(t, newFake(), i18n.EN, "feat/surge")
	o.focusOn(fTemplate)
	o.d.RequireGoldenSizes("newmr-template-en")
}
