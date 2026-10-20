// Package render holds small view helpers shared by the project tabs and
// the MR, pipeline and job screens: durations, approvals, status chips.
package render

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
)

// Duration formats a duration compactly: 45s, 3m40s, 1h02m.
func Duration(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d <= 0:
		return "—"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

// ShortSHA cuts a commit SHA to 8 characters.
func ShortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// PipelineCommit is the Commit cell of a pipeline row: short SHA (7
// characters, as GitLab shows it) and the commit title when known.
func PipelineCommit(p gitlab.PipelineSummary) string {
	commit := ShortSHA(p.SHA)
	if len(commit) > 7 {
		commit = commit[:7]
	}
	if p.CommitTitle != "" {
		commit += " " + p.CommitTitle
	}
	return commit
}

// PipelineUser is the User cell of a pipeline row; "—" when unknown.
func PipelineUser(p gitlab.PipelineSummary) string {
	if p.User == nil {
		return "—"
	}
	return p.User.Username
}

// ShortURL turns ".../group/project/-/merge_requests/479" into
// "…/merge_requests/479" for toasts.
func ShortURL(url string) string {
	if _, rest, ok := strings.Cut(url, "/-/"); ok {
		return "…/" + rest
	}
	return url
}

// Ago renders the age of t as a short relative time.
func Ago(lang i18n.Lang, now, t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return i18n.RelTimeShort(lang, now.Sub(t))
}

// Approvals renders "1/2" while approvals are missing, "3 ✓" once the
// requirement is met (or exceeded) and a bare count when the instance has no
// required approvals (CE). The user's own approval is highlighted.
func Approvals(s *common.Styles, a gitlab.Approvals) string {
	n := len(a.ApprovedBy)
	switch {
	case a.Required > 0 && n >= a.Required:
		return s.Success.Render(fmt.Sprintf("%d ✓", n))
	case a.Required > 0:
		text := fmt.Sprintf("%d/%d", n, a.Required)
		if a.ApprovedByMe {
			return s.Success.Render(text)
		}
		if n == 0 {
			return s.Muted.Render(text)
		}
		return text
	}
	text := fmt.Sprint(n)
	switch {
	case a.ApprovedByMe:
		return s.Success.Render(text)
	case n == 0:
		return s.Muted.Render(text)
	}
	return text
}

// PipelineRef shortens the ref of a merge request pipeline
// ("refs/merge-requests/482/merge") to "!482"; other refs are returned as is.
// Display only: filters and URLs keep the original ref.
func PipelineRef(ref string) string {
	rest, ok := strings.CutPrefix(ref, "refs/merge-requests/")
	if !ok {
		return ref
	}
	iid, kind, ok := strings.Cut(rest, "/")
	if !ok || (kind != "head" && kind != "merge" && kind != "train") {
		return ref
	}
	if n, err := strconv.ParseUint(iid, 10, 64); err != nil || n == 0 {
		return ref
	}
	return "!" + iid
}

// Threads renders the unresolved-thread count: "—" when unknown, a dim 0,
// a warning number otherwise.
func Threads(s *common.Styles, d gitlab.DiscussionStats) string {
	switch {
	case d.Unresolved < 0:
		return s.Muted.Render("—")
	case d.Unresolved == 0:
		return s.Muted.Render("0")
	}
	return s.Warning.Render(fmt.Sprint(d.Unresolved))
}

// MergeRole is the color role of a detailed_merge_status.
type MergeRole int

const (
	RoleOK MergeRole = iota
	RoleRunning
	RoleWarn
	RoleBad
	RoleNeutral
)

// MergeStatusRole classifies detailed_merge_status values.
func MergeStatusRole(status string) MergeRole {
	switch status {
	case "mergeable":
		return RoleOK
	case "ci_still_running", "checking", "unchecked", "approvals_syncing", "preparing":
		return RoleRunning
	case "conflict", "need_rebase", "discussions_not_resolved", "draft_status", "requested_changes", "ci_must_pass":
		return RoleWarn
	case "not_approved", "blocked_status", "external_status_checks", "not_open":
		return RoleBad
	}
	return RoleNeutral
}

// MergeGlyph is the glyph of a detailed_merge_status chip.
func MergeGlyph(status string) string {
	switch MergeStatusRole(status) {
	case RoleOK:
		return "●"
	case RoleRunning:
		return "◐"
	case RoleWarn:
		return "⚠"
	case RoleBad:
		return "✗"
	}
	return "○"
}

// MRStatusChip returns the text and color role of an MR's status chip. A
// merged, closed or locked MR shows its state (detailed_merge_status is
// "not_open" there and says nothing useful); an open one shows the status
// exactly as GitLab returns it.
func MRStatusChip(mr *gitlab.MergeRequest) (string, MergeRole) {
	switch mr.State {
	case gitlab.MRStateMerged:
		return "✓ merged", RoleOK
	case gitlab.MRStateClosed, gitlab.MRStateLocked:
		return "○ " + string(mr.State), RoleNeutral
	}
	if mr.MergeStatus == "" {
		return "○ —", RoleNeutral
	}
	return MergeGlyph(mr.MergeStatus) + " " + mr.MergeStatus, MergeStatusRole(mr.MergeStatus)
}

// MergeChip renders the status chip of an MR as colored text.
func MergeChip(s *common.Styles, mr *gitlab.MergeRequest) string {
	text, role := MRStatusChip(mr)
	switch role {
	case RoleOK:
		return s.Success.Render(text)
	case RoleRunning:
		return s.StatusStyle(gitlab.StatusRunning).Render(text)
	case RoleWarn:
		return s.Warning.Render(text)
	case RoleBad:
		return s.Error.Render(text)
	}
	return s.Muted.Render(text)
}

// PipelineCell renders the glyph of an MR's head pipeline for tables.
func PipelineCell(s *common.Styles, p *gitlab.PipelineSummary) string {
	if p == nil {
		return s.Muted.Render("—")
	}
	return s.StatusGlyph(p.Status)
}

// StageChain renders "✓ build → ◐ test 2/3 → ○ deploy".
func StageChain(s *common.Styles, stages []gitlab.StageSummary) string {
	parts := make([]string, len(stages))
	for i, st := range stages {
		text := s.StatusGlyph(st.Status) + " " + st.Name
		if st.Status.Active() && st.Total > 0 {
			text += fmt.Sprintf(" %d/%d", st.Done, st.Total)
		}
		parts[i] = text
	}
	return strings.Join(parts, s.Muted.Render(" → "))
}

// Names joins usernames: "egor, anna.k".
func Names(users []gitlab.User) string {
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = u.Username
	}
	return strings.Join(names, ", ")
}

// FirstLine returns the first non-empty line of a markdown text.
func FirstLine(md string) string {
	for _, l := range strings.Split(md, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// Clip truncates styled text to w cells with an ellipsis.
func Clip(s string, w int) string { return ansi.Truncate(s, w, "…") }
