package i18n

func fillM3EN(m *Messages) {
	fillTagsEN(m)
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
