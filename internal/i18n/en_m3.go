package i18n

import "fmt"

func fillM3EN(m *Messages) {
	fillTagsEN(m)
	fillTagFormEN(m)
	fillTagsTabEN(m)
	fillTagsWSEN(m)
	fillIssuesEN(m)
	l := &m.Links
	l.Key = "links"
	l.Title = func(ref string) string { return "Links · " + ref }
	l.Hints = "j/k move · enter open · y copy · esc close"
	l.NoLinks = "No links in the MR"
	l.What = "link"
	l.BadRule = func(detail string) string { return "Config: links rule skipped: " + detail }
	l.LoadFailed = "Could not load the MR links"
}

func fillTagsEN(m *Messages) {
	t := &m.Tag
	t.DeleteTitle = func(name string) string { return "Delete tag " + name + "?" }
	t.DeleteBody = func(sha string) string { return "The tag is deleted in GitLab, commit " + sha + " stays." }
	t.DeleteProtect = "The tag is protected: GitLab will most likely refuse."
	t.Deleted = func(name string) string { return "Tag " + name + " deleted" }
	t.Protected = func(name string) string { return "Tag " + name + " is protected" }
	t.Created = func(name string) string { return "Tag " + name + " created" }
	t.NoPipeline = "The tag has no pipeline"
	t.WhatName = "tag name"
	t.KeyNew = "new tag"
	t.KeyDelete = "delete"
	t.KeyPipeline = "tag pipeline"
	t.KeyCopyName = "copy name"
	t.KeySearch = "search"
	t.CommitsPlural = func(n int) string { return Count(EN, n, PluralForms{"commit", "", "commits"}) }
	t.PrevProtected = "protected"
	t.PrevAhead = func(branch string, n int) string {
		return branch + " is " + Count(EN, n, PluralForms{"commit", "", "commits"}) + " ahead"
	}
	t.PrevSame = func(branch string) string { return branch + " matches the tag" }
	t.PrevComparing = "◐ comparing…"
	t.PrevCompareErr = "comparison unavailable"
	t.PrevSuggest = func(name string) string { return "n suggests " + name }
	t.PrevRelease = "Release notes:"
}

func fillTagFormEN(m *Messages) {
	t := &m.Tag
	t.FormTitle = "New tag"
	t.LblName = "Name"
	t.LblRef = "From ref"
	t.LblMessage = "Message (annotated)"
	t.LblCommits = "The tag will include:"
	t.Custom = "custom…"
	t.SuggestedFrom = func(kind, base string) string { return "suggested: " + kind + " from " + base }
	t.SuggestedNoBase = func(name string) string { return "no semver tags · suggested " + name }
	t.LoadingTags = "◐ loading tags…"
	t.RefCaption = func(ref, sha string, n int, last string) string {
		s := ref
		if sha != "" {
			s += " @ " + sha
		}
		return s + " · " + Count(EN, n, PluralForms{"commit", "", "commits"}) + " after " + last
	}
	t.RefFirstTag = func(ref string) string { return ref + " · first tag of the project" }
	t.RefComparing = "◐ comparing…"
	t.RefCompareErr = "ref not found or comparison unavailable"
	t.NoNewCommits = func(last string) string { return "no new commits after " + last }
	t.CommitsMore = func(n int) string { return fmt.Sprintf("… %d more", n) }
	t.ErrNoName = "enter a tag name"
	t.ErrNoRef = "enter a ref"
	t.ErrExists = func(name string) string { return "tag " + name + " already exists" }
	t.ConfirmTitle = func(name, ref, sha string) string {
		if sha == "" {
			return "Create tag " + name + " on " + ref + "?"
		}
		return "Create tag " + name + " on " + ref + " @ " + sha + "?"
	}
	t.ConfirmAnnotated = func(msg string) string { return "Annotated tag: “" + msg + "”" }
	t.ConfirmLightweight = "Lightweight tag without a message"
	t.ButtonCreate = "Create"
	t.KeysName = "↑/↓ variant · enter next · tab field · esc cancel"
	t.KeysCustom = "↑/↓ variant · type a name · enter next · esc cancel"
	t.KeysRef = "type a ref · enter next · tab field · esc cancel"
	t.KeysRefList = "↑/↓ pick · enter use · tab field · esc cancel"
	t.KeysMessage = "empty = lightweight tag · enter next · esc cancel"
	t.KeysButton = "enter create · tab field · esc cancel"
	t.KeysCreating = "creating…"
}

func fillTagsTabEN(m *Messages) {
	t := &m.Tag
	t.ColTag = "Tag"
	t.ColMessage = "Message"
	t.ReleaseMark = "◆ release"
	t.SearchLabel = "search"
	t.SortLabel = "sort: semver ↓"
	t.SearchHint = "enter search · esc cancel"
	t.EmptyNone = "No tags"
	t.EmptyQuery = func(q string) string { return "No tags matching “" + q + "” · backspace — reset the search" }
}

func fillTagsWSEN(m *Messages) {
	t := &m.Tag
	t.GroupEmpty = "no tags"
	t.NoMine = "me: not applied to tags"
	t.PerProject = func(n int) string { return fmt.Sprintf("per project: %d", n) }
	t.SortVersion = "sort: version ↓"
}

func fillIssuesEN(m *Messages) {
	c := &m.Comment
	c.Title = func(ref, title string) string { return "Comment · " + ref + " " + title }
	c.Hint = "markdown · tab — to the button · alt+enter — send"
	c.Close = "esc close"
	c.Send = "Send"
	c.Empty = "comment is empty"
	c.Sending = "sending…"
	c.Added = "Comment added"
	c.Key = "comment"
	i := &m.Issue
	i.ConfirmClose = func(iid int64) string { return fmt.Sprintf("Close issue #%d?", iid) }
	i.Closed = func(iid int64) string { return fmt.Sprintf("Issue #%d closed", iid) }
	i.Reopened = func(iid int64) string { return fmt.Sprintf("Issue #%d reopened", iid) }
	i.Crumb = "Issues"
	i.ColLabels = "Labels"
	i.EmptyOpen = "No open issues"
	i.EmptyMine = "No open issues assigned to you · m — drop the “me” filter"
	i.EmptyFiltered = "No issues for the filters · backspace — reset"
	i.KeyAssignee = "assignee"
	i.KeyLabel = "labels (all of them)"
	i.KeyMilestone = "milestone"
	i.KeyToggle = "close/reopen"
	i.KeyBrowser = "issue in browser"
	i.StatusOpen = "open"
	i.StatusClosed = "closed"
	i.Assignee = func(names string) string { return "assignee: " + names }
	i.Milestone = func(title string) string { return "milestone: " + title }
	i.NoDescription = "no description"
	i.PrevComments = func(n int) string { return fmt.Sprintf("COMMENTS %d", n) }
	i.NotesLoading = "◐ loading…"
	i.NotesFailed = "comments unavailable"
	i.WriteHint = "c — write a comment…"
	i.Labels = "Labels:"
	i.Comments = func(n int) string { return fmt.Sprintf("Comments %d", n) }
	i.NoComments = "No comments · c — write one"
}
