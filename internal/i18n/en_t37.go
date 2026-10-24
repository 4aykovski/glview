package i18n

import "fmt"

func fillT37EN(m *Messages) {
	n := &m.NewMR
	n.Crumb = "new MR"
	n.KeyNew = "new MR"
	n.KeyField = "field"
	n.KeyClose = "close"
	n.LblFrom = "From branch"
	n.LblInto = "Into branch"
	n.LblTitle = "Title"
	n.LblTemplate = "Template"
	n.LblDesc = "Description"
	n.LblAssignees = "Assignees"
	n.LblReviewers = "Reviewers"
	n.LblLabels = "Labels"
	n.LblMilestone = "Milestone"
	n.CheckDraft = "Draft"
	n.CheckRemove = "delete branch after merge"
	n.CheckSquash = "squash"
	n.ByProject = "by project setting"
	n.NoTemplate = "no template"
	n.TemplatesLoading = "◐ loading templates…"
	n.TemplatesFailed = "templates unavailable"
	n.PhBranch = "branch name"
	n.PhSearch = "search…"
	n.PhDesc = "markdown"
	n.Comparing = "◐ comparing…"
	n.CompareErr = "branch not found or comparison unavailable"
	n.DupCheckErr = "could not check for an open MR from this branch"
	n.Checking = "checking branches…"
	n.Creating = "creating…"
	n.Stat = func(commits, files int) string {
		return Count(EN, commits, PluralForms{"commit", "", "commits"}) + " · " + Count(EN, files, PluralForms{"file", "", "files"})
	}
	n.NoChanges = func(src, dst string) string { return "no changes: " + src + " is not ahead of " + dst }
	n.NotPushed = func(src string) string { return "branch " + src + " is not on the server — git push first" }
	n.Unpushed = func(c int) string {
		return Count(EN, c, PluralForms{"commit", "", "commits"}) + " not pushed — they will not be in the MR"
	}
	n.DupOpen = func(src string, iid int64) string { return fmt.Sprintf("an MR from %s is already open: !%d", src, iid) }
	n.ErrNoBranch = "enter a branch"
	n.ErrSame = "branches are the same"
	n.ErrNoTitle = "enter a title"
	n.ConfirmReplaceTitle = func(name string) string { return "Replace the description with the template “" + name + "”?" }
	n.ConfirmReplaceBody = "Your edits of the description will be lost."
	n.ConfirmDup = func(src string, iid int64, title string) string {
		return fmt.Sprintf("An MR from %s is already open: !%d “%s”. Open it?", src, iid, title)
	}
	n.ConfirmDupTitle = "MR already exists"
	n.PrevWill = func(src, dst string) string { return "will be created: " + src + " → " + dst }
	n.PrevDraft = "draft"
	n.PrevAssignee = func(name string) string { return "assignee " + name }
	n.PrevAssignees = func(c int) string { return fmt.Sprintf("assignees %d", c) }
	n.PrevReviewers = func(c int) string { return fmt.Sprintf("reviewers %d", c) }
	n.PrevLabels = func(c int) string { return fmt.Sprintf("labels %d", c) }
	n.PrevMilestone = func(title string) string { return "milestone " + title }
	n.ButtonCreate = "Create"
	n.KeysText = "tab next field · esc close"
	n.KeysHints = "↑/↓ pick · enter use · tab next field · esc close"
	n.KeysDesc = "enter newline · tab next field · esc close"
	n.KeysCheck = "space toggle · tab next field · esc close"
	n.KeysTemplate = "↑/↓ pick · enter apply · tab next field · esc close"
	n.KeysButton = "enter create · tab next field · esc close"
	n.KeysWait = "checking branches… · esc stop"
	n.KeysRun = "creating…"
	n.Created = func(iid int64) string { return fmt.Sprintf("MR !%d created", iid) }
}
