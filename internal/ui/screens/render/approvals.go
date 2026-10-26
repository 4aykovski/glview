package render

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/common"
)

// APPROVALS column of the MR Overview tab and the approvers overlay (SPEC
// 3.5): who has approved and who must still approve. NewApprovalView computes
// the rules, candidates and the "needs my approval" flag once; the column
// and the overlay draw from it.

// defaultSection is the CODEOWNERS section GitLab uses without sections.
const defaultSection = "codeowners"

// RuleView is a shown approval rule with the people who may still approve it.
type RuleView struct {
	Rule  gitlab.ApprovalRule
	Cands []gitlab.User // Eligible without those who approved by the rule
}

// Reviewer is a reviewer (or an approver outside the reviewers) with the
// state shown without rules.
type Reviewer struct {
	User  gitlab.User
	State ReviewerState
}

type ReviewerState int

const (
	ReviewerWaiting ReviewerState = iota
	ReviewerApproved
	ReviewerChanges
)

// ApprovalView is the approval picture of an MR for one render.
type ApprovalView struct {
	MR        gitlab.MergeRequest
	Rules     []RuleView // shown rules in display order; empty: no rules mode
	Outside   []gitlab.User
	Reviewers []Reviewer // no rules mode
	Needs     bool       // my approval is needed
	Me        string     // my username when Needs (highlighted), else ""
}

// Legacy reports that the MR details are not loaded: the column is the
// plain list of approvers.
func (a ApprovalView) Legacy() bool { return !a.MR.Detailed }

// NewApprovalView builds the picture of mr for the user me.
func NewApprovalView(mr gitlab.MergeRequest, me string) ApprovalView {
	a := ApprovalView{MR: mr}
	if !mr.Detailed {
		a.Reviewers = reviewers(mr)
		return a
	}
	for _, r := range shownRules(mr.Approvals.Rules) {
		a.Rules = append(a.Rules, RuleView{Rule: r, Cands: candidates(r)})
	}
	if len(a.Rules) > 0 {
		a.Outside = outside(mr, a.Rules)
	} else {
		a.Reviewers = reviewers(mr)
	}
	a.Needs = needsMyApproval(a, me)
	if a.Needs {
		a.Me = me
	}
	return a
}

// People reports whether there is anybody to list in the overlay.
func (a ApprovalView) People() bool {
	if len(a.Rules) == 0 {
		return len(a.Reviewers) > 0
	}
	for _, r := range a.Rules {
		if len(r.Rule.ApprovedBy)+len(r.Cands) > 0 {
			return true
		}
	}
	return false
}

// shownRules returns the rules with Required > 0: unmet first, then
// invalid, then met; API order inside a group.
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

// RuleTitle is the block title: "<section> · <name>" for a code owner rule
// with a named section, the API name otherwise.
func RuleTitle(r gitlab.ApprovalRule) string {
	if r.Type == gitlab.ApprovalCodeOwner && r.Section != "" && !strings.EqualFold(r.Section, defaultSection) {
		return r.Section + " · " + r.Name
	}
	return r.Name
}

// RuleCount is "have/required" of a rule, have capped at required.
func RuleCount(r gitlab.ApprovalRule) string {
	return strconv.Itoa(min(len(r.ApprovedBy), r.Required)) + "/" + strconv.Itoa(r.Required)
}

func hasUser(us []gitlab.User, name string) bool {
	return slices.ContainsFunc(us, func(u gitlab.User) bool { return u.Username == name })
}

func candidates(r gitlab.ApprovalRule) []gitlab.User {
	var out []gitlab.User
	for _, u := range r.Eligible {
		if !hasUser(r.ApprovedBy, u.Username) {
			out = append(out, u)
		}
	}
	return out
}

// outside lists the approvers of the MR who approved by none of the rules.
func outside(mr gitlab.MergeRequest, rules []RuleView) []gitlab.User {
	var out []gitlab.User
	for _, u := range mr.Approvals.ApprovedBy {
		if !slices.ContainsFunc(rules, func(r RuleView) bool { return hasUser(r.Rule.ApprovedBy, u.Username) }) {
			out = append(out, u)
		}
	}
	return out
}

func reviewerApproved(mr gitlab.MergeRequest, name string) bool {
	return mr.ReviewStates[name] == gitlab.ReviewApproved || hasUser(mr.Approvals.ApprovedBy, name)
}

// reviewers lists the reviewers with their state, then the approvers who
// are not reviewers.
func reviewers(mr gitlab.MergeRequest) []Reviewer {
	var out []Reviewer
	for _, u := range mr.Reviewers {
		st := ReviewerWaiting
		switch {
		case reviewerApproved(mr, u.Username):
			st = ReviewerApproved
		case mr.ReviewStates[u.Username] == gitlab.ReviewRequestedChanges:
			st = ReviewerChanges
		}
		out = append(out, Reviewer{u, st})
	}
	for _, u := range mr.Approvals.ApprovedBy {
		if !hasUser(mr.Reviewers, u.Username) {
			out = append(out, Reviewer{u, ReviewerApproved})
		}
	}
	return out
}

// needsMyApproval: the MR is open and me is a candidate of an unmet valid
// rule (not any_approver) or, without rules, a reviewer who has not
// answered yet.
func needsMyApproval(a ApprovalView, me string) bool {
	mr := a.MR
	if me == "" || mr.State != gitlab.MRStateOpened || mr.Approvals.ApprovedByMe || reviewerApproved(mr, me) {
		return false
	}
	if len(a.Rules) > 0 {
		for _, r := range a.Rules {
			if !r.Rule.Approved && !r.Rule.Invalid && r.Rule.Type != gitlab.ApprovalAnyApprover && hasUser(r.Cands, me) {
				return true
			}
		}
		return false
	}
	for _, r := range a.Reviewers {
		if r.User.Username == me {
			return r.State == ReviewerWaiting
		}
	}
	return false
}

// fitNames returns how many of users fit into width columns joined by ", "
// (the rest becomes " +K"). At least one is kept; a first name that is too
// wide is cut by the caller.
func fitNames(users []gitlab.User, width int) int {
	for k := len(users); k > 1; k-- {
		w := ansi.StringWidth(Names(users[:k]))
		if k < len(users) {
			w += ansi.StringWidth(" +" + strconv.Itoa(len(users)-k))
		}
		if w <= width {
			return k
		}
	}
	return min(1, len(users))
}

// nameLine renders "  <glyph> names +K" (names cut to w columns, me Bold)
// and the number of names that did not fit.
func nameLine(st *common.Styles, glyph string, users []gitlab.User, me string, w int) (string, int) {
	room := w - 4
	k := fitNames(users, room)
	more := len(users) - k
	if more > 0 {
		room -= ansi.StringWidth(" +" + strconv.Itoa(more))
	}
	var text string
	if me != "" && hasUser(users[:k], me) {
		parts := make([]string, k)
		for i, u := range users[:k] {
			parts[i] = u.Username
			if u.Username == me {
				parts[i] = st.Bold.Render(u.Username)
			}
		}
		text = strings.Join(parts, ", ")
	} else {
		text = Names(users[:k])
	}
	line := "  " + glyph + " " + Clip(text, room)
	if more > 0 {
		line += st.Muted.Render(" +" + strconv.Itoa(more))
	}
	return line, more
}

func ruleGlyph(st *common.Styles, r gitlab.ApprovalRule) string {
	switch {
	case r.Invalid:
		return st.Error.Render("✗")
	case r.Approved:
		return st.Success.Render("✓")
	}
	return st.Muted.Render("○")
}

// RuleHeader is the title line of a rule: glyph, name, "1/2" on the right.
func RuleHeader(st *common.Styles, r gitlab.ApprovalRule, w int) string {
	count := RuleCount(r)
	title := Clip(RuleTitle(r), max(w-2-ansi.StringWidth(count)-1, 1))
	return ruleGlyph(st, r) + " " + Spread(title, st.Muted.Render(count), w-2)
}

// RuleKey identifies a rule across refreshes.
func RuleKey(r gitlab.ApprovalRule) string {
	return string(r.Type) + "|" + r.Section + "|" + r.Name
}

func styled(st *common.Styles, name, me string) string {
	if name == me {
		return st.Bold.Render(name)
	}
	return name
}

func reviewerGlyph(st *common.Styles, s ReviewerState) string {
	switch s {
	case ReviewerApproved:
		return st.Success.Render("✓")
	case ReviewerChanges:
		return st.Error.Render("✗")
	}
	return st.Muted.Render("○")
}

// ApprovalColumn is the body of the APPROVALS column (without the heading)
// for the column width w.
func ApprovalColumn(st *common.Styles, m *i18n.Messages, a ApprovalView, w int) []string {
	var out []string
	cut := false // some list ends in "+K"
	if a.Needs {
		out = append(out, st.Warning.Render(m.MR.NeedYourApproval))
	}
	switch {
	case len(a.Rules) > 0:
		for _, rv := range a.Rules {
			r := rv.Rule
			out = append(out, RuleHeader(st, r, w))
			if len(r.ApprovedBy) > 0 {
				line, more := nameLine(st, st.Success.Render("✓"), r.ApprovedBy, a.Me, w)
				out, cut = append(out, line), cut || more > 0
			}
			switch {
			case r.Invalid:
				out = append(out, "  "+st.Muted.Render(m.MR.InvalidRule))
			case r.Approved:
			case r.Type == gitlab.ApprovalAnyApprover:
				out = append(out, "  "+st.Muted.Render(m.MR.AnyApprover))
			case len(rv.Cands) > 0:
				line, more := nameLine(st, st.Muted.Render("○"), rv.Cands, a.Me, w)
				out, cut = append(out, line), cut || more > 0
			}
		}
		for _, u := range a.Outside {
			out = append(out, st.Success.Render("✓ ")+Clip(styled(st, u.Username, a.Me), w-2))
		}
	default:
		for _, r := range a.Reviewers {
			if a.Legacy() && r.State != ReviewerApproved {
				continue // before the details only the approvers are known
			}
			tail := ""
			if r.State == ReviewerChanges {
				tail = " " + m.MR.ChangesAsked
			}
			name := Clip(styled(st, r.User.Username, a.Me), w-2-ansi.StringWidth(tail))
			out = append(out, reviewerGlyph(st, r.State)+" "+name+st.Muted.Render(tail))
		}
		if len(a.Reviewers) == 0 || (a.Legacy() && len(out) == 0) {
			out = append(out, st.Muted.Render(m.MR.None))
		}
		if left := a.MR.Approvals.Left; left > 0 {
			out = append(out, st.Muted.Render(m.MR.NeedApprovals(left)))
		}
	}
	if cut {
		out = append(out, st.Muted.Render(m.Approvers.More))
	}
	return out
}

// ApproverRows is the list of the approvers overlay: with rules, each rule
// header followed by its people (approved first), without rules, the
// reviewers and approvers.
func ApproverRows(st *common.Styles, m *i18n.Messages, a ApprovalView) []common.ApproverRow {
	person := func(key string, u gitlab.User, glyph, tail string) common.ApproverRow {
		return common.ApproverRow{Rule: key, User: u, Person: true, Render: func(w int) string {
			line := glyph + " " + styled(st, u.Username, a.Me)
			if u.Name != "" && u.Name != u.Username {
				line += " " + st.Muted.Render(u.Name)
			}
			if tail != "" {
				line += " " + st.Muted.Render(tail)
			}
			return Clip(line, w)
		}}
	}
	note := func(text string) common.ApproverRow {
		return common.ApproverRow{Render: func(w int) string { return Clip("  "+st.Muted.Render(text), w) }}
	}
	var rows []common.ApproverRow
	if len(a.Rules) == 0 {
		for _, r := range a.Reviewers {
			tail := ""
			if r.State == ReviewerChanges {
				tail = m.MR.ChangesAsked
			}
			rows = append(rows, person("", r.User, reviewerGlyph(st, r.State), tail))
		}
		return rows
	}
	for _, rv := range a.Rules {
		r, key := rv.Rule, RuleKey(rv.Rule)
		rows = append(rows, common.ApproverRow{Render: func(w int) string { return RuleHeader(st, r, w) }})
		for _, u := range r.ApprovedBy {
			rows = append(rows, person(key, u, st.Success.Render("✓"), ""))
		}
		switch {
		case r.Invalid:
			rows = append(rows, note(m.MR.InvalidRule))
		case r.Type == gitlab.ApprovalAnyApprover:
			rows = append(rows, note(m.MR.AnyApprover))
		}
		for _, u := range rv.Cands {
			rows = append(rows, person(key, u, st.Muted.Render("○"), ""))
		}
	}
	return rows
}
