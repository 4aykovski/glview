package i18n

func fillT35EN(m *Messages) {
	d := &m.Diff
	d.Tab = "Diff"
	d.New, d.Deleted, d.Generated, d.TooLarge = "new", "deleted", "generated", "too large"
	d.Hidden = "diff hidden by GitLab — o opens the file in the browser"
	d.NoText = "no text diff"
	d.Empty = "No changes"
	d.Loading = "◐ loading diff…"
	d.CommentTitle = func(ref, where string) string { return "Comment · " + ref + " · " + where }
	d.StaleHint = "refresh (r)"
	d.KeyLine, d.KeyFile, d.KeyShift = "line", "file", "shift"
	d.KeyToLine, d.KeyThread, d.KeyDiff = "to line", "thread", "diff"
	d.KeyBrowser = "file in browser"
}
