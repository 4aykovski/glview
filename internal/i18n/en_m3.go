package i18n

func fillM3EN(m *Messages) {
	l := &m.Links
	l.Key = "links"
	l.Title = func(ref string) string { return "Links · " + ref }
	l.Hints = "j/k move · enter open · y copy · esc close"
	l.NoLinks = "No links in the MR"
	l.What = "link"
	l.BadRule = func(detail string) string { return "Config: links rule skipped: " + detail }
	l.LoadFailed = "Could not load the MR links"
}
