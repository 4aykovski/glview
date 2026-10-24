package rest

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Diffs returns the changes of the MR file by file (all pages).
func (s mrs) Diffs(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.DiffFile, error) {
	var out []gitlab.DiffFile
	opt := &gl.ListMergeRequestDiffsOptions{ListOptions: gl.ListOptions{PerPage: 100, Page: 1}}
	for {
		page, resp, err := s.c.api.MergeRequests.ListMergeRequestDiffs(pid(p), iid, opt, gl.WithContext(ctx))
		if err != nil {
			return nil, mapError(err)
		}
		for _, d := range page {
			out = append(out, gitlab.DiffFile{
				OldPath: d.OldPath, NewPath: d.NewPath,
				NewFile: d.NewFile, RenamedFile: d.RenamedFile, DeletedFile: d.DeletedFile,
				Generated: d.GeneratedFile, TooLarge: d.TooLarge || d.Collapsed,
				Diff: d.Diff,
			})
		}
		if resp.NextPage == 0 {
			return out, nil
		}
		opt.Page = resp.NextPage
	}
}

// CommentLine starts a diff thread on one line of the MR.
func (s mrs) CommentLine(ctx context.Context, p gitlab.ProjectRef, iid int64, body string, pos gitlab.DiffPosition) (gitlab.Discussion, error) {
	typ := "text"
	po := &gl.PositionOptions{
		PositionType: &typ,
		BaseSHA:      &pos.BaseSHA, StartSHA: &pos.StartSHA, HeadSHA: &pos.HeadSHA,
		OldPath: &pos.OldPath, NewPath: &pos.NewPath,
	}
	if pos.NewLine > 0 {
		n := int64(pos.NewLine)
		po.NewLine = &n
	}
	if pos.OldLine > 0 {
		n := int64(pos.OldLine)
		po.OldLine = &n
	}
	d, _, err := s.c.api.Discussions.CreateMergeRequestDiscussion(pid(p), iid,
		&gl.CreateMergeRequestDiscussionOptions{Body: &body, Position: po}, gl.WithContext(ctx))
	if err != nil {
		return gitlab.Discussion{}, mapError(err)
	}
	return mapDiscussion(d), nil
}
