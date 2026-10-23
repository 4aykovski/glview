package i18n

import "fmt"

func fillM3RU(m *Messages) {
	fillTagsRU(m)
	fillTagFormRU(m)
	fillTagsTabRU(m)
	fillTagsWSRU(m)
	fillIssuesRU(m)
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

func fillTagFormRU(m *Messages) {
	t := &m.Tag
	t.FormTitle = "Новый тег"
	t.LblName = "Имя"
	t.LblRef = "Из ref"
	t.LblMessage = "Сообщение (annotated)"
	t.LblCommits = "В теге окажутся:"
	t.Custom = "своё…"
	t.SuggestedFrom = func(kind, base string) string { return "предложено: " + kind + " от " + base }
	t.SuggestedNoBase = func(name string) string { return "нет semver-тегов · предложено " + name }
	t.LoadingTags = "◐ загружаем теги…"
	t.RefCaption = func(ref, sha string, n int, last string) string {
		s := ref
		if sha != "" {
			s += " @ " + sha
		}
		return s + " · " + Count(RU, n, PluralForms{"коммит", "коммита", "коммитов"}) + " после " + last
	}
	t.RefFirstTag = func(ref string) string { return ref + " · первый тег проекта" }
	t.RefComparing = "◐ сравниваем…"
	t.RefCompareErr = "ref не найден или сравнение недоступно"
	t.NoNewCommits = func(last string) string { return "нет новых коммитов после " + last }
	t.CommitsMore = func(n int) string { return fmt.Sprintf("… ещё %d", n) }
	t.ErrNoName = "укажи имя тега"
	t.ErrNoRef = "укажи ref"
	t.ErrExists = func(name string) string { return "тег " + name + " уже есть" }
	t.ConfirmTitle = func(name, ref, sha string) string {
		if sha == "" {
			return "Создать тег " + name + " на " + ref + "?"
		}
		return "Создать тег " + name + " на " + ref + " @ " + sha + "?"
	}
	t.ConfirmAnnotated = func(msg string) string { return "Annotated-тег: «" + msg + "»" }
	t.ConfirmLightweight = "Lightweight-тег без сообщения"
	t.ButtonCreate = "Создать"
	t.KeysName = "↑/↓ вариант · enter далее · tab поле · esc отмена"
	t.KeysCustom = "↑/↓ вариант · вводи имя · enter далее · esc отмена"
	t.KeysRef = "вводи ref · enter далее · tab поле · esc отмена"
	t.KeysRefList = "↑/↓ выбор · enter подставить · tab поле · esc отмена"
	t.KeysMessage = "пусто — lightweight-тег · enter далее · esc отмена"
	t.KeysButton = "enter создать · tab поле · esc отмена"
	t.KeysCreating = "создаём…"
}

func fillTagsTabRU(m *Messages) {
	t := &m.Tag
	t.ColTag = "Tag"
	t.ColMessage = "Message"
	t.ReleaseMark = "◆ release"
	t.SearchLabel = "поиск"
	t.SortLabel = "сорт: semver ↓"
	t.SearchHint = "enter искать · esc отмена"
	t.EmptyNone = "Нет тегов"
	t.EmptyQuery = func(q string) string {
		return "Нет тегов по запросу «" + q + "» · backspace — сбросить поиск"
	}
}

func fillTagsWSRU(m *Messages) {
	t := &m.Tag
	t.GroupEmpty = "нет тегов"
	t.NoMine = "я: не применяется к тегам"
	t.PerProject = func(n int) string { return fmt.Sprintf("на проект: %d", n) }
	t.SortVersion = "сорт: version ↓"
}

func fillIssuesRU(m *Messages) {
	c := &m.Comment
	c.Title = func(ref, title string) string { return "Комментарий · " + ref + " " + title }
	c.Hint = "markdown · tab — к кнопке · alt+enter — отправить"
	c.Close = "esc закрыть"
	c.Send = "Отправить"
	c.Empty = "комментарий пуст"
	c.Sending = "отправляем…"
	c.Added = "Комментарий добавлен"
	c.Key = "комментарий"
}
