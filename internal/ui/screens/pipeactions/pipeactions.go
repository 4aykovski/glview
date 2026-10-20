// Package pipeactions holds the actions on a pipeline row shared by the
// project Pipelines tab and the workspace Pipelines tab: retry and cancel
// with a confirm modal, the log of the first failed (else running) job and
// the "run pipeline" form.
package pipeactions

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitremote"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/joblog"
	"github.com/4aykovski/glview/internal/ui/screens/runpipeline"
)

const requestTimeout = 15 * time.Second

type (
	// DoneMsg reports a finished retry or cancel of pipeline ID of Ref.
	DoneMsg struct {
		Ref   gitlab.ProjectRef
		Retry bool
		ID    int64
		Err   error
	}
	// LogJobMsg carries the job whose log l opens (nil: none to show).
	LogJobMsg struct {
		Ref  gitlab.ProjectRef
		Pipe gitlab.PipelineSummary
		Job  *gitlab.Job
		Err  error
	}
)

// Retry asks for confirmation and retries the failed jobs of p.
func Retry(env *app.Env, ref gitlab.ProjectRef, p gitlab.PipelineSummary) tea.Cmd {
	m := env.Msg.Pipe
	return app.OpenOverlay(app.NewConfirm(env, m.RetryTitle(p.ID), m.RetryBody, mutate(env, ref, p, true)))
}

// Cancel asks for confirmation and cancels p.
func Cancel(env *app.Env, ref gitlab.ProjectRef, p gitlab.PipelineSummary) tea.Cmd {
	m := env.Msg.Pipe
	return app.OpenOverlay(app.NewConfirm(env, m.CancelTitle(p.ID), m.CancelBody, mutate(env, ref, p, false)))
}

func mutate(env *app.Env, ref gitlab.ProjectRef, p gitlab.PipelineSummary, retry bool) tea.Cmd {
	client := env.Client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		var err error
		if retry {
			err = client.Pipelines().Retry(ctx, ref, p.ID)
		} else {
			err = client.Pipelines().Cancel(ctx, ref, p.ID)
		}
		return DoneMsg{Ref: ref, Retry: retry, ID: p.ID, Err: err}
	}
}

// Done drops the caches of the pipeline and the project's lists and
// returns the result toast. The caller reloads its list on success.
func Done(env *app.Env, msg DoneMsg) tea.Cmd {
	if env.Cache != nil {
		env.Cache.Invalidate("pipes:" + msg.Ref.FullPath + ":")
		env.Cache.Invalidate("stages:" + msg.Ref.FullPath + ":")
		env.Cache.Invalidate(fmt.Sprintf("pipe:%s:%d", msg.Ref.Key(), msg.ID))
	}
	if msg.Err != nil {
		return env.Fail(msg.Err)
	}
	toast := env.Msg.Pipe.ToastCanceled(msg.ID)
	if msg.Retry {
		toast = env.Msg.Pipe.ToastRetried(msg.ID)
	}
	return app.Toast(common.ToastSuccess, toast, "")
}

// FindLogJob looks for the first failed job of the pipeline, else the first
// running one; the job comes with its earlier attempts for the log. The
// pipeline is always read anew (the list and its status may be behind) and
// a final one is cached under the key of the Pipeline screen.
func FindLogJob(env *app.Env, ref gitlab.ProjectRef, p gitlab.PipelineSummary) tea.Cmd {
	key := fmt.Sprintf("pipe:%s:%d", ref.Key(), p.ID)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		pl, err := env.Client.Pipelines().Get(ctx, ref, p.ID)
		if err != nil {
			return LogJobMsg{Ref: ref, Pipe: p, Err: err}
		}
		if env.Cache != nil && pl.Status.Final() {
			env.Cache.Set(key, pl)
		}
		return LogJobMsg{Ref: ref, Pipe: pl.PipelineSummary, Job: PickLogJob(pl.Jobs)}
	}
}

// OpenLog opens the log of the found job, or tells there is none.
func OpenLog(env *app.Env, msg LogJobMsg) tea.Cmd {
	if msg.Err != nil {
		return env.Fail(msg.Err)
	}
	if msg.Job == nil {
		return app.Toast(common.ToastInfo, env.Msg.Pipe.NoLogJobs, "")
	}
	return app.Push(joblog.New(env, msg.Ref, msg.Pipe.ID, *msg.Job))
}

// PickLogJob returns the first failed job, else the first running one.
func PickLogJob(jobs []gitlab.Job) *gitlab.Job {
	for _, want := range []gitlab.PipelineStatus{gitlab.StatusFailed, gitlab.StatusRunning} {
		for i := range jobs {
			if jobs[i].Status == want {
				return &jobs[i]
			}
		}
	}
	return nil
}

// RunForm opens the "run pipeline" overlay for the project. With
// run_pipeline.prefill_local_branch the field holds the local branch (or
// the default branch without one), otherwise it starts empty.
func RunForm(env *app.Env, ref gitlab.ProjectRef, repo *gitremote.Repo, defaultBranch string) tea.Cmd {
	local, initial := "", ""
	if repo != nil && !repo.Detached && repo.Branch != "" {
		local = repo.Branch
	}
	if env.Settings.PrefillLocalBranch {
		initial = local
		if initial == "" {
			initial = defaultBranch
		}
	}
	f := runpipeline.New(env, ref, initial, local)
	return tea.Sequence(app.OpenOverlay(f), f.Init())
}
