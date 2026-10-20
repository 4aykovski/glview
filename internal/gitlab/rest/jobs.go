package rest

import (
	"context"
	"io"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
)

type jobs struct{ c *Client }

func (s jobs) Get(ctx context.Context, p gitlab.ProjectRef, id int64) (gitlab.Job, error) {
	j, _, err := s.c.api.Jobs.GetJob(pid(p), id, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Job{}, mapError(err)
	}
	return mapJob(j), nil
}

// Trace returns the whole log of the job.
func (s jobs) Trace(ctx context.Context, p gitlab.ProjectRef, id int64) ([]byte, error) {
	r, _, err := s.c.api.Jobs.GetTraceFile(pid(p), id, gl.WithContext(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	return io.ReadAll(r)
}

func (s jobs) Retry(ctx context.Context, p gitlab.ProjectRef, id int64) (gitlab.Job, error) {
	j, _, err := s.c.api.Jobs.RetryJob(pid(p), id, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Job{}, mapError(err)
	}
	return mapJob(j), nil
}

func (s jobs) Cancel(ctx context.Context, p gitlab.ProjectRef, id int64) error {
	_, _, err := s.c.api.Jobs.CancelJob(pid(p), id, gl.WithContext(ctx))
	return mapError(err)
}

func (s jobs) Play(ctx context.Context, p gitlab.ProjectRef, id int64, vars []gitlab.CIVariable) (gitlab.Job, error) {
	opt := &gl.PlayJobOptions{}
	if len(vars) > 0 {
		list := make([]*gl.JobVariableOptions, len(vars))
		for i, v := range vars {
			list[i] = &gl.JobVariableOptions{Key: gl.Ptr(v.Key), Value: gl.Ptr(v.Value), VariableType: variableType(v.Type)}
		}
		opt.JobVariablesAttributes = &list
	}
	j, _, err := s.c.api.Jobs.PlayJob(pid(p), id, opt, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Job{}, mapError(err)
	}
	return mapJob(j), nil
}
