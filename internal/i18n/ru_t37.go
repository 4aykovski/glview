package i18n

import "fmt"

func fillT37RU(m *Messages) {
	n := &m.NewMR
	n.Crumb = "новый MR"
	n.KeyNew = "новый MR"
	n.KeyField = "поле"
	n.KeyClose = "закрыть"
	n.LblFrom = "Из ветки"
	n.LblInto = "В ветку"
	n.LblTitle = "Заголовок"
	n.LblTemplate = "Шаблон"
	n.LblDesc = "Описание"
	n.LblAssignees = "Assignees"
	n.LblReviewers = "Reviewers"
	n.LblLabels = "Labels"
	n.LblMilestone = "Milestone"
	n.CheckDraft = "Draft"
	n.CheckRemove = "удалить ветку после merge"
	n.CheckSquash = "squash"
	n.ByProject = "по настройке проекта"
	n.NoTemplate = "без шаблона"
	n.TemplatesLoading = "◐ загружаем шаблоны…"
	n.TemplatesFailed = "шаблоны недоступны"
	n.PhBranch = "имя ветки"
	n.PhSearch = "поиск…"
	n.PhDesc = "markdown"
	n.Comparing = "◐ сравниваем…"
	n.CompareErr = "ветка не найдена или сравнение недоступно"
	n.DupCheckErr = "не удалось проверить, есть ли открытый MR из этой ветки"
	n.Checking = "проверяем ветки…"
	n.Creating = "создаём…"
	n.Stat = func(commits, files int) string {
		return Count(RU, commits, PluralForms{"коммит", "коммита", "коммитов"}) + " · " + Count(RU, files, PluralForms{"файл", "файла", "файлов"})
	}
	n.NoChanges = func(src, dst string) string {
		return "нет изменений: " + src + " не опережает " + dst
	}
	n.NotPushed = func(src string) string {
		return "ветки " + src + " нет на сервере — сначала git push"
	}
	n.Unpushed = func(c int) string {
		return Count(RU, c, PluralForms{"коммит не запушен", "коммита не запушены", "коммитов не запушено"}) + " — в MR их не будет"
	}
	n.DupOpen = func(src string, iid int64) string { return fmt.Sprintf("из %s уже открыт !%d", src, iid) }
	n.ErrNoBranch = "укажи ветку"
	n.ErrSame = "ветки совпадают"
	n.ErrNoTitle = "укажи заголовок"
	n.ConfirmReplaceTitle = func(name string) string {
		return "Заменить описание шаблоном «" + name + "»?"
	}
	n.ConfirmReplaceBody = "Правки описания будут потеряны."
	n.ConfirmDup = func(src string, iid int64, title string) string {
		return fmt.Sprintf("Из %s уже открыт !%d «%s». Открыть его?", src, iid, title)
	}
	n.ConfirmDupTitle = "MR уже есть"
	n.PrevWill = func(src, dst string) string { return "будет создан: " + src + " → " + dst }
	n.PrevDraft = "draft"
	n.PrevAssignee = func(name string) string { return "assignee " + name }
	n.PrevAssignees = func(c int) string { return fmt.Sprintf("assignees %d", c) }
	n.PrevReviewers = func(c int) string { return fmt.Sprintf("reviewers %d", c) }
	n.PrevLabels = func(c int) string { return fmt.Sprintf("labels %d", c) }
	n.PrevMilestone = func(title string) string { return "milestone " + title }
	n.ButtonCreate = "Создать"
	n.KeysText = "tab след. поле · esc закрыть"
	n.KeysHints = "↑/↓ выбор · enter взять · tab след. поле · esc закрыть"
	n.KeysDesc = "enter перенос строки · tab след. поле · esc закрыть"
	n.KeysCheck = "space переключить · tab след. поле · esc закрыть"
	n.KeysTemplate = "↑/↓ выбор · enter применить · tab след. поле · esc закрыть"
	n.KeysButton = "enter создать · tab след. поле · esc закрыть"
	n.KeysWait = "проверяем ветки… · esc прервать"
	n.KeysRun = "создаём…"
	n.Created = func(iid int64) string { return fmt.Sprintf("MR !%d создан", iid) }
}
