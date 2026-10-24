// Package threads lays out discussion threads as cards: shared by the MR
// Discussions tab and the issue screen.
package threads

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	// replyIndent is the left offset of replies inside a thread card.
	replyIndent = 2
	// foldLines: a first note longer than this (rendered) starts folded.
	foldLines = 20
)

// Card is the position of one thread card in the laid-out text.
type Card struct {
	ID    string
	Start int // first line of the frame
	H     int // frame height
	Link  string
}

// Model holds the threads, the folds the user toggled and the current
// thread; folds and the current thread are keyed by discussion ID, so they
// survive SetDiscussions.
type Model struct {
	env  *app.Env
	link func(gitlab.Discussion) string
	ds   []gitlab.Discussion
	fold map[string]bool
	md   map[string]string // rendered markdown by width|lang|text
	cur  string
	w    int // width of the last Layout
}

// New returns an empty model; link gives the browser/clipboard target of a
// thread (it may return "").
func New(env *app.Env, link func(gitlab.Discussion) string) *Model {
	return &Model{env: env, link: link, fold: map[string]bool{}, md: map[string]string{}}
}

// User drops discussions that only hold system notes (and the system notes
// of mixed ones).
func User(ds []gitlab.Discussion) []gitlab.Discussion {
	var out []gitlab.Discussion
	for _, d := range ds {
		var notes []gitlab.Note
		for _, n := range d.Notes {
			if !n.System {
				notes = append(notes, n)
			}
		}
		if len(notes) > 0 {
			d.Notes = notes
			out = append(out, d)
		}
	}
	return out
}

// SetDiscussions replaces the threads (system notes are dropped); folds and
// the current thread stay.
func (m *Model) SetDiscussions(ds []gitlab.Discussion) { m.ds = User(ds) }

// Count is the number of threads.
func (m *Model) Count() int { return len(m.ds) }

// Reset forgets the current thread (the first one is current again).
func (m *Model) Reset() { m.cur = "" }

// ClearCache drops the rendered markdown (language or theme changed).
func (m *Model) ClearCache() { m.md = map[string]string{} }

// SelectLast makes the last thread current.
func (m *Model) SelectLast() {
	if n := len(m.ds); n > 0 {
		m.cur = m.ds[n-1].ID
	}
}

// noteMarkdown renders a note body: HTML left by bots becomes markdown first.
// The result is cached together with the markdown (key — body and width).
func (m *Model) noteMarkdown(body string, width int) []string {
	key := fmt.Sprintf("n|%d|%s|%s", width, m.env.Lang, body)
	out, ok := m.md[key]
	if !ok {
		out = common.Markdown(m.env.Styles, common.HTMLToMarkdown(body), width)
		m.md[key] = out
	}
	return strings.Split(out, "\n")
}

// noteHead is the header line of a note: author, age and, for the first
// note of a thread, its status and the code position.
func (m *Model) noteHead(d gitlab.Discussion, n gitlab.Note, first bool) string {
	st, msg := m.env.Styles, m.env.Msg.MR
	head := st.Bold.Render(n.Author.Username) + " " + st.Muted.Render(render.Ago(m.env.Lang, m.env.Now(), n.CreatedAt))
	if !first {
		return head
	}
	if d.Resolvable {
		if d.Resolved {
			head += "  " + st.Success.Render("✓ "+msg.ThreadResolved)
		} else {
			head += "  " + st.Warning.Render("● "+msg.ThreadUnresolved)
		}
	}
	if p := d.Position; p != nil {
		head += "  " + st.Accent.Render(fmt.Sprintf("%s:%d", p.Path, p.Line))
	}
	return head
}

// folded reports whether the thread is shown as one line: the user's choice
// if any, else resolved threads and threads with a long first note.
func (m *Model) folded(d gitlab.Discussion, iw int) bool {
	if v, ok := m.fold[d.ID]; ok {
		return v
	}
	if d.Resolvable && d.Resolved {
		return true
	}
	return len(m.noteMarkdown(d.Notes[0].Body, iw)) > foldLines
}

// cardInner lays out the inside of an expanded thread card, iw columns wide.
func (m *Model) cardInner(d gitlab.Discussion, iw int) []string {
	var out []string
	for i, n := range d.Notes {
		pad := ""
		if i > 0 {
			out = append(out, common.Rule(m.env.Styles, iw))
			pad = strings.Repeat(" ", replyIndent)
		}
		out = append(out, pad+m.noteHead(d, n, i == 0))
		for _, l := range m.noteMarkdown(n.Body, iw-len(pad)) {
			out = append(out, pad+l)
		}
	}
	return out
}

// foldedLine is the one-line form: header of the first note, the first line
// of its text and the number of replies on the right.
func (m *Model) foldedLine(d gitlab.Discussion, iw int) string {
	st := m.env.Styles
	first := ""
	for _, l := range m.noteMarkdown(d.Notes[0].Body, iw) {
		if l = strings.TrimSpace(l); l != "" {
			first = l
			break
		}
	}
	left := st.Muted.Render("▸ ") + m.noteHead(d, d.Notes[0], true) + "  " + first
	if len(d.Notes) < 2 {
		return render.Clip(left, iw)
	}
	right := st.Muted.Render(m.env.Msg.Plural.Replies(len(d.Notes) - 1))
	left = render.Clip(left, max(1, iw-ansi.StringWidth(right)-2))
	gap := max(2, iw-ansi.StringWidth(left)-ansi.StringWidth(right))
	return left + strings.Repeat(" ", gap) + right
}

func innerWidth(width int) int { return max(16, width-4) }

// Layout draws all thread cards one under another for a screen width and
// reports where each one starts. The current thread gets the Accent frame.
func (m *Model) Layout(width int) ([]string, []Card) {
	m.w = width
	st := m.env.Styles
	iw := innerWidth(width)
	cur := m.currentIndex()
	var lines []string
	cards := make([]Card, 0, len(m.ds))
	for i, d := range m.ds {
		if i > 0 {
			lines = append(lines, "")
		}
		var inner []string
		if m.folded(d, iw) {
			inner = []string{m.foldedLine(d, iw)}
		} else {
			inner = m.cardInner(d, iw)
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
		link := ""
		if m.link != nil {
			link = m.link(d)
		}
		cards = append(cards, Card{ID: d.ID, Start: len(lines), H: len(inner) + 2, Link: link})
		lines = append(lines, strings.Split(common.Box(st, strings.Join(inner, "\n"), iw+4, len(inner)+2, kind), "\n")...)
	}
	return lines, cards
}

// currentIndex is the index of the current thread; the first one when none
// was picked yet.
func (m *Model) currentIndex() int {
	for i, d := range m.ds {
		if d.ID == m.cur {
			return i
		}
	}
	return 0
}

// Current is the current card as of the last Layout.
func (m *Model) Current() (Card, bool) {
	if len(m.ds) == 0 {
		return Card{}, false
	}
	_, cards := m.Layout(m.w)
	return cards[m.currentIndex()], true
}

// Next makes the next thread current and returns its first line (stays on
// the last thread). The model must have been laid out once.
func (m *Model) Next() int { return m.move(1) }

// Prev makes the previous thread current and returns its first line.
func (m *Model) Prev() int { return m.move(-1) }

func (m *Model) move(delta int) int {
	if len(m.ds) == 0 {
		return 0
	}
	_, cards := m.Layout(m.w)
	i := min(max(m.currentIndex()+delta, 0), len(cards)-1)
	m.cur = cards[i].ID
	return cards[i].Start
}

// ToggleCurrent folds or unfolds the current thread and returns its first
// line.
func (m *Model) ToggleCurrent() int {
	if len(m.ds) == 0 {
		return 0
	}
	i := m.currentIndex()
	d := m.ds[i]
	m.fold[d.ID] = !m.folded(d, innerWidth(m.w))
	m.cur = d.ID
	_, cards := m.Layout(m.w)
	return cards[i].Start
}

// Repick makes current the first card that reaches the top of a window
// scrolled to scroll; past the last card — the last one.
func (m *Model) Repick(scroll int) {
	_, cards := m.Layout(m.w)
	for _, c := range cards {
		if c.Start+c.H > scroll {
			m.cur = c.ID
			return
		}
	}
	if n := len(cards); n > 0 {
		m.cur = cards[n-1].ID
	}
}
