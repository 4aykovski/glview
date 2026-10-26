package graphql

import (
	"strconv"
	"strings"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Shared response shapes and their mapping to domain types.

type gqlUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	WebURL   string `json:"webUrl"`
}

type gqlUsers struct {
	Nodes []gqlUser `json:"nodes"`
}

func (u gqlUser) domain() gitlab.User {
	return gitlab.User{Username: u.Username, Name: u.Name, WebURL: u.WebURL}
}

func (u gqlUsers) domain() []gitlab.User {
	out := make([]gitlab.User, len(u.Nodes))
	for i, n := range u.Nodes {
		out[i] = n.domain()
	}
	return out
}

type gqlPipeline struct {
	ID         string  `json:"id"`
	IID        string  `json:"iid"`
	Status     string  `json:"status"`
	Ref        string  `json:"ref"`
	SHA        string  `json:"sha"`
	Source     string  `json:"source"`
	Duration   *int64  `json:"duration"`
	CreatedAt  string  `json:"createdAt"`
	FinishedAt *string `json:"finishedAt"`
	Path       string  `json:"path"`
	User       *struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"user"`
	Commit *struct {
		Title string `json:"title"`
	} `json:"commit"`
	Stages *struct {
		Nodes []gqlStage `json:"nodes"`
	} `json:"stages"`
}

type gqlStage struct {
	Name           string `json:"name"`
	DetailedStatus *struct {
		Group string `json:"group"`
	} `json:"detailedStatus"`
	Jobs struct {
		Nodes []struct {
			Status   string `json:"status"`
			Duration *int64 `json:"duration"`
		} `json:"nodes"`
	} `json:"jobs"`
}

// pipelineSummary maps a pipeline node; origin is scheme://host used to
// build the web URL from the relative path GitLab returns.
func (p gqlPipeline) summary(project gitlab.ProjectRef, origin string) gitlab.PipelineSummary {
	s := gitlab.PipelineSummary{
		Project:   project,
		ID:        gidOrZero(p.ID),
		IID:       parseInt(p.IID),
		Status:    gitlab.PipelineStatus(lower(p.Status)),
		Ref:       p.Ref,
		SHA:       p.SHA,
		Source:    p.Source,
		CreatedAt: parseTime(p.CreatedAt),
	}
	if p.Duration != nil {
		s.Duration = time.Duration(*p.Duration) * time.Second
	}
	if p.FinishedAt != nil {
		if t := parseTime(*p.FinishedAt); !t.IsZero() {
			s.FinishedAt = &t
		}
	}
	if p.Path != "" {
		s.WebURL = origin + p.Path
	}
	if p.User != nil {
		s.User = &gitlab.User{Username: p.User.Username, Name: p.User.Name}
	}
	if p.Commit != nil {
		s.CommitTitle = p.Commit.Title
	}
	if p.Stages != nil {
		s.Stages = stagesOf(p.Stages.Nodes)
	}
	return s
}

// stagesOf derives per-stage progress from the jobs' statuses. The stage's
// own detailedStatus group wins when GitLab returned one.
func stagesOf(nodes []gqlStage) []gitlab.StageSummary {
	out := make([]gitlab.StageSummary, len(nodes))
	for i, n := range nodes {
		statuses := make([]gitlab.PipelineStatus, len(n.Jobs.Nodes))
		var total time.Duration
		for j, job := range n.Jobs.Nodes {
			statuses[j] = gitlab.PipelineStatus(lower(job.Status))
			if job.Duration != nil {
				total += time.Duration(*job.Duration) * time.Second
			}
		}
		st := gitlab.NewStage(n.Name, statuses)
		st.Duration = total
		if n.DetailedStatus != nil && n.DetailedStatus.Group != "" {
			g := lower(n.DetailedStatus.Group)
			if strings.HasPrefix(g, "success") {
				g = "success" // success-with-warnings
			}
			st.Status = gitlab.PipelineStatus(strings.ReplaceAll(g, "-", "_"))
		}
		out[i] = st
	}
	return out
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// gqlReviewers is the reviewers selection; details add the per-user merge
// request interaction.
type gqlReviewers struct {
	Nodes []struct {
		Username                string `json:"username"`
		Name                    string `json:"name"`
		WebURL                  string `json:"webUrl"`
		MergeRequestInteraction *struct {
			Approved    bool   `json:"approved"`
			ReviewState string `json:"reviewState"`
		} `json:"mergeRequestInteraction"`
	} `json:"nodes"`
}

func (r gqlReviewers) users() []gitlab.User {
	out := make([]gitlab.User, len(r.Nodes))
	for i, n := range r.Nodes {
		out[i] = gitlab.User{Username: n.Username, Name: n.Name, WebURL: n.WebURL}
	}
	return out
}

// states maps reviewers to review states; no interaction means no entry.
func (r gqlReviewers) states() map[string]gitlab.ReviewState {
	var out map[string]gitlab.ReviewState
	for _, n := range r.Nodes {
		i := n.MergeRequestInteraction
		if i == nil {
			continue
		}
		st := gitlab.ReviewState(lower(i.ReviewState))
		if i.Approved {
			st = gitlab.ReviewApproved
		}
		if out == nil {
			out = map[string]gitlab.ReviewState{}
		}
		out[n.Username] = st
	}
	return out
}

type gqlApprovalRule struct {
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	Section           string    `json:"section"`
	ApprovalsRequired int       `json:"approvalsRequired"`
	Approved          bool      `json:"approved"`
	Invalid           bool      `json:"invalid"`
	ApprovedBy        gqlUsers  `json:"approvedBy"`
	EligibleApprovers []gqlUser `json:"eligibleApprovers"`
}

func (r gqlApprovalRule) domain() gitlab.ApprovalRule {
	rule := gitlab.ApprovalRule{
		Name: r.Name, Type: gitlab.ApprovalRuleType(lower(r.Type)), Section: r.Section,
		Required: r.ApprovalsRequired, Approved: r.Approved, Invalid: r.Invalid,
		ApprovedBy: r.ApprovedBy.domain(),
	}
	for _, u := range r.EligibleApprovers {
		rule.Eligible = append(rule.Eligible, u.domain())
	}
	return rule
}
