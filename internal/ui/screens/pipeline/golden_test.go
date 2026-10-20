package pipeline

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/uitest"
)

func TestGoldenPipeline(t *testing.T) {
	d, _, _ := open(t, fixture())
	d.Key("l") // the test stage is active
	d.RequireGoldenSizes("pipeline")
}

func TestGoldenPlayForm(t *testing.T) {
	d, _, _ := open(t, fixture())
	d.Key("l", "l", "P", "a")
	for _, r := range "TARGET" {
		d.Key(string(r))
	}
	d.Key("enter")
	for _, r := range "canary" {
		d.Key(string(r))
	}
	d.Key("enter")
	d.RequireGoldenSizes("pipeline-play")
}

func TestGoldenPlayFormButton(t *testing.T) {
	d, _, _ := open(t, fixture())
	d.Key("l", "l", "P", "tab")
	d.RequireGolden("pipeline-play-button")
}

// TestGoldenPipelineStretched: two stages share a wide screen and a long job
// name fits whole.
func TestGoldenPipelineStretched(t *testing.T) {
	c := fake.New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: 42, FullPath: path}})
	c.AddPipeline(path, gitlab.Pipeline{
		PipelineSummary: gitlab.PipelineSummary{ID: 91850, IID: 513, Status: gitlab.StatusRunning, Ref: "main", SHA: "a91f2c0d4e",
			CommitTitle: "Bump grpc to 1.64", Source: "push", User: &gitlab.User{Username: "anna.k"}, Duration: 90 * time.Second},
		Jobs: []gitlab.Job{
			{ID: 1, Name: "build", Stage: "build", Status: gitlab.StatusSuccess, Duration: 60 * time.Second},
			{ID: 2, Name: "test:integration:postgres:with-long-name", Stage: "test", Status: gitlab.StatusRunning},
		},
	})
	env := app.NewEnv(i18n.EN)
	env.Client, env.User, env.Host = c, c.User, "https://gitlab.example.com"
	env.Now = func() time.Time { return now }
	scr := New(env, gitlab.ProjectRef{ID: 42, FullPath: path}, 91850, nil)
	d := uitest.New(t, app.New(env, scr, app.Options{}))
	d.Send(tea.WindowSizeMsg{Width: 160, Height: 40})
	d.RequireView("test:integration:postgres:with-long-name")
	d.RequireGolden("pipeline-stretched-160x40")
}

func TestGoldenAttempts(t *testing.T) {
	d, _, _ := open(t, fixture())
	d.Key("l", "a")
	d.RequireGoldenSizes("pipeline-attempts")
}
