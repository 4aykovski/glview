package mrdetail

import (
	"fmt"

	"github.com/4aykovski/glview/internal/gitlab"
)

// threadLink is the link of a thread: the MR page anchored at its first note.
func (s *Screen) threadLink(d gitlab.Discussion) string {
	link := s.mr.WebURL
	if id := d.Notes[0].ID; id != 0 && link != "" {
		link += fmt.Sprintf("#note_%d", id)
	}
	return link
}

func (s *Screen) discussionsView() []string {
	st, m := s.env.Styles, s.env.Msg.MR
	if !s.discLoaded {
		return []string{st.Muted.Render(m.Loading)}
	}
	if s.threads.Count() == 0 {
		return []string{"", "  " + m.EmptyDiscussions}
	}
	lines, _ := s.threads.Layout(s.w)
	return lines
}

// link is the browser/clipboard target: the current thread on the
// Discussions tab, the selected commit on the Commits tab, the file under
// the cursor on the Diff tab, else the MR.
func (s *Screen) link() string {
	if s.tab == tabDiscussions && s.discLoaded {
		if c, ok := s.threads.Current(); ok {
			return c.Link
		}
	}
	if s.tab == tabDiff {
		return s.diffLink()
	}
	if s.tab == tabCommits {
		if c := s.selectedCommit(); c != nil && c.WebURL != "" {
			return c.WebURL
		}
	}
	return s.mr.WebURL
}

// threadKey handles scrolling keys on the Discussions tab: J/K jump between
// cards, space folds the current one, the rest scroll and re-pick the
// current thread (the first card reaching the top of the window).
func (s *Screen) threadKey(k string) {
	if !s.discLoaded || s.threads.Count() == 0 {
		return
	}
	lines, _ := s.threads.Layout(s.w)
	switch k {
	case "J":
		s.scroll = s.threads.Next()
		return
	case "K":
		s.scroll = s.threads.Prev()
		return
	case "space":
		s.scroll = min(s.scroll, s.threads.ToggleCurrent())
		return
	}
	s.scrollKey(k)
	s.scroll = max(0, min(s.scroll, len(lines)-s.bodyHeight()))
	s.threads.Repick(s.scroll)
}
