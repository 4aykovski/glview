package graphql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

var testProject = gitlab.ProjectRef{FullPath: "platform/route-service"}

func TestProjectMRsMapping(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string { return fixture(t, "project_mrs.json") })
	yes := true
	page, err := c.ProjectMRs(context.Background(), testProject, gitlab.MRFilter{
		State: gitlab.MRStateOpened, Reviewer: "@me", Draft: &yes, Labels: []string{"bug"}, Search: "race",
	}, "egor", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 41 || page.NextCursor != "eyJpZCI6IjEifQ" || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	q, vars := rec.queries[0], rec.vars[0]
	for _, want := range []string{"state: $state", "reviewerUsername: $reviewer", "draft: $draft", "labels: $labels", "search: $search", "sort: UPDATED_DESC", "fragment MRRow", "fragment PipelineRow"} {
		if !strings.Contains(q, want) {
			t.Errorf("query lacks %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "#@") || strings.Contains(q, "$me") || strings.Contains(q, "after") {
		t.Errorf("unexpected parts in query:\n%s", q)
	}
	if vars["reviewer"] != "egor" || vars["state"] != "opened" || vars["path"] != "platform/route-service" || vars["first"].(float64) != 20 {
		t.Errorf("vars = %v", vars)
	}

	mr := page.Items[0]
	if mr.IID != 479 || !mr.Draft || mr.State != gitlab.MRStateOpened || mr.MergeStatus != "not_approved" || mr.SHA != "a91f2c0d" {
		t.Errorf("mr = %+v", mr)
	}
	if mr.Approvals.Required != 2 || mr.Approvals.Left != 1 || !mr.Approvals.ApprovedByMe || len(mr.Approvals.ApprovedBy) != 1 {
		t.Errorf("approvals = %+v", mr.Approvals)
	}
	if mr.Discussions.Unresolved != 2 || mr.Discussions.Total != 5 {
		t.Errorf("discussions = %+v", mr.Discussions)
	}
	if mr.Diff == nil || mr.Diff.Additions != 48 || mr.Diff.Files != 3 {
		t.Errorf("diff = %+v", mr.Diff)
	}
	if len(mr.Labels) != 2 || mr.Reviewers[0].Username != "egor" || mr.Author.Username != "anna.k" {
		t.Errorf("people/labels = %+v", mr)
	}
	if !mr.Squash || !mr.ForceRemoveSourceBranch {
		t.Errorf("merge defaults lost: %+v", mr)
	}
	hp := mr.HeadPipeline
	if hp == nil || hp.ID != 91840 || hp.IID != 512 || hp.Status != gitlab.StatusRunning || hp.Duration != 220*time.Second ||
		hp.WebURL != rec.originOf(c)+"/platform/route-service/-/pipelines/91840" || hp.CommitTitle != "fix: guard slot map" || hp.User.Username != "anna.k" {
		t.Errorf("head pipeline = %+v", hp)
	}

	second := page.Items[1]
	if second.Approvals.Required != 0 || len(second.Approvals.ApprovedBy) != 2 || second.Approvals.ApprovedByMe {
		t.Errorf("CE approvals = %+v", second.Approvals)
	}
	if !second.HasConflicts || second.Diff != nil || second.HeadPipeline != nil || second.MergeStatus != "ci_still_running" {
		t.Errorf("second = %+v", second)
	}
}

func (r *recorder) originOf(c *Client) string { return c.origin() }

func TestProjectMRsUnknownThreadFields(t *testing.T) {
	c, _ := newRecorder(t, func(n int, q string, _ map[string]any) string {
		if strings.Contains(q, "resolvableDiscussionsCount") {
			return `{"errors":[{"message":"Field 'resolvableDiscussionsCount' doesn't exist on type 'MergeRequest'"}]}`
		}
		return `{"data":{"project":{"m":{"count":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"iid":"1","title":"x","state":"opened","detailedMergeStatus":"MERGEABLE"}]}}}}`
	})
	page, err := c.ProjectMRs(context.Background(), testProject, gitlab.MRFilter{State: gitlab.MRStateOpened}, "egor", "", 20)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("%+v %v", page, err)
	}
	if d := page.Items[0].Discussions; d.Unresolved != -1 || d.Total != -1 {
		t.Fatalf("unknown thread counts must be -1, got %+v", d)
	}
}

func TestProjectMRsNotFound(t *testing.T) {
	c, _ := newRecorder(t, func(int, string, map[string]any) string { return `{"data":{"project":null}}` })
	_, err := c.ProjectMRs(context.Background(), testProject, gitlab.MRFilter{}, "egor", "", 20)
	if !isNotFound(err) {
		t.Fatalf("err = %v", err)
	}
}

func isNotFound(err error) bool {
	ge, ok := err.(*gitlab.Error)
	return ok && ge.Kind == gitlab.ErrNotFound
}

func node(iid int, updated string) string {
	return fmt.Sprintf(`{"iid":"%d","title":"mr %d","state":"opened","updatedAt":"%s","detailedMergeStatus":"MERGEABLE"}`, iid, iid, updated)
}

// aliasOf returns the single connection alias of a list query ("a", "s", "r").
func aliasOf(t *testing.T, q string) string {
	t.Helper()
	var found []string
	for _, a := range []string{"m", "a", "s", "r"} {
		if strings.Contains(q, a+": mergeRequests(") {
			found = append(found, a)
		}
	}
	if len(found) != 1 {
		t.Errorf("each request must carry exactly one connection, got %v:\n%s", found, q)
		return ""
	}
	return found[0]
}

func TestProjectMRsMineUnionDedupAndCursor(t *testing.T) {
	c, rec := newRecorder(t, func(_ int, q string, vars map[string]any) string {
		conn := func(alias, body string) string { return `{"data":{"project":{"` + alias + `":` + body + `}}}` }
		switch a := aliasOf(t, q); {
		case a == "a" && vars["after_a"] == "CA":
			return conn(a, `{"count":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[`+node(15, "2026-10-18T10:00:00Z")+`]}`)
		case a == "r" && vars["after_r"] == "CR":
			return conn(a, `{"count":3,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}`)
		case a == "a":
			// !12 is both authored and reviewed; !13 is assigned only.
			return conn(a, `{"count":1,"pageInfo":{"hasNextPage":true,"endCursor":"CA"},"nodes":[`+node(12, "2026-10-19T10:00:00Z")+`]}`)
		case a == "s":
			return conn(a, `{"count":1,"pageInfo":{"hasNextPage":false,"endCursor":"CS"},"nodes":[`+node(13, "2026-10-19T12:00:00Z")+`]}`)
		default:
			return conn(a, `{"count":3,"pageInfo":{"hasNextPage":true,"endCursor":"CR"},"nodes":[`+node(12, "2026-10-19T10:00:00Z")+`,`+node(14, "2026-10-19T11:00:00Z")+`]}`)
		}
	})
	ctx := context.Background()
	f := gitlab.MRFilter{State: gitlab.MRStateOpened, Mine: true, Scope: gitlab.MRScopeAll}
	page, err := c.ProjectMRs(ctx, testProject, f, "egor", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if rec.count() != 3 {
		t.Fatalf("requests = %d, want one per alias", rec.count())
	}
	all := strings.Join(rec.queries, "\n")
	for _, want := range []string{"a: mergeRequests(", "s: mergeRequests(", "r: mergeRequests(", "authorUsername: $me", "assigneeUsername: $me", "reviewerUsername: $me"} {
		if !strings.Contains(all, want) {
			t.Errorf("queries lack %q", want)
		}
	}
	var iids []int64
	for _, m := range page.Items {
		iids = append(iids, m.IID)
	}
	if fmt.Sprint(iids) != "[13 14 12]" {
		t.Fatalf("union must be deduped and sorted by update, got %v", iids)
	}
	if page.Total != 3 {
		t.Errorf("Total = %d, want the largest connection count", page.Total)
	}
	if page.NextCursor != "CA||CR" {
		t.Fatalf("composite cursor = %q", page.NextCursor)
	}

	page, err = c.ProjectMRs(ctx, testProject, f, "egor", page.NextCursor, 20)
	if err != nil {
		t.Fatal(err)
	}
	if rec.count() != 5 {
		t.Fatalf("second page requests = %d, want 2 (exhausted alias skipped)", rec.count()-3)
	}
	for _, q := range rec.queries[3:] {
		if aliasOf(t, q) == "s" {
			t.Errorf("exhausted alias must be skipped:\n%s", q)
		}
	}
	if page.NextCursor != "" || len(page.Items) != 1 {
		t.Errorf("last page = %+v", page)
	}
}

func TestProjectMRsScopeAndMineCombine(t *testing.T) {
	c, rec := newRecorder(t, func(_ int, q string, _ map[string]any) string {
		return `{"data":{"project":{"` + aliasOf(t, q) + `":{"count":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}}}`
	})
	f := gitlab.MRFilter{State: gitlab.MRStateOpened, Mine: true, Scope: gitlab.MRScopeCreatedByMe}
	if _, err := c.ProjectMRs(context.Background(), testProject, f, "egor", "", 20); err != nil {
		t.Fatal(err)
	}
	if rec.count() != 3 {
		t.Fatalf("requests = %d", rec.count())
	}
	// scope stays a restriction on every alias: author is fixed in all three.
	for _, q := range rec.queries {
		if strings.Count(q, "authorUsername: $me") != 1 {
			t.Errorf("scope must apply to every alias:\n%s", q)
		}
	}
}

func TestProjectMRsMineFailsAsAWhole(t *testing.T) {
	c, _ := newRecorder(t, func(_ int, q string, _ map[string]any) string {
		if aliasOf(t, q) == "s" {
			return `{"data":{"project":null}}`
		}
		return `{"data":{"project":{"` + aliasOf(t, q) + `":{"count":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}}}`
	})
	f := gitlab.MRFilter{State: gitlab.MRStateOpened, Mine: true}
	if _, err := c.ProjectMRs(context.Background(), testProject, f, "egor", "", 20); !errors.Is(err, gitlab.ErrNotFound) {
		t.Errorf("err = %v, want not found", err)
	}
}

func TestMRDetail(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string { return fixture(t, "mr_detail.json") })
	mr, err := c.MR(context.Background(), testProject, 479, "egor")
	if err != nil {
		t.Fatal(err)
	}
	if rec.vars[0]["iid"] != "479" || !strings.Contains(rec.queries[0], "fragment MRDetail") || strings.Contains(rec.queries[0], "#@") {
		t.Errorf("request = %v\n%s", rec.vars[0], rec.queries[0])
	}
	if mr.Description == "" || !mr.RebaseInProgress || mr.MergeError != "Rebase failed: conflict in slot.go" || !mr.ForceRemoveSourceBranch {
		t.Errorf("detail fields = %+v", mr)
	}
	if mr.CommitCount != 3 || !strings.Contains(rec.queries[0], "commitCount") {
		t.Errorf("commit count = %d", mr.CommitCount)
	}
	if mr.Discussions.Unresolved != 0 || mr.Discussions.Total != 4 {
		t.Errorf("discussions = %+v", mr.Discussions)
	}
	hp := mr.HeadPipeline
	if hp == nil || hp.Status != gitlab.StatusFailed || len(hp.Stages) != 2 || hp.Stages[1].Status != gitlab.StatusFailed || hp.Stages[1].Done != 2 {
		t.Errorf("head pipeline = %+v", hp)
	}
}

func TestMRDetailLinks(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string { return fixture(t, "mr_detail.json") })
	mr, err := c.MR(context.Background(), testProject, 479, "egor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.queries[0], "titleHtml") || !strings.Contains(rec.queries[0], "descriptionHtml") {
		t.Errorf("detail query lacks the html fields")
	}
	origin := c.origin()
	wantTitle := []gitlab.Link{{Text: "ABC-123", URL: "https://tracker.example.com/browse/ABC-123"}}
	wantDesc := []gitlab.Link{
		{Text: "#12", URL: origin + "/platform/route-service/-/issues/12"},
		{Text: "@anna.k", URL: origin + "/anna.k"},
	}
	if !reflect.DeepEqual(mr.TitleLinks, wantTitle) || !reflect.DeepEqual(mr.DescriptionLinks, wantDesc) {
		t.Errorf("links = %+v / %+v", mr.TitleLinks, mr.DescriptionLinks)
	}
}

func TestMRDetailWithoutHTMLFields(t *testing.T) {
	c, rec := newRecorder(t, func(n int, q string, _ map[string]any) string {
		for _, f := range []string{"titleHtml", "descriptionHtml"} {
			if strings.Contains(q, f) {
				return `{"errors":[{"message":"Field '` + f + `' doesn't exist on type 'MergeRequest'"}]}`
			}
		}
		return `{"data":{"project":{"mergeRequest":{"iid":"1","title":"t","state":"opened","detailedMergeStatus":"MERGEABLE"}}}}`
	})
	mr, err := c.MR(context.Background(), testProject, 1, "egor")
	if err != nil || mr.IID != 1 || len(mr.TitleLinks) != 0 || len(mr.DescriptionLinks) != 0 {
		t.Fatalf("%+v %v", mr, err)
	}
	if rec.count() != 3 {
		t.Fatalf("requests = %d, want 3 (one retry per missing field)", rec.count())
	}
}

func TestMRDetailWithoutDiffStats(t *testing.T) {
	c, rec := newRecorder(t, func(n int, q string, _ map[string]any) string {
		if strings.Contains(q, "diffStatsSummary") {
			return `{"errors":[{"message":"Field 'diffStatsSummary' doesn't exist on type 'MergeRequest'"}]}`
		}
		return `{"data":{"project":{"mergeRequest":{"iid":"1","title":"t","state":"opened","detailedMergeStatus":"MERGEABLE"}}}}`
	})
	for range 2 {
		mr, err := c.MR(context.Background(), testProject, 1, "egor")
		if err != nil || mr.Diff != nil || mr.IID != 1 {
			t.Fatalf("%+v %v", mr, err)
		}
	}
	if rec.count() != 3 {
		t.Fatalf("requests = %d, want 3 (retry once, then straight)", rec.count())
	}
}

func TestMRDetailWithoutCommitCount(t *testing.T) {
	c, rec := newRecorder(t, func(n int, q string, _ map[string]any) string {
		if strings.Contains(q, "commitCount") {
			return `{"errors":[{"message":"Field 'commitCount' doesn't exist on type 'MergeRequest'"}]}`
		}
		return `{"data":{"project":{"mergeRequest":{"iid":"1","title":"t","state":"opened","detailedMergeStatus":"MERGEABLE"}}}}`
	})
	for range 2 {
		mr, err := c.MR(context.Background(), testProject, 1, "egor")
		if err != nil || mr.CommitCount != 0 || mr.IID != 1 {
			t.Fatalf("%+v %v", mr, err)
		}
	}
	if rec.count() != 3 {
		t.Fatalf("requests = %d, want 3 (retry once, then straight)", rec.count())
	}
}

func TestMRDetailDiffRefs(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string {
		return `{"data":{"project":{"mergeRequest":{"iid":"1","title":"t","state":"opened","detailedMergeStatus":"MERGEABLE","diffRefs":{"baseSha":"b1","headSha":"h1","startSha":"s1"}}}}}`
	})
	mr, err := c.MR(context.Background(), testProject, 1, "egor")
	if err != nil {
		t.Fatal(err)
	}
	if want := (gitlab.DiffRefs{BaseSHA: "b1", StartSHA: "s1", HeadSHA: "h1"}); mr.DiffRefs == nil || *mr.DiffRefs != want {
		t.Errorf("DiffRefs = %+v, want %+v", mr.DiffRefs, want)
	}
	if !strings.Contains(rec.queries[0], "diffRefs { baseSha headSha startSha }") {
		t.Errorf("detail query lacks diffRefs:\n%s", rec.queries[0])
	}
}

func TestMRDetailWithoutDiffRefs(t *testing.T) {
	c, rec := newRecorder(t, func(n int, q string, _ map[string]any) string {
		if strings.Contains(q, "diffRefs") {
			return `{"errors":[{"message":"Field 'diffRefs' doesn't exist on type 'MergeRequest'"}]}`
		}
		return `{"data":{"project":{"mergeRequest":{"iid":"1","title":"t","state":"opened","detailedMergeStatus":"MERGEABLE"}}}}`
	})
	for range 2 {
		mr, err := c.MR(context.Background(), testProject, 1, "egor")
		if err != nil || mr.DiffRefs != nil || mr.IID != 1 {
			t.Fatalf("%+v %v", mr, err)
		}
	}
	if rec.count() != 3 {
		t.Fatalf("requests = %d, want 3 (retry once, then straight)", rec.count())
	}
}

func TestMRDetailNotFound(t *testing.T) {
	c, _ := newRecorder(t, func(int, string, map[string]any) string { return `{"data":{"project":{"mergeRequest":null}}}` })
	if _, err := c.MR(context.Background(), testProject, 9, "egor"); !isNotFound(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestMRDetailMergeTimes(t *testing.T) {
	cases := []struct {
		name, file, state, by string
		merged, closed        bool
	}{
		{"merged with user", "mr_merged.json", "merged", "d.orlov", true, false},
		{"merged without user", "mr_merged_nouser.json", "merged", "", true, false},
		{"closed", "mr_closed.json", "closed", "", false, true},
		{"opened", "mr_detail.json", "opened", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newRecorder(t, func(int, string, map[string]any) string { return fixture(t, tc.file) })
			mr, err := c.MR(context.Background(), testProject, 606, "egor")
			if err != nil {
				t.Fatal(err)
			}
			if string(mr.State) != tc.state || mr.MergedAt.IsZero() == tc.merged || mr.ClosedAt.IsZero() == tc.closed {
				t.Errorf("state/times = %s %v %v", mr.State, mr.MergedAt, mr.ClosedAt)
			}
			if tc.merged && mr.MergedAt.UTC().Format("2006-01-02T15:04:05Z") != "2026-10-19T21:34:36Z" {
				t.Errorf("MergedAt = %v", mr.MergedAt)
			}
			if (tc.by == "") != (mr.MergedBy == nil) || (mr.MergedBy != nil && mr.MergedBy.Username != tc.by) {
				t.Errorf("MergedBy = %+v, want %q", mr.MergedBy, tc.by)
			}
			if !strings.Contains(rec.queries[0], "mergeUser { username name }") {
				t.Errorf("detail query lacks merge fields:\n%s", rec.queries[0])
			}
		})
	}
}

func TestMRRowFragmentHasNoMergeFields(t *testing.T) {
	for _, f := range []string{"mergedAt", "closedAt", "mergeUser", "commitCount", "titleHtml", "descriptionHtml", "diffRefs"} {
		if strings.Contains(mrRowFragment, f) {
			t.Errorf("MRRow must not select %s (complexity limit)", f)
		}
	}
}

func TestMRDetailApprovalRules(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string { return fixture(t, "mr_detail_rules.json") })
	mr, err := c.MR(context.Background(), testProject, 479, "egor")
	if err != nil {
		t.Fatal(err)
	}
	q := rec.queries[0]
	if !strings.Contains(q, "approvalState {") || !strings.Contains(q, "reviewState") || strings.Contains(q, "#@") {
		t.Errorf("detail query:\n%s", q)
	}
	if len(mr.Reviewers) != 3 || mr.Reviewers[1].Username != "dima" {
		t.Errorf("reviewers = %+v", mr.Reviewers)
	}
	wantStates := map[string]gitlab.ReviewState{"bob": gitlab.ReviewApproved, "dima": gitlab.ReviewRequestedChanges}
	if !reflect.DeepEqual(mr.ReviewStates, wantStates) {
		t.Errorf("ReviewStates = %v, want %v", mr.ReviewStates, wantStates)
	}
	rules := mr.Approvals.Rules
	if len(rules) != 5 {
		t.Fatalf("rules = %+v", rules)
	}
	if r := rules[0]; r.Name != "Backend" || r.Type != gitlab.ApprovalRegular || r.Required != 2 || len(r.ApprovedBy) != 1 || len(r.Eligible) != 3 {
		t.Errorf("rule 0 = %+v", r)
	}
	if r := rules[1]; r.Type != gitlab.ApprovalCodeOwner || r.Section != "Docs" || r.Name != "/docs/" {
		t.Errorf("rule 1 = %+v", r)
	}
	if rules[2].Type != gitlab.ApprovalAnyApprover || !rules[3].Invalid || rules[4].Required != 0 || !rules[4].Approved {
		t.Errorf("rules = %+v", rules[2:])
	}
}

func TestMRDetailWithoutApprovalState(t *testing.T) {
	c, rec := newRecorder(t, func(n int, q string, _ map[string]any) string {
		if strings.Contains(q, "approvalState") {
			return `{"errors":[{"message":"Field 'approvalState' doesn't exist on type 'MergeRequest'"}]}`
		}
		return `{"data":{"project":{"mergeRequest":{"iid":"1","title":"t","state":"opened","detailedMergeStatus":"MERGEABLE","reviewers":{"nodes":[{"username":"bob","mergeRequestInteraction":{"approved":false,"reviewState":"UNREVIEWED"}}]}}}}}`
	})
	for range 2 {
		mr, err := c.MR(context.Background(), testProject, 1, "egor")
		if err != nil || mr.Approvals.Rules != nil || mr.IID != 1 || mr.ReviewStates["bob"] != gitlab.ReviewUnreviewed {
			t.Fatalf("%+v %v", mr, err)
		}
	}
	if rec.count() != 3 {
		t.Fatalf("requests = %d, want 3 (retry once, then straight)", rec.count())
	}
}

func TestMRDetailWithoutReviewState(t *testing.T) {
	c, rec := newRecorder(t, func(n int, q string, _ map[string]any) string {
		if strings.Contains(q, "reviewState") {
			return `{"errors":[{"message":"Field 'reviewState' doesn't exist on type 'UserMergeRequestInteraction'"}]}`
		}
		return `{"data":{"project":{"mergeRequest":{"iid":"1","title":"t","state":"opened","detailedMergeStatus":"MERGEABLE","reviewers":{"nodes":[{"username":"bob","mergeRequestInteraction":{"approved":true}},{"username":"dima","mergeRequestInteraction":{"approved":false}}]}}}}}`
	})
	for range 2 {
		mr, err := c.MR(context.Background(), testProject, 1, "egor")
		if err != nil || mr.IID != 1 {
			t.Fatalf("%+v %v", mr, err)
		}
		if mr.ReviewStates["bob"] != gitlab.ReviewApproved || mr.ReviewStates["dima"] != "" {
			t.Errorf("ReviewStates = %v", mr.ReviewStates)
		}
	}
	if rec.count() != 3 {
		t.Fatalf("requests = %d, want 3 (retry once, then straight)", rec.count())
	}
}
