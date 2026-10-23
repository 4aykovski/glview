package newtag

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
)

func TestGoldenForm(t *testing.T) {
	c := newFake()
	head := commit("0b7d3e9", "Merge branch 'feat/pprof'")
	c.SetCompare(path, "v1.14.2", "main", gitlab.Comparison{Head: &head, Commits: []gitlab.CommitSummary{
		head, commit("5c0e7d1", "slots: retry on serialization failure"), commit("e41ab08", "pprof: expose on admin port"), commit("77d0c13", "deps: bump grpc")}})
	d, _, _ := open(t, c, i18n.EN)
	d.RequireGoldenSizes("newtag")
}

func TestGoldenFormRefHints(t *testing.T) {
	d, _, f := open(t, newFake(), i18n.EN)
	d.Key("tab", "backspace", "backspace", "backspace", "backspace")
	typeText(d, "f")
	d.Send(f.DebounceMsg())
	d.RequireGoldenSizes("newtag-hints")
}

func TestGoldenFormConfirm(t *testing.T) {
	d, _, _ := open(t, newFake(), i18n.RU)
	d.Key("tab", "tab", "tab", "enter")
	d.RequireGolden("newtag-confirm-ru")
}
