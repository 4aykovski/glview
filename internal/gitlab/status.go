package gitlab

// PipelineStatus is a pipeline or job status as GitLab reports it. Statuses
// are never translated in the UI.
type PipelineStatus string

const (
	StatusCreated            PipelineStatus = "created"
	StatusWaitingForResource PipelineStatus = "waiting_for_resource"
	StatusPreparing          PipelineStatus = "preparing"
	StatusPending            PipelineStatus = "pending"
	StatusRunning            PipelineStatus = "running"
	StatusSuccess            PipelineStatus = "success"
	StatusFailed             PipelineStatus = "failed"
	StatusCanceled           PipelineStatus = "canceled"
	StatusCanceling          PipelineStatus = "canceling"
	StatusSkipped            PipelineStatus = "skipped"
	StatusManual             PipelineStatus = "manual"
	StatusScheduled          PipelineStatus = "scheduled"
)

// Active reports whether the status may still change soon (drives 3s polling).
func (s PipelineStatus) Active() bool {
	switch s {
	case StatusCreated, StatusWaitingForResource, StatusPreparing, StatusPending, StatusRunning, StatusCanceling:
		return true
	}
	return false
}

// Final reports whether the status will not change without user action.
func (s PipelineStatus) Final() bool {
	switch s {
	case StatusSuccess, StatusFailed, StatusCanceled, StatusSkipped:
		return true
	}
	return false
}

// FoldStatus reduces the statuses of a stage's jobs to one status
// (failed > running > pending > manual > canceled > success/skipped).
func FoldStatus(jobs []PipelineStatus) PipelineStatus {
	has := func(s PipelineStatus) bool {
		for _, j := range jobs {
			if j == s {
				return true
			}
		}
		return false
	}
	switch {
	case len(jobs) == 0:
		return StatusCreated
	case has(StatusFailed):
		return StatusFailed
	case has(StatusRunning):
		return StatusRunning
	case has(StatusPending), has(StatusPreparing), has(StatusWaitingForResource):
		return StatusPending
	case has(StatusManual):
		return StatusManual
	case has(StatusCanceled), has(StatusCanceling):
		return StatusCanceled
	case has(StatusCreated), has(StatusScheduled):
		return StatusCreated
	}
	for _, j := range jobs {
		if j != StatusSkipped {
			return StatusSuccess
		}
	}
	return StatusSkipped
}

// NewStage builds a stage summary from its jobs' statuses.
func NewStage(name string, jobs []PipelineStatus) StageSummary {
	st := StageSummary{Name: name, Status: FoldStatus(jobs), Total: len(jobs)}
	for _, j := range jobs {
		if j.Final() {
			st.Done++
		}
	}
	return st
}
