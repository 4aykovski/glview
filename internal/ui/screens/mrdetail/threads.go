package mrdetail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	// replyIndent is the left offset of replies inside a thread card.
	replyIndent = 2
	// foldLines: a first note longer than this (rendered) starts folded.
	foldLines = 20
)

// card is the position of one thread card in the laid-out tab.
type card struct {
	id    string
	start int // first line of the frame
	h     int // frame height
	link  string
}

// noteMarkdown renders a note body: HTML left by bots becomes markdown first.
// The result is cached together with the markdown (key — body and width).
func (s *Screen) noteMarkdown(body string, width int) []string {
	key := fmt.Sprintf("n|%d|%s|%s", width, s.env.Lang, body)
	out, ok := s.md[key]
	if !ok {
		out = common.Markdown(s.env.Styles, common.HTMLToMarkdown(body), width)
		s.md[key] = out
	}
	return strings.Split(out, "\n")
}

// noteHead is the header line of a note: author, age and, for the first
// note of a thread, its status and the code position.
func (s *Screen) noteHead(d gitlab.Discussion, n gitlab.Note, first bool) string {
	st, m := s.env.Styles, s.env.Msg.MR
	head := st.Bold.Render(n.Author.Username) + " " + st.Muted.Render(render.Ago(s.env.Lang, s.env.Now(), n.CreatedAt))
	if !first {
		return head
	}
	if d.Resolvable {
		if d.Resolved {
			head += "  " + st.Success.Render("✓ "+m.ThreadResolved)
		} else {
			head += "  " + st.Warning.Render("● "+m.ThreadUnresolved)
		}
	}
	if p := d.Position; p != nil {
		head += "  " + st.Accent.Render(fmt.Sprintf("%s:%d", p.Path, p.Line))
	}
	return head
}

// folded reports whether the thread is shown as one line: the user's choice
// if any, else resolved threads and threads with a long first note.
func (s *Screen) folded(d gitlab.Discussion, iw int) bool {
	if v, ok := s.fold[d.ID]; ok {
		return v
	}
	if d.Resolvable && d.Resolved {
		return true
	}
	return len(s.noteMarkdown(d.Notes[0].Body, iw)) > foldLines
}

// cardInner lays out the inside of an expanded thread card, iw columns wide.
func (s *Screen) cardInner(d gitlab.Discussion, iw int) []string {
	var out []string
	for i, n := range d.Notes {
		pad := ""
		if i > 0 {
			out = append(out, common.Rule(s.env.Styles, iw))
			pad = strings.Repeat(" ", replyIndent)
		}
		out = append(out, pad+s.noteHead(d, n, i == 0))
		for _, l := range s.noteMarkdown(n.Body, iw-len(pad)) {
			out = append(out, pad+l)
		}
	}
	return out
}

// foldedLine is the one-line form: header of the first note, the first line
// of its text and the number of replies on the right.
func (s *Screen) foldedLine(d gitlab.Discussion, iw int) string {
	st := s.env.Styles
	first := ""
	for _, l := range s.noteMarkdown(d.Notes[0].Body, iw) {
		if l = strings.TrimSpace(l); l != "" {
			first = l
			break
		}
	}
	left := st.Muted.Render("▸ ") + s.noteHead(d, d.Notes[0], true) + "  " + first
	if len(d.Notes) < 2 {
		return render.Clip(left, iw)
	}
	right := st.Muted.Render(s.env.Msg.Plural.Replies(len(d.Notes) - 1))
	left = render.Clip(left, max(1, iw-ansi.StringWidth(right)-2))
	gap := max(2, iw-ansi.StringWidth(left)-ansi.StringWidth(right))
	return left + strings.Repeat(" ", gap) + right
}

// layout draws all thread cards one under another and reports where each
// one starts. The current thread gets the Accent frame.
func (s *Screen) layout() ([]string, []card) {
	st := s.env.Styles
	ds := userDiscussions(s.discs)
	iw := max(16, s.w-4)
	cur := s.currentIndex(ds)
	var lines []string
	cards := make([]card, 0, len(ds))
	for i, d := range ds {
		if i > 0 {
			lines = append(lines, "")
		}
		var inner []string
		if s.folded(d, iw) {
			inner = []string{s.foldedLine(d, iw)}
		} else {
			inner = s.cardInner(d, iw)
		}
		for j := range inner {
			inner[j] = " " + inner[j]
		}
		kind := common.BoxNormal
		switch {
		case i == cur:
			kind = common.BoxSelected
		case d.Resolvable && !d.Resolved:
			kind = common.BoxWarning
		}
		link := s.mr.WebURL
		if id := d.Notes[0].ID; id != 0 && link != "" {
			link += fmt.Sprintf("#note_%d", id)
		}
		cards = append(cards, card{id: d.ID, start: len(lines), h: len(inner) + 2, link: link})
		lines = append(lines, strings.Split(common.Box(st, strings.Join(inner, "\n"), iw+4, len(inner)+2, kind), "\n")...)
	}
	return lines, cards
}

func (s *Screen) currentIndex(ds []gitlab.Discussion) int {
	for i, d := range ds {
		if d.ID == s.curThread {
			return i
		}
	}
	return 0
}

func (s *Screen) discussionsView() []string {
	st, m := s.env.Styles, s.env.Msg.MR
	if !s.discLoaded {
		return []string{st.Muted.Render(m.Loading)}
	}
	if len(userDiscussions(s.discs)) == 0 {
		return []string{"", "  " + m.EmptyDiscussions}
	}
	lines, _ := s.layout()
	return lines
}

// link is the browser/clipboard target: the current thread on the
// Discussions tab, else the MR.
func (s *Screen) link() string {
	if s.tab == tabDiscussions && s.discLoaded {
		if _, cards := s.layout(); len(cards) > 0 {
			return cards[s.cardIndex(cards)].link
		}
	}
	return s.mr.WebURL
}

func (s *Screen) cardIndex(cards []card) int {
	for i, c := range cards {
		if c.id == s.curThread {
			return i
		}
	}
	return 0
}

// threadKey handles scrolling keys on the Discussions tab: J/K jump between
// cards, space folds the current one, the rest scroll and re-pick the
// current thread (the first card reaching the top of the window).
func (s *Screen) threadKey(k string) {
	if !s.discLoaded {
		return
	}
	lines, cards := s.layout()
	if len(cards) == 0 {
		return
	}
	cur := s.cardIndex(cards)
	switch k {
	case "J", "K":
		if k == "J" {
			cur = min(cur+1, len(cards)-1)
		} else {
			cur = max(cur-1, 0)
		}
		s.curThread, s.scroll = cards[cur].id, cards[cur].start
		return
	case "space":
		ds := userDiscussions(s.discs)
		s.fold[cards[cur].id] = !s.folded(ds[cur], max(16, s.w-4))
		s.curThread = cards[cur].id
		_, cards = s.layout()
		s.scroll = min(s.scroll, cards[cur].start)
		return
	}
	s.scrollKey(k)
	s.scroll = max(0, min(s.scroll, len(lines)-s.bodyHeight()))
	for i, c := range cards {
		if c.start+c.h > s.scroll {
			s.curThread = cards[i].id
			return
		}
	}
	s.curThread = cards[len(cards)-1].id
}
