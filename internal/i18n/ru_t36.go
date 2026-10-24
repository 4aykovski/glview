package i18n

import "fmt"

func fillTodoRU(m *Messages) {
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
		"assigned":                "назначен",
		"mentioned":               "упоминание",
		"directly_addressed":      "обращение",
		"review_requested":        "ревью",
		"review_submitted":        "ревью оставлено",
		"approval_required":       "нужен апрув",
		"build_failed":            "пайплайн упал",
		"unmergeable":             "нельзя смержить",
		"merge_train_removed":     "убран из train",
		"marked":                  "добавлено",
		"member_access_requested": "запрос доступа",
	}
	t.Action = func(name string) string {
		if l, ok := labels[name]; ok {
			return l
		}
		return name
	}
	t.ColTarget, t.ColTitle, t.ColAction, t.ColAgo, t.ColAuthor, t.ColProject = "Цель", "Заголовок", "Причина", "Назад", "Автор", "Проект"
	t.EmptyPending = "В To-Do пусто"
	t.EmptyDone = "Нет выполненных задач"
	t.EmptyFiltered = "Нет задач по фильтрам · backspace — сбросить"
	t.KeyType = "тип"
	t.KeyDone = "выполнено"
	t.KeyDoneAll = "все выполнены"
	t.KeyBrowserPage = "To-Do в браузере"
	t.KeyTarget = "цель в браузере"
	t.Done = func(ref string) string { return "Готово: " + ref }
	t.Cleared = "To-Do очищен"
	t.ConfirmAll = "Отметить все задачи выполненными?"
	t.ConfirmBody = func(n int) string {
		if n < 0 {
			return "Больше 10 000 задач в To-Do. Вернуть их в TUI нельзя."
		}
		return Count(RU, n, PluralForms{"задача", "задачи", "задач"}) + " в To-Do. Вернуть их в TUI нельзя."
	}
}
