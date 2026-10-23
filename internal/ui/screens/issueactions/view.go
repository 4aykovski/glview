package issueactions

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/render"
)

const (
	// NoteLines is the longest comment body in the preview.
	NoteLines = 6
	// NotesLast is how many last notes the preview asks for.
	NotesLast = 50
)

// Open reports whether the issue is open.
func Open(i gitlab.Issue) bool { return i.State != "closed" }

// StatusLine is "● open · #1318 · anna.k · assignee: egor · milestone:
// 2026-Q4"; a closed issue gets "○ closed", empty parts are left out.
func StatusLine(env *app.Env, i gitlab.Issue) string {
	s, m := env.Styles, env.Msg.Issue
	dot := s.Muted.Render(" · ")
	state := s.Success.Render("● " + m.StatusOpen)
	if !Open(i) {
		state = s.Muted.Render("○ " + m.StatusClosed)
	}
	parts := []string{state, s.Muted.Render(fmt.Sprintf("#%d", i.IID))}
	if i.Author.Username != "" {
		parts = append(parts, i.Author.Username)
	}
	if len(i.Assignees) > 0 {
		names := make([]string, len(i.Assignees))
		for k, u := range i.Assignees {
			names[k] = u.Username
		}
		parts = append(parts, m.Assignee(strings.Join(names, ", ")))
	}
	if i.Milestone != "" {
		parts = append(parts, m.Milestone(i.Milestone))
	}
	return strings.Join(parts, dot)
}

// RelatedLine is "→ !479 Fix slot race · opened".
func RelatedLine(env *app.Env, r gitlab.RelatedMR) string {
	s := env.Styles
	line := s.Success.Render(fmt.Sprintf("→ !%d", r.IID))
	if r.Title != "" {
		line += " " + r.Title
	}
	if r.State != "" {
		line += s.Muted.Render(" · " + string(r.State))
	}
	return line
}

// RelatedMark is the marker after the title in a list row: "→ !479", or
// "→ !479 +1" when there are more.
func RelatedMark(env *app.Env, rel []gitlab.RelatedMR) string {
	if len(rel) == 0 {
		return ""
	}
	text := fmt.Sprintf("→ !%d", rel[0].IID)
	if len(rel) > 1 {
		text += fmt.Sprintf(" +%d", len(rel)-1)
	}
	return env.Styles.Muted.Render(text)
}

// NotesState is how far the comments of the preview are.
type NotesState int

const (
	NotesLoading NotesState = iota
	NotesReady
	NotesFailed
)

// PreviewData is what the preview of an issue shows.
type PreviewData struct {
	Issue   gitlab.Issue
	Related []gitlab.RelatedMR // nil until loaded
	Notes   []gitlab.Note      // oldest first, system ones included
	State   NotesState
	Hint    bool // show "c — write a comment…" (not in an archived project)
}

// Preview renders the preview of the issue into exactly h lines of at most w
// cells: title, status, related MRs, the description (at most half of the
// height), the comments that fit whole, and the hint (SPEC §3.4).
func Preview(env *app.Env, d PreviewData, w, h int) []string {
	s, m := env.Styles, env.Msg.Issue
	i := d.Issue
	var out []string
	out = append(out, s.Bold.Render(i.Title), StatusLine(env, i))
	for _, r := range d.Related {
		out = append(out, RelatedLine(env, r))
	}
	hint := 0
	if d.Hint {
		hint = 1
	}
	room := func() int { return h - len(out) - hint }

	// Description.
	if room() > 1 {
		out = append(out, "")
		desc := strings.Split(common.Markdown(s, common.HTMLToMarkdown(i.Description), w), "\n")
		if strings.TrimSpace(i.Description) == "" {
			desc = []string{s.Muted.Render(m.NoDescription)}
		}
		limit := max(1, min(h/2, room()-2))
		if len(desc) > limit {
			desc = append(desc[:limit-1:limit-1], s.Muted.Render("…"))
		}
		out = append(out, desc...)
	}

	// Comments.
	if room() >= 2 {
		out = append(out, common.Rule(s, w), s.Muted.Render(m.PrevComments(i.UserNotesCount)))
		switch d.State {
		case NotesLoading:
			if room() > 0 {
				out = append(out, s.Muted.Render(m.NotesLoading))
			}
		case NotesFailed:
			if room() > 0 {
				out = append(out, s.Muted.Render(m.NotesFailed))
			}
		default:
			out = append(out, lastNotes(env, d.Notes, w, room())...)
		}
	}

	for len(out) < h-hint {
		out = append(out, "")
	}
	out = out[:min(len(out), h-hint)]
	if d.Hint {
		out = append(out, s.Muted.Render(m.WriteHint))
	}
	for k := range out {
		out[k] = render.Clip(out[k], w)
	}
	return out
}

// lastNotes lays out the newest comments from the bottom up, as many as fit
// whole into room lines, oldest of them first.
func lastNotes(env *app.Env, notes []gitlab.Note, w, room int) []string {
	s := env.Styles
	var blocks [][]string
	used := 0
	for k := len(notes) - 1; k >= 0; k-- {
		n := notes[k]
		if n.System {
			continue
		}
		body := strings.Split(common.Markdown(s, common.HTMLToMarkdown(n.Body), w), "\n")
		if len(body) > NoteLines {
			body = append(body[:NoteLines-1:NoteLines-1], s.Muted.Render("…"))
		}
		block := append([]string{s.Bold.Render(n.Author.Username) + s.Muted.Render(" · "+render.Ago(env.Lang, env.Now(), n.CreatedAt))}, body...)
		if used+len(block) > room {
			break
		}
		blocks = append([][]string{block}, blocks...)
		used += len(block)
	}
	var out []string
	for _, b := range blocks {
		out = append(out, b...)
	}
	return out
}

// ListURL is the project's issues page in the web UI with the filters.
// Defaults are omitted; "@me" is the login of the current user and is left
// out while it is unknown.
func ListURL(projectURL string, f gitlab.IssueFilter, me string) string {
	q := url.Values{}
	if f.State == "closed" || f.State == "all" {
		q.Set("state", f.State)
	}
	switch f.Assignee {
	case "":
	case "@me":
		if me != "" {
			q.Set("assignee_username", me)
		}
	case "none":
		q.Set("assignee_id", "None")
	default:
		q.Set("assignee_username", f.Assignee)
	}
	for _, l := range f.Labels {
		q.Add("label_name[]", l)
	}
	switch f.Milestone {
	case "":
	case "none":
		q.Set("milestone_title", "None")
	default:
		q.Set("milestone_title", f.Milestone)
	}
	if f.Search != "" {
		q.Set("search", f.Search)
	}
	u := projectURL + "/-/issues"
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}
