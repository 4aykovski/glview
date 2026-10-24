package i18n

func fillT35RU(m *Messages) {
	d := &m.Diff
	d.Tab = "Diff"
	d.New, d.Deleted, d.Generated, d.TooLarge = "новый", "удалён", "сгенерирован", "слишком большой"
	d.Hidden = "дифф скрыт GitLab — o откроет файл в браузере"
	d.NoText = "нет текстового диффа"
	d.Empty = "Нет изменений"
	d.Loading = "◐ загружаем дифф…"
	d.CommentTitle = func(ref, where string) string { return "Комментарий · " + ref + " · " + where }
	d.StaleHint = "обнови (r)"
	d.KeyLine, d.KeyFile, d.KeyShift = "строка", "файл", "сдвиг"
	d.KeyToLine, d.KeyThread, d.KeyDiff = "к строке", "тред", "diff"
	d.KeyBrowser = "файл в браузере"
}
