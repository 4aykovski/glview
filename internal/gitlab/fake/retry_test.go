package fake

import (
	"context"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestFakeRetryOfEarlierAttemptBecomesLatest(t *testing.T) {
	c := New()
	p := gitlab.ProjectRef{ID: 1, FullPath: "platform/route-service"}
	c.AddProject(gitlab.Project{ProjectRef: p})
	c.AddPipeline(p.FullPath, gitlab.Pipeline{PipelineSummary: gitlab.PipelineSummary{ID: 10}, Jobs: []gitlab.Job{
		{ID: 7, Name: "test", Stage: "test", Status: gitlab.StatusFailed, Attempts: []gitlab.Job{
			{ID: 5, Name: "test", Stage: "test", Status: gitlab.StatusFailed},
		}},
	}})
	ctx := context.Background()

	nj, err := c.Jobs().Retry(ctx, p, 5) // an earlier attempt
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.Jobs().Get(ctx, p, nj.ID); err != nil || got.Status != gitlab.StatusPending {
		t.Fatalf("the new job is stored: %+v %v", got, err)
	}
	pl, _ := c.Pipelines().Get(ctx, p, 10)
	if len(pl.Jobs) != 1 {
		t.Fatalf("jobs: %+v", pl.Jobs)
	}
	j := pl.Jobs[0]
	if j.ID != nj.ID || len(j.Attempts) != 2 || j.Attempts[0].ID != 7 || j.Attempts[1].ID != 5 || j.Attempts[0].Attempts != nil {
		t.Fatalf("latest %d, attempts %+v", j.ID, j.Attempts)
	}

	// Retrying the latest attempt keeps every earlier one.
	nj2, _ := c.Jobs().Retry(ctx, p, nj.ID)
	pl, _ = c.Pipelines().Get(ctx, p, 10)
	if j := pl.Jobs[0]; j.ID != nj2.ID || len(j.Attempts) != 3 || j.Attempts[0].ID != nj.ID {
		t.Fatalf("latest %d, attempts %+v", j.ID, j.Attempts)
	}
}
