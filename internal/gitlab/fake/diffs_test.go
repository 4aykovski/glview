package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestFakeDiffsAndCommentLine(t *testing.T) {
	c := New()
	p := gitlab.ProjectRef{ID: 1, FullPath: "platform/route-service"}
	c.AddProject(gitlab.Project{ProjectRef: p})
	c.AddMR(p.FullPath, gitlab.MergeRequest{IID: 479, Title: "t"})
	c.SetMRDiffs(p.FullPath, 479, []gitlab.DiffFile{{OldPath: "a.go", NewPath: "a.go", Diff: "@@ -1 +1 @@\n-a\n+b\n"}})
	ctx := context.Background()

	fs, err := c.MergeRequests().Diffs(ctx, p, 479)
	if err != nil || len(fs) != 1 || fs[0].NewPath != "a.go" {
		t.Fatalf("diffs: %+v %v", fs, err)
	}
	pos := gitlab.DiffPosition{OldPath: "a.go", NewPath: "a.go", NewLine: 1}
	d, err := c.MergeRequests().CommentLine(ctx, p, 479, "why?", pos)
	if err != nil || d.Position == nil || d.Position.NewLine != 1 || d.Position.Path != "a.go" {
		t.Fatalf("comment line: %+v %v", d, err)
	}
	ds, _ := c.MergeRequests().Discussions(ctx, p, 479)
	if len(ds) != 1 || ds[0].Notes[0].Body != "why?" {
		t.Fatalf("discussions = %+v", ds)
	}
	c.FailNext("MergeRequests.CommentLine", &gitlab.Error{Kind: gitlab.ErrNotPossible, Status: 400, Message: "line gone"})
	if _, err := c.MergeRequests().CommentLine(ctx, p, 479, "x", pos); !errors.Is(err, gitlab.ErrNotPossible) {
		t.Fatalf("injected error: %v", err)
	}
}
