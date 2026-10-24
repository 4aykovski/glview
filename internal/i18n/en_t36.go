package i18n

import "fmt"

func fillTodoEN(m *Messages) {
	t := &m.Todo
	t.Crumb = "To-Do"
	t.Count = func(n int) string {
		if n < 0 {
			return "To-Do 9999+"
		}
		return fmt.Sprintf("To-Do %d", n)
	}
	t.KeyGlobal = "To-Do"
	labels := map[string]string{
		"assigned":                "assigned",
		"mentioned":               "mentioned",
		"directly_addressed":      "addressed",
		"review_requested":        "review requested",
		"review_submitted":        "review submitted",
		"approval_required":       "approval required",
		"build_failed":            "pipeline failed",
		"unmergeable":             "unmergeable",
		"merge_train_removed":     "removed from train",
		"marked":                  "added",
		"member_access_requested": "access requested",
	}
	t.Action = func(name string) string {
		if l, ok := labels[name]; ok {
			return l
		}
		return name
	}
	t.ColTarget, t.ColTitle, t.ColAction, t.ColAgo, t.ColAuthor, t.ColProject = "Target", "Title", "Action", "Ago", "Author", "Project"
	t.EmptyPending = "To-Do is empty"
	t.EmptyDone = "No done to-dos"
	t.FilterAny, t.FilterMR, t.FilterIssue = "any", "MR", "issue"
	t.EmptyFiltered = "No to-dos for the filters · backspace — reset"
	t.KeyType = "type"
	t.KeyDone = "done"
	t.KeyDoneAll = "all done"
	t.KeyBrowserPage = "To-Do in browser"
	t.KeyTarget = "target in browser"
	t.Done = func(ref string) string { return "Done: " + ref }
	t.Cleared = "To-Do cleared"
	t.ConfirmAll = "Mark all to-dos as done?"
	t.ConfirmBody = func(n int) string {
		if n < 0 {
			return "Over 10 000 to-dos. They cannot be brought back into the TUI."
		}
		return Count(EN, n, PluralForms{"to-do", "", "to-dos"}) + " in To-Do. They cannot be brought back into the TUI."
	}
}
