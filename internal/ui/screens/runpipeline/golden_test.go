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
