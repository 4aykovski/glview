package i18n

func fillM3RU(m *Messages) {
	l := &m.Links
	l.Key = "ссылки"
	l.Title = func(ref string) string { return "Ссылки · " + ref }
	l.Hints = "j/k ход · enter открыть · y копировать · esc закрыть"
	l.NoLinks = "В MR нет ссылок"
	l.What = "ссылка"
	l.BadRule = func(detail string) string { return "Конфиг: правило links пропущено: " + detail }
	l.LoadFailed = "Не удалось загрузить ссылки MR"
}
