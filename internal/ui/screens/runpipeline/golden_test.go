package runpipeline

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestGoldenForm(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "feat/surge", gitlab.CIConfigVariable{Key: "DEPLOY_ENV", Value: "staging", ValueOptions: []string{"staging", "prod-canary"}}, gitlab.CIConfigVariable{Key: "SKIP_E2E"})
	d, _, f := open(t, c, "feat/surge", "feat/surge")
	f.vt.Add("DEPLOY_ENV", "staging", gitlab.VarEnv)
	d.RequireGoldenSizes("runpipeline")
}

func TestGoldenVariablesWithHintsList(t *testing.T) {
	c := newFake()
	c.SetCIVariables(path, "main", manyHints()...)
	d, _, _ := open(t, c, "main", "")
	d.Key("tab", "a")
	typeText(d, "TARGET")
	d.Key("enter", "esc", "s")
	d.RequireGoldenSizes("runpipeline-hints")
}

func TestGoldenTagConfirm(t *testing.T) {
	c := tagWithPipeline()
	d, _, f := open(t, c, "", "")
	typeText(d, "v1.14.2")
	d.Send(f.DebounceMsg())
	d.Key("ctrl+s")
	d.RequireGolden("runpipeline-tag-confirm")
}
