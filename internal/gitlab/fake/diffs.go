package fake

import (
	"context"
	"fmt"
	"slices"

	"github.com/4aykovski/glview/internal/gitlab"
)

// SetMRDiffs replaces the diff files of an MR.
func (c *Client) SetMRDiffs(path string, iid int64, files []gitlab.DiffFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data.mrDiffs == nil {
		c.data.mrDiffs = map[string][]gitlab.DiffFile{}
	}
	c.data.mrDiffs[mrKey(path, iid)] = files
}

// Diffs returns the diff files set with SetMRDiffs.
func (s mrService) Diffs(ctx context.Context, p gitlab.ProjectRef, iid int64) ([]gitlab.DiffFile, error) {
	if err := s.c.enter(ctx, "MergeRequests.Diffs", p.FullPath, iid); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return slices.Clone(s.c.data.mrDiffs[mrKey(s.c.pathOf(p), iid)]), nil
}

// CommentLine adds a diff thread by the fake user.
func (s mrService) CommentLine(ctx context.Context, p gitlab.ProjectRef, iid int64, body string, pos gitlab.DiffPosition) (gitlab.Discussion, error) {
	if err := s.c.enter(ctx, "MergeRequests.CommentLine", p.FullPath, iid, body, pos); err != nil {
		return gitlab.Discussion{}, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	path := s.c.pathOf(p)
	if _, err := s.c.findMR(path, iid); err != nil {
		return gitlab.Discussion{}, err
	}
	n := s.c.newNote(body)
	np := &gitlab.NotePosition{
		Path: pos.NewPath, Line: pos.NewLine,
		OldPath: pos.OldPath, NewPath: pos.NewPath, OldLine: pos.OldLine, NewLine: pos.NewLine,
		BaseSHA: pos.BaseSHA, StartSHA: pos.StartSHA, HeadSHA: pos.HeadSHA,
	}
	if pos.NewLine == 0 {
		np.Path, np.Line = pos.OldPath, pos.OldLine
	}
	d := gitlab.Discussion{ID: fmt.Sprintf("n%d", n.ID), Resolvable: true, Notes: []gitlab.Note{n}, Position: np}
	k := mrKey(path, iid)
	s.c.data.discs[k] = append(s.c.data.discs[k], d)
	return d, nil
}
