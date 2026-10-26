package mrdetail

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
)

// APPROVALS column of the Overview tab (SPEC 3.5): who must still approve.

// defaultSection is the CODEOWNERS section GitLab uses without sections.
const defaultSection = "codeowners"

// shownRules returns the rules the column shows (Required > 0): unmet
// first, then invalid, then met; API order inside a group.
func shownRules(rules []gitlab.ApprovalRule) []gitlab.ApprovalRule {
	var out []gitlab.ApprovalRule
	for _, r := range rules {
		if r.Required > 0 {
			out = append(out, r)
		}
	}
	group := func(r gitlab.ApprovalRule) int {
		switch {
		case r.Invalid:
			return 1
		case r.Approved:
			return 2
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool { return group(out[i]) < group(out[j]) })
	return out
}

// ruleTitle is the block title: "<section> · <name>" for a code owner rule
// with a named section, the API name otherwise.
func ruleTitle(r gitlab.ApprovalRule) string {
	if r.Type == gitlab.ApprovalCodeOwner && r.Section != "" && !strings.EqualFold(r.Section, defaultSection) {
		return r.Section + " · " + r.Name
	}
	return r.Name
}

// ruleCount is "have/required" of a rule, have capped at required.
func ruleCount(r gitlab.ApprovalRule) (have, required int) {
	return min(len(r.ApprovedBy), r.Required), r.Required
}

// candidates are the eligible approvers who have not approved by the rule.
func candidates(r gitlab.ApprovalRule) []string {
	var out []string
	for _, u := range r.Eligible {
		if !slices.ContainsFunc(r.ApprovedBy, func(a gitlab.User) bool { return a.Username == u.Username }) {
			out = append(out, u.Username)
		}
	}
	return out
}

func usernames(us []gitlab.User) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = u.Username
	}
	return out
}

// fitNames joins names with ", " into width columns; the names that do not
// fit are replaced by " +K". At least one name is kept (cut with "…").
func fitNames(names []string, width int) (shown []string, more int) {
	for k := len(names); k >= 1; k-- {
		w := ansi.StringWidth(strings.Join(names[:k], ", "))
		if k < len(names) {
			w += ansi.StringWidth(" +") + len(strconv.Itoa(len(names)-k))
		}
		if w <= width {
			return names[:k], len(names) - k
		}
	}
	if len(names) == 0 {
		return nil, 0
	}
	more = len(names) - 1
	room := width
	if more > 0 {
		room -= len(" +") + len(strconv.Itoa(more))
	}
	return []string{ansi.Truncate(names[0], max(room, 1), "…")}, more
}

// hasRules reports whether the column is in the rules mode.
func hasRules(a gitlab.Approvals) bool { return len(shownRules(a.Rules)) > 0 }

// reviewerApproved: the reviewer approved, by state or by the approvals list.
func reviewerApproved(mr gitlab.MergeRequest, username string) bool {
	return mr.ReviewStates[username] == gitlab.ReviewApproved ||
		slices.ContainsFunc(mr.Approvals.ApprovedBy, func(u gitlab.User) bool { return u.Username == username })
}

// needsMyApproval: me is a candidate of an unmet valid rule (not
// any_approver), or, without rules, a reviewer who has not answered yet.
func needsMyApproval(mr gitlab.MergeRequest, me string) bool {
	if me == "" || mr.Approvals.ApprovedByMe || reviewerApproved(mr, me) {
		return false
	}
	if hasRules(mr.Approvals) {
		for _, r := range shownRules(mr.Approvals.Rules) {
			if r.Approved || r.Invalid || r.Type == gitlab.ApprovalAnyApprover {
				continue
			}
			if slices.Contains(candidates(r), me) {
				return true
			}
		}
		return false
	}
	for _, u := range mr.Reviewers {
		if u.Username == me {
			return mr.ReviewStates[me] != gitlab.ReviewRequestedChanges
		}
	}
	return false
}

// approvalLines renders the body of the APPROVALS column (without the
// heading) for the column width w.
func approvalLines(st *common.Styles, m approvalText, mr gitlab.MergeRequest, me string, w int) []string {
	var out []string
	if needsMyApproval(mr, me) {
		out = append(out, st.Warning.Render(m.needYour))
	} else {
		me = "" // my name is highlighted only when my approval is needed
	}
	if hasRules(mr.Approvals) {
		return append(out, ruleLines(st, m, mr.Approvals.Rules, me, w)...)
	}
	return append(out, reviewerLines(st, m, mr, me, w)...)
}

// approvalText are the strings the column needs.
type approvalText struct {
	none, needYour, anyApprover, invalid, changes string
	needMore                                      func(int) string
}

// nameLine renders "<indent><glyph> names +K", me in Bold.
func nameLine(st *common.Styles, glyph string, names []string, me string, w int) string {
	shown, more := fitNames(names, w-2-2)
	parts := make([]string, len(shown))
	for i, n := range shown {
		parts[i] = n
		if n == me {
			parts[i] = st.Bold.Render(n)
		}
	}
	line := "  " + glyph + " " + strings.Join(parts, ", ")
	if more > 0 {
		line += st.Muted.Render(" +" + strconv.Itoa(more))
	}
	return line
}

func ruleLines(st *common.Styles, m approvalText, rules []gitlab.ApprovalRule, me string, w int) []string {
	var out []string
	for _, r := range shownRules(rules) {
		glyph := st.Muted.Render("○")
		switch {
		case r.Invalid:
			glyph = st.Error.Render("✗")
		case r.Approved:
			glyph = st.Success.Render("✓")
		}
		have, req := ruleCount(r)
		count := strconv.Itoa(have) + "/" + strconv.Itoa(req)
		title := ansi.Truncate(ruleTitle(r), max(w-2-len(count)-1, 1), "…")
		gap := max(1, w-2-ansi.StringWidth(title)-len(count))
		out = append(out, glyph+" "+title+strings.Repeat(" ", gap)+st.Muted.Render(count))
		if r.Approved && !r.Invalid {
			continue
		}
		if len(r.ApprovedBy) > 0 {
			out = append(out, nameLine(st, st.Success.Render("✓"), usernames(r.ApprovedBy), me, w))
		}
		switch {
		case r.Invalid:
			out = append(out, "  "+st.Muted.Render(m.invalid))
		case r.Type == gitlab.ApprovalAnyApprover:
			out = append(out, "  "+st.Muted.Render(m.anyApprover))
		default:
			if c := candidates(r); len(c) > 0 {
				out = append(out, nameLine(st, st.Muted.Render("○"), c, me, w))
			}
		}
	}
	return out
}

// reviewerLines is the column without rules: reviewers with their state,
// approvers who are not reviewers, the count of approvals still needed.
func reviewerLines(st *common.Styles, m approvalText, mr gitlab.MergeRequest, me string, w int) []string {
	var out []string
	name := func(glyph string, u gitlab.User, tail string) string {
		n := ansi.Truncate(u.Username, max(w-2-ansi.StringWidth(tail), 1), "…")
		if u.Username == me {
			n = st.Bold.Render(n)
		}
		return glyph + " " + n + st.Muted.Render(tail)
	}
	for _, u := range mr.Reviewers {
		switch {
		case reviewerApproved(mr, u.Username):
			out = append(out, name(st.Success.Render("✓"), u, ""))
		case mr.ReviewStates[u.Username] == gitlab.ReviewRequestedChanges:
			out = append(out, name(st.Error.Render("✗"), u, " "+m.changes))
		default:
			out = append(out, name(st.Muted.Render("○"), u, ""))
		}
	}
	for _, u := range mr.Approvals.ApprovedBy {
		if !slices.ContainsFunc(mr.Reviewers, func(r gitlab.User) bool { return r.Username == u.Username }) {
			out = append(out, name(st.Success.Render("✓"), u, ""))
		}
	}
	if len(out) == 0 {
		out = append(out, st.Muted.Render(m.none))
	}
	if mr.Approvals.Left > 0 {
		out = append(out, st.Muted.Render(m.needMore(mr.Approvals.Left)))
	}
	return out
}
