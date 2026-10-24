package gitlab

// Milestone is a project milestone as the New MR form needs it.
type Milestone struct {
	ID    int64
	Title string
}

// NewMR is a merge request to create (SPEC 3.18). Title is final: it
// already carries the "Draft: " prefix when one is needed.
type NewMR struct {
	SourceBranch, TargetBranch string
	Title, Description         string
	AssigneeIDs, ReviewerIDs   []int64
	Labels                     []string
	MilestoneID                int64 // 0 = no milestone
	RemoveSourceBranch, Squash bool
}
