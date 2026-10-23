package i18n

func fillM3RU(m *Messages) {
	fillTagsRU(m)
	l := &m.Links
	l.Key = "ссылки"
	l.Title = func(ref string) string { return "Ссылки · " + ref }
	l.Hints = "j/k ход · enter открыть · y копировать · esc закрыть"
	l.NoLinks = "В MR нет ссылок"
	l.What = "ссылка"
	l.BadRule = func(detail string) string { return "Конфиг: правило links пропущено: " + detail }
	l.LoadFailed = "Не удалось загрузить ссылки MR"
}

func fillTagsRU(m *Messages) {
	t := &m.Tag
	t.DeleteTitle = func(name string) string { return "Удалить тег " + name + "?" }
	t.DeleteBody = func(sha string) string {
		return "Тег удалится в GitLab, коммит " + sha + " останется."
	}
	t.DeleteProtect = "Тег защищён: GitLab, скорее всего, откажет."
	t.Deleted = func(name string) string { return "Тег " + name + " удалён" }
	t.Protected = func(name string) string { return "Тег " + name + " защищён" }
	t.Created = func(name string) string { return "Тег " + name + " создан" }
	t.NoPipeline = "У тега нет пайплайна"
	t.WhatName = "имя тега"
	t.KeyNew = "новый тег"
	t.KeyDelete = "удалить"
	t.KeyPipeline = "пайплайн тега"
	t.KeyCopyName = "копировать имя"
	t.KeySearch = "поиск"
	t.CommitsPlural = func(n int) string {
		return Count(RU, n, PluralForms{"коммит", "коммита", "коммитов"})
	}
	t.PrevProtected = "protected"
	t.PrevAhead = func(branch string, n int) string {
		return branch + " опережает на " + Count(RU, n, PluralForms{"коммит", "коммита", "коммитов"})
	}
	t.PrevSame = func(branch string) string { return branch + " совпадает с тегом" }
	t.PrevComparing = "◐ сравниваем…"
	t.PrevCompareErr = "сравнение недоступно"
	t.PrevSuggest = func(name string) string { return "n предложит " + name }
	t.PrevRelease = "Release notes:"
}
