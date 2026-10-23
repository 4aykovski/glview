// Package issuedetail is the full-screen view of an issue: title, status,
// labels, related merge requests, the description and the comment threads in
// one scrolling body (SPEC §3.15).
package issuedetail

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/comment"
	"github.com/4aykovski/glview/internal/ui/screens/issueactions"
	"github.com/4aykovski/glview/internal/ui/screens/render"
	"github.com/4aykovski/glview/internal/ui/screens/threads"
)

const (
	issueTTL       = 30 * time.Second
	relatedTTL     = 5 * time.Minute
	requestTimeout = 15 * time.Second
	headerLines    = 2 // header and its rule
)

type (
	issueLoadedMsg struct {
		tok   common.Token
		issue gitlab.Issue
		err   error
	}
	discLoadedMsg struct {
		tok   common.Token
		discs []gitlab.Discussion
		err   error
	}
	relatedLoadedMsg struct {
		tok common.Token
		mrs []gitlab.RelatedMR
		err error
	}
)

// Screen is the issue screen.
type Screen struct {
	env      *app.Env
	ref      gitlab.ProjectRef
	iid      int64
	issue    gitlab.Issue
	archived bool // the project is archived: no comments, no close

	threads    *threads.Model
	discLoaded bool
	related    []gitlab.RelatedMR

	issueGen, discGen, relGen, poll common.Gen

	scroll  int
	toEnd   bool // after a comment: scroll to the last thread once it arrives
	updated time.Time
	active  bool
	dirty   bool // something changed while covered: reload on return
	w, h    int

	md map[string]string // rendered description by width|lang|text
}

// New opens the issue; initial (from a list row) is shown until the details
// arrive. archived hides the comment and close actions.
func New(env *app.Env, ref gitlab.ProjectRef, initial gitlab.Issue, archived bool) *Screen {
	s := &Screen{
		env: env, ref: ref, iid: initial.IID, issue: initial, archived: archived,
		issueGen: common.NewGen(), discGen: common.NewGen(), relGen: common.NewGen(), poll: common.NewGen(),
		md: map[string]string{},
	}
	s.threads = threads.New(env, s.threadLink)
	return s
}

// Issue returns the issue as currently known (for tests).
func (s *Screen) Issue() gitlab.Issue { return s.issue }

func (s *Screen) Init() tea.Cmd { return nil }

func (s *Screen) Title() string { return fmt.Sprintf("%s #%d", s.ref.FullPath, s.iid) }

func (s *Screen) SetSize(w, h int) { s.w, s.h = w, h }

func (s *Screen) bodyHeight() int { return max(1, s.h-headerLines) }

func (s *Screen) threadLink(d gitlab.Discussion) string {
	link := s.webURL()
	if id := d.Notes[0].ID; id != 0 && link != "" {
		link += fmt.Sprintf("#note_%d", id)
	}
	return link
}

// webURL is the issue page: the one GitLab sent, else built from the host.
func (s *Screen) webURL() string {
	if s.issue.WebURL != "" {
		return s.issue.WebURL
	}
	return fmt.Sprintf("%s/-/issues/%d", s.env.ProjectURL("", s.ref.FullPath), s.iid)
}

func (s *Screen) StatusHint() string {
	return s.env.Msg.Project.StatusPoll(s.env.PollInterval(false).Round(time.Second).String())
}

func (s *Screen) KeyMap() common.KeyMap {
	k, im := s.env.Msg.Keys, s.env.Msg.Issue
	hide := func(b common.Binding) common.Binding {
		b.Hidden = s.archived
		return b
	}
	short := []common.Binding{common.NewBinding([]string{"j", "down"}, "j/k", k.Down, common.PriorityHigh)}
	if s.threads.Count() > 0 {
		short = append(short,
			common.NewBinding([]string{"J", "K"}, "J/K", k.Threads, common.PriorityNormal),
			common.NewBinding([]string{"space"}, "space", k.Fold, common.PriorityNormal),
		)
	}
	short = append(short,
		hide(common.NewBinding([]string{"c"}, "", k.Comment, common.PriorityNormal)),
		hide(common.NewBinding([]string{"x"}, "", im.KeyToggle, common.PriorityNormal)),
		common.NewBinding([]string{"r"}, "", k.Refresh, common.PriorityNormal),
		common.NewBinding([]string{"o"}, "", im.KeyBrowser, common.PriorityLow),
		common.NewBinding([]string{"y"}, "", k.Copy, common.PriorityLow),
		common.NewBinding([]string{"esc"}, "", k.Back, common.PriorityLow),
	)
	return common.KeyMap{Short: short}
}

// ---- loading ----

func (s *Screen) loadIssue(force bool) tea.Cmd {
	tok := s.issueGen.Next()
	env, ref, iid := s.env, s.ref, s.iid
	key := issueactions.GetKey(ref.FullPath, iid)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		is, _, err := common.Fetch(ctx, env.Cache, key, issueTTL, force, func(ctx context.Context) (gitlab.Issue, error) {
			return env.Client.Issues().Get(ctx, ref, iid)
		})
		return issueLoadedMsg{tok: tok, issue: is, err: err}
	}
}

func (s *Screen) loadDiscussions(force bool) tea.Cmd {
	tok := s.discGen.Next()
	env, ref, iid := s.env, s.ref, s.iid
	key := issueactions.DiscsKey(ref.FullPath, iid)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		d, _, err := common.Fetch(ctx, env.Cache, key, issueTTL, force, func(ctx context.Context) ([]gitlab.Discussion, error) {
			return env.Client.Issues().Discussions(ctx, ref, iid)
		})
		return discLoadedMsg{tok: tok, discs: d, err: err}
	}
}

func (s *Screen) loadRelated(force bool) tea.Cmd {
	if s.issue.MergeRequestsCount == 0 && !force {
		return nil // nothing is related as far as the list knows; r asks anyway
	}
	tok := s.relGen.Next()
	env, ref, iid := s.env, s.ref, s.iid
	key := issueactions.RelatedKey(ref.FullPath, iid)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		mrs, _, err := common.Fetch(ctx, env.Cache, key, relatedTTL, force, func(ctx context.Context) ([]gitlab.RelatedMR, error) {
			return env.Client.Issues().RelatedMRs(ctx, ref, iid)
		})
		return relatedLoadedMsg{tok: tok, mrs: mrs, err: err}
	}
}

// loadAll reloads everything; force goes past the cache, manual (r) also
// for the related MRs, which the poll leaves to their five-minute cache.
func (s *Screen) loadAll(force, manual bool) tea.Cmd {
	return tea.Batch(s.loadIssue(force), s.loadDiscussions(force), s.loadRelated(manual))
}

func (s *Screen) schedulePoll() tea.Cmd {
	if !s.active || s.env.AuthLost {
		s.poll.Next()
		return nil
	}
	return common.PollAfter(s.env.PollInterval(false), s.poll.Next())
}

// ---- update ----

func (s *Screen) Update(msg tea.Msg) (app.Screen, tea.Cmd) {
	env := s.env
	switch msg := msg.(type) {
	case app.ActivatedMsg:
		s.active = true
		switch {
		case s.updated.IsZero():
			return s, s.loadAll(false, false)
		case s.dirty || env.Now().Sub(s.updated) >= env.PollInterval(false):
			s.dirty = false
			return s, s.loadAll(true, false)
		}
		return s, s.schedulePoll()
	case app.DeactivatedMsg:
		s.active = false
		s.poll.Next()

	case issueLoadedMsg:
		if !s.issueGen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			return s, tea.Batch(env.Fail(msg.err), s.schedulePoll())
		}
		s.issue = msg.issue
		s.updated = env.Now()
		return s, s.schedulePoll()
	case discLoadedMsg:
		if !s.discGen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err != nil {
			return s, env.Fail(msg.err)
		}
		s.discLoaded = true
		s.threads.SetDiscussions(msg.discs)
		if s.toEnd {
			s.toEnd = false
			s.threads.SelectLast()
			s.scroll = 1 << 20
		}
	case relatedLoadedMsg:
		if !s.relGen.Valid(msg.tok) {
			return s, nil
		}
		if msg.err == nil {
			s.related = msg.mrs
		}

	case comment.SentMsg:
		if msg.Target.Kind != comment.KindIssue || msg.Target.Project.FullPath != s.ref.FullPath || msg.Target.IID != s.iid {
			return s, nil
		}
		s.toEnd = true
		if !s.active {
			s.dirty = true
			return s, nil
		}
		return s, tea.Batch(s.loadIssue(true), s.loadDiscussions(true))
	case issueactions.StateMsg:
		if msg.Ref.FullPath != s.ref.FullPath || msg.IID != s.iid || !s.active {
			if msg.Ref.FullPath == s.ref.FullPath && msg.IID == s.iid {
				s.dirty = true
			}
			return s, nil
		}
		cmd := issueactions.Done(env, msg)
		if msg.Err != nil {
			return s, cmd
		}
		return s, tea.Batch(cmd, s.loadIssue(true))

	case common.PollTickMsg:
		if !s.poll.Valid(msg.Token) || !s.active || env.AuthLost {
			return s, nil
		}
		return s, s.loadAll(true, false)
	case app.AuthLostMsg:
		s.poll.Next()
	case app.AuthRestoredMsg:
		if s.active {
			return s, s.loadAll(true, false)
		}
	case app.FocusChangedMsg:
		if msg.Focused && s.active && !s.updated.IsZero() && env.Now().Sub(s.updated) >= env.PollInterval(false) {
			return s, s.loadAll(true, false)
		}
	case app.LangChangedMsg, app.ThemeChangedMsg:
		s.md = map[string]string{}
		s.threads.ClearCache()

	case tea.KeyPressMsg:
		return s, s.onKey(msg)
	}
	return s, nil
}

func (s *Screen) onKey(k tea.KeyPressMsg) tea.Cmd {
	env := s.env
	switch k.String() {
	case "r":
		return s.loadAll(true, true)
	case "o":
		return app.OpenURL(env, s.webURL())
	case "y":
		return app.Copy(env, s.webURL(), render.ShortURL(s.webURL()))
	case "c":
		if !s.archived {
			return comment.Open(env, comment.Target{Project: s.ref, Kind: comment.KindIssue, IID: s.iid, Title: s.issue.Title})
		}
		return nil
	case "x":
		if !s.archived {
			return issueactions.Toggle(env, s.ref, s.issue)
		}
		return nil
	}
	s.scrollKey(k.String())
	return nil
}

// scrollKey scrolls the body; J/K and space also move between the threads.
func (s *Screen) scrollKey(k string) {
	lines, start := s.body()
	threadsShown := s.threads.Count() > 0 && s.discLoaded
	switch {
	case threadsShown && k == "J":
		s.scroll = start + s.threads.Next()
		return
	case threadsShown && k == "K":
		s.scroll = start + s.threads.Prev()
		return
	case threadsShown && k == "space" && s.scroll+s.bodyHeight() > start:
		s.scroll = min(s.scroll, start+s.threads.ToggleCurrent())
		return
	}
	page := max(1, s.bodyHeight()-1)
	switch k {
	case "j", "down":
		s.scroll++
	case "k", "up":
		s.scroll--
	case "pgdown", "ctrl+d", "space":
		s.scroll += page
	case "pgup", "ctrl+u":
		s.scroll -= page
	case "g", "home":
		s.scroll = 0
	case "G", "end":
		s.scroll = 1 << 20
	default:
		return
	}
	s.scroll = max(0, min(s.scroll, len(lines)-s.bodyHeight()))
	if threadsShown {
		s.threads.Repick(s.scroll - start)
	}
}

// ---- view ----

func (s *Screen) descriptionLines(width int) []string {
	text := s.issue.Description
	if strings.TrimSpace(text) == "" {
		return []string{s.env.Styles.Muted.Render(s.env.Msg.Issue.NoDescription)}
	}
	key := fmt.Sprintf("%d|%s|%s", width, s.env.Lang, text)
	out, ok := s.md[key]
	if !ok {
		out = common.Markdown(s.env.Styles, common.HTMLToMarkdown(text), width)
		s.md[key] = out
	}
	return strings.Split(out, "\n")
}

// body lays out the whole scrolling text and tells the line where the
// thread cards start.
func (s *Screen) body() (lines []string, threadsStart int) {
	st, m := s.env.Styles, s.env.Msg.Issue
	is := s.issue
	w := max(20, s.w-4)
	pad := func(l string) string { return "  " + render.Clip(l, s.w-2) }

	lines = append(lines, pad(st.Bold.Render(is.Title)), pad(issueactions.StatusLine(s.env, is)))
	if len(is.Labels) > 0 {
		lines = append(lines, pad(st.Muted.Render(m.Labels)+" "+strings.Join(is.Labels, st.Muted.Render(" · "))))
	}
	for _, r := range s.related {
		lines = append(lines, pad(issueactions.RelatedLine(s.env, r)))
	}
	lines = append(lines, common.Rule(st, s.w))
	for _, l := range s.descriptionLines(w) {
		lines = append(lines, "  "+l)
	}
	lines = append(lines, "", pad(st.Bold.Render(m.Comments(is.UserNotesCount))))
	switch {
	case !s.discLoaded:
		lines = append(lines, pad(st.Muted.Render(s.env.Msg.MR.Loading)))
		return lines, len(lines)
	case s.threads.Count() == 0:
		lines = append(lines, pad(st.Muted.Render(m.NoComments)))
		return lines, len(lines)
	}
	threadsStart = len(lines)
	cards, _ := s.threads.Layout(s.w)
	return append(lines, cards...), threadsStart
}

func (s *Screen) View() string {
	env := s.env
	st, m := env.Styles, env.Msg
	crumbs := st.Muted.Render(s.ref.FullPath+" › "+m.Issue.Crumb+" › ") + st.Accent.Render(fmt.Sprintf("#%d", s.iid))
	head := common.Header(st, crumbs, env.HeaderRight(s.updated), s.w)
	lines, _ := s.body()
	h := s.bodyHeight()
	s.scroll = max(0, min(s.scroll, len(lines)-h))
	end := min(len(lines), s.scroll+h)
	return head + "\n" + common.Rule(st, s.w) + "\n" + strings.Join(lines[s.scroll:end], "\n")
}
